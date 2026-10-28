package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kaogong/internal/auth"
)

type adminUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"createdAt"`
	LastLoginAt string `json:"lastLoginAt,omitempty"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, username, role, status, created_at, COALESCE(last_login_at, '') FROM users ORDER BY id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取用户失败")
		return
	}
	defer rows.Close()
	users := make([]adminUser, 0)
	for rows.Next() {
		var u adminUser
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Status, &u.CreatedAt, &u.LastLoginAt); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "读取用户失败")
			return
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 32 {
		writeError(w, http.StatusBadRequest, "BAD_USERNAME", "用户名长度需为 3–32 个字符")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "WEAK_PASSWORD", "密码至少 8 位")
		return
	}
	role := req.Role
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "admin" {
		writeError(w, http.StatusBadRequest, "BAD_ROLE", "角色只能是 user 或 admin")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "生成密码失败")
		return
	}
	res, err := s.db.ExecContext(r.Context(),
		`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`, username, hash, role)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "USERNAME_TAKEN", "用户名已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", "创建用户失败")
		return
	}
	id, _ := res.LastInsertId()
	actor, _ := userFromContext(r.Context())
	s.audit(r, actor.ID, "admin.user_create", "user:"+username, "role="+role)
	writeJSON(w, http.StatusCreated, map[string]any{"user": adminUser{ID: id, Username: username, Role: role, Status: 1}})
}

type updateUserRequest struct {
	Role     *string `json:"role"`
	Status   *int    `json:"status"`
	Password *string `json:"password"`
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_ID", "用户 id 不合法")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	actor, _ := userFromContext(r.Context())
	if actor.ID == id && req.Status != nil && *req.Status == 0 {
		writeError(w, http.StatusBadRequest, "SELF_BAN", "不能封禁自己")
		return
	}
	if req.Role != nil && *req.Role != "user" && *req.Role != "admin" {
		writeError(w, http.StatusBadRequest, "BAD_ROLE", "角色只能是 user 或 admin")
		return
	}
	if req.Role != nil || (req.Status != nil && *req.Status == 0) {
		var admins int
		if err := s.db.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 1`).Scan(&admins); err == nil && admins <= 1 {
			var targetRole string
			var targetStatus int
			if err := s.db.QueryRowContext(r.Context(), `SELECT role, status FROM users WHERE id = ?`, id).Scan(&targetRole, &targetStatus); err == nil {
				if targetRole == "admin" && targetStatus == 1 {
					writeError(w, http.StatusBadRequest, "LAST_ADMIN", "至少需要保留一个管理员")
					return
				}
			}
		}
	}
	if req.Password != nil {
		if len(*req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "WEAK_PASSWORD", "密码至少 8 位")
			return
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "生成密码失败")
			return
		}
		if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "修改密码失败")
			return
		}
		if _, err := s.db.ExecContext(r.Context(), `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, nowUnix(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "注销会话失败")
			return
		}
	}
	if req.Role != nil {
		if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET role = ? WHERE id = ?`, *req.Role, id); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "修改角色失败")
			return
		}
	}
	if req.Status != nil {
		if *req.Status != 0 && *req.Status != 1 {
			writeError(w, http.StatusBadRequest, "BAD_STATUS", "status 只能是 0 或 1")
			return
		}
		if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET status = ? WHERE id = ?`, *req.Status, id); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "修改状态失败")
			return
		}
		if *req.Status == 0 {
			_, _ = s.db.ExecContext(r.Context(), `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, nowUnix(), id)
		}
	}
	s.audit(r, actor.ID, "admin.user_update", "user:"+strconv.FormatInt(id, 10), "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func nowUnix() int64 {
	return time.Now().Unix()
}
