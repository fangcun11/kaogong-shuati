package api

import (
	"context"
	"net/http"
	"strings"

	"kaogong/internal/mcp"
)

type mcpBaseURLKey struct{}

// mcpHandler builds the tool registry once per server instance.
func (s *Server) mcpHandler() http.Handler {
	srv := mcp.NewServer("kaogong", "1.0.0")
	srv.SetInstructions("考公刷题数据的只读接口。先用 study_overview 了解科目与正确率，" +
		"再用 weakness_report 找薄弱章节、compose_practice 组一组题。题目答案与解析用 get_question 获取。")
	for _, tool := range s.mcpTools() {
		srv.AddTool(tool)
	}
	return s.mcpAuth(srv)
}

// mcpRoute keeps the endpoint visible to clients but reports 404 while the
// feature is disabled, instead of falling through to the SPA handler.
func (s *Server) mcpRoute() http.Handler {
	handler := s.mcpHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.MCPEnabled {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "MCP 未启用")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Server) mcpAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kaogong-mcp"`)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "缺少 MCP token")
			return
		}
		user, scope, err := s.auth.LookupMCPToken(r.Context(), token)
		if err != nil || !scopeAllows(scope, "read") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kaogong-mcp"`)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "MCP token 无效或已过期")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, &authUser{
			ID: user.ID, Username: user.Username, Role: user.Role,
		})
		ctx = context.WithValue(ctx, mcpBaseURLKey{}, requestBaseURL(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		return ""
	}
	parts := strings.Fields(raw)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func scopeAllows(scope, want string) bool {
	for _, item := range strings.Split(scope, ",") {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	}
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func firstHeaderValue(raw string) string {
	if raw == "" {
		return ""
	}
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}
