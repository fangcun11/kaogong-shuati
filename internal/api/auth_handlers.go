package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"kaogong/internal/auth"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	username := strings.TrimSpace(req.Username)
	key := clientIP(r) + "|" + strings.ToLower(username)
	if !s.limiter.allowed(key) {
		writeError(w, http.StatusTooManyRequests, "LOCKED", "登录失败次数过多，请 15 分钟后再试")
		return
	}
	if username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "BAD_CREDENTIALS", "用户名或密码错误")
		return
	}

	var id int64
	var hash, role string
	var status int
	err := s.db.QueryRowContext(r.Context(),
		`SELECT id, password_hash, role, status FROM users WHERE username = ?`, username).
		Scan(&id, &hash, &role, &status)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "登录失败")
			return
		}
		s.limiter.fail(key)
		writeError(w, http.StatusUnauthorized, "BAD_CREDENTIALS", "用户名或密码错误")
		return
	}
	ok, verifyErr := auth.VerifyPassword(req.Password, hash)
	if verifyErr != nil || !ok || status != 1 {
		s.limiter.fail(key)
		writeError(w, http.StatusUnauthorized, "BAD_CREDENTIALS", "用户名或密码错误")
		return
	}

	token, expires, err := s.auth.Create(r.Context(), id, r.UserAgent(), clientIP(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "创建会话失败")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `UPDATE users SET last_login_at = datetime('now','localtime') WHERE id = ?`, id)
	s.audit(r, id, "auth.login", "user:"+username, "")
	s.limiter.success(key)
	s.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": sessionUser{ID: id, Username: username, Role: role},
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.auth.Revoke(r.Context(), c.Value)
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "WEAK_PASSWORD", "新密码至少 8 位")
		return
	}
	u, _ := userFromContext(r.Context())
	var hash string
	if err := s.db.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id = ?`, u.ID).Scan(&hash); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取账号失败")
		return
	}
	ok, err := auth.VerifyPassword(req.OldPassword, hash)
	if err != nil || !ok {
		writeError(w, http.StatusBadRequest, "BAD_CREDENTIALS", "原密码错误")
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "生成密码失败")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash = ? WHERE id = ?`, newHash, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "修改密码失败")
		return
	}
	// Revoke every session for this account after a password change.
	_, _ = s.db.ExecContext(r.Context(), `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, time.Now().Unix(), u.ID)
	s.clearSessionCookie(w)
	s.audit(r, u.ID, "auth.password_change", "user:"+u.Username, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "relogin": true})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) audit(r *http.Request, actorID int64, action, target, detail string) {
	_, _ = s.db.Exec(
		`INSERT INTO audit_logs (actor_id, action, target, ip, detail) VALUES (?, ?, ?, ?, ?)`,
		actorID, action, target, clientIP(r), detail,
	)
}
