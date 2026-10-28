package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const maxSkillText = 2 << 20

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, description, files, created_at FROM user_skills
		WHERE user_id = ? ORDER BY id`, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取技能库失败")
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var name, desc, files, created string
		if rows.Scan(&id, &name, &desc, &files, &created) != nil {
			continue
		}
		var fileList any
		_ = json.Unmarshal([]byte(files), &fileList)
		if fileList == nil {
			fileList = []any{}
		}
		inUse := make([]string, 0)
		arows, _ := s.db.QueryContext(r.Context(), `SELECT name FROM ai_agents WHERE skill = ?`, name)
		if arows != nil {
			for arows.Next() {
				var an string
				if arows.Scan(&an) == nil {
					inUse = append(inUse, an)
				}
			}
			arows.Close()
		}
		out = append(out, map[string]any{"id": id, "name": name, "description": desc, "files": fileList, "inUse": inUse, "created_at": created})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) addSkill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Text        string          `json:"text"`
		Files       json.RawMessage `json:"files"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	name := strings.TrimSpace(req.Name)
	text := strings.TrimSpace(req.Text)
	if name == "" || text == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "技能名和内容不能为空")
		return
	}
	if len(name) > 100 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "技能名过长")
		return
	}
	if len(text) > maxSkillText {
		writeError(w, http.StatusBadRequest, "TOO_LARGE", "技能内容不能超过 2MB")
		return
	}
	files := req.Files
	if len(files) == 0 || !json.Valid(files) {
		files = json.RawMessage("[]")
	}
	u, _ := userFromContext(r.Context())
	var existing int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM user_skills WHERE user_id = ? AND name = ?`, u.ID, name).Scan(&existing)
	_, err := s.db.ExecContext(r.Context(), `
		INSERT INTO user_skills (user_id, name, description, text, files)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, name) DO UPDATE SET
		  description = excluded.description, text = excluded.text, files = excluded.files`,
		u.ID, name, req.Description, text, string(files))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "保存技能失败")
		return
	}
	var referenced int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM ai_agents WHERE skill = ?`, name).Scan(&referenced)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "replaced": existing > 0, "referenced": referenced > 0})
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少技能名")
		return
	}
	u, _ := userFromContext(r.Context())
	_, err := s.db.ExecContext(r.Context(), `DELETE FROM user_skills WHERE user_id = ? AND name = ?`, u.ID, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "删除技能失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) fetchSkill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeBody(w, r, &req); err != nil || !safeOutboundURL(req.URL) {
		writeError(w, http.StatusBadRequest, "BAD_URL", "URL 不合法或指向内网")
		return
	}
	client := safeHTTPClient(30 * time.Second)
	res, err := client.Get(req.URL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "UPSTREAM", "拉取失败")
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "UPSTREAM", "拉取失败")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 5<<20))
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	filename := path.Base(mustURLPath(req.URL))
	kind := "text"
	if strings.Contains(ct, "zip") || strings.HasSuffix(strings.ToLower(filename), ".zip") {
		kind = "zip"
		writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "filename": filename, "data": base64.StdEncoding.EncodeToString(body)})
		return
	}
	if strings.Contains(ct, "json") || strings.HasSuffix(strings.ToLower(filename), ".json") {
		kind = "json"
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "filename": filename, "data": string(body)})
}

func mustURLPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "skill.md"
	}
	return u.Path
}
