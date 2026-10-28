package api

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ctxKey int

const userKey ctxKey = 1

func userFromContext(ctx context.Context) (*authUser, bool) {
	u, ok := ctx.Value(userKey).(*authUser)
	return u, ok
}

type authUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (s *Server) currentUser(r *http.Request) (*authUser, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}
	u, err := s.auth.Lookup(r.Context(), cookie.Value)
	if err != nil {
		return nil, err
	}
	return &authUser{ID: u.ID, Username: u.Username, Role: u.Role}, nil
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.currentUser(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := userFromContext(r.Context())
		if u == nil || u.Role != "admin" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "需要管理员权限")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(self), microphone=()")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob: https:; connect-src 'self' https: http:; font-src 'self' data:; "+
				"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, v)
				writeError(w, http.StatusInternalServerError, "INTERNAL", "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// csrfOriginCheck is a first line of defense for cookie-authenticated unsafe
// requests. A CSRF token can be layered on top later without changing routes.
func (s *Server) csrfOriginCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					writeError(w, http.StatusForbidden, "CSRF", "请求来源不合法")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
