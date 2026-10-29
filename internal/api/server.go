package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"kaogong/internal/ai"
	"kaogong/internal/auth"
	"kaogong/internal/config"
)

const sessionCookieName = auth.SessionCookie

type Server struct {
	cfg     config.Config
	db      *sql.DB
	auth    *auth.Manager
	ai      *ai.Manager
	limiter *loginLimiter
}

func New(cfg config.Config, db *sql.DB, authManager *auth.Manager) *Server {
	s := &Server{
		cfg:     cfg,
		db:      db,
		auth:    authManager,
		ai:      ai.NewManager(db, cfg.Secret, cfg.WebDir),
		limiter: newLoginLimiter(5, 15*time.Minute, 15*time.Minute),
	}
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.limiter.cleanup()
			_ = s.auth.Cleanup(context.Background())
		}
	}()
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)
	mux.Handle("/mcp", s.mcpRoute())
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.Handle("POST /api/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/auth/me", s.requireAuth(http.HandlerFunc(s.me)))
	mux.Handle("POST /api/auth/password", s.requireAuth(http.HandlerFunc(s.changePassword)))

	mux.Handle("GET /api/subjects", s.requireAuth(http.HandlerFunc(s.subjects)))
	mux.Handle("GET /api/categories", s.requireAuth(http.HandlerFunc(s.categories)))
	mux.Handle("GET /api/papers", s.requireAuth(http.HandlerFunc(s.papers)))
	mux.Handle("GET /api/papers/{id}", s.requireAuth(http.HandlerFunc(s.paperByID)))
	mux.Handle("GET /api/materials", s.requireAuth(http.HandlerFunc(s.materials)))
	mux.Handle("GET /api/chapters", s.requireAuth(http.HandlerFunc(s.chapters)))
	mux.Handle("GET /api/practice", s.requireAuth(http.HandlerFunc(s.practice)))
	mux.Handle("POST /api/paper/generate", s.requireAuth(http.HandlerFunc(s.generatePaper)))
	mux.Handle("GET /api/question", s.requireAuth(http.HandlerFunc(s.question)))
	mux.Handle("POST /api/check", s.requireAuth(http.HandlerFunc(s.check)))
	mux.Handle("POST /api/records", s.requireAuth(http.HandlerFunc(s.submitRecord)))
	mux.Handle("GET /api/records/stats", s.requireAuth(http.HandlerFunc(s.stats)))
	mux.Handle("GET /api/records/recent", s.requireAuth(http.HandlerFunc(s.recent)))
	mux.Handle("GET /api/records/wrong/groups", s.requireAuth(http.HandlerFunc(s.wrongGroups)))
	mux.Handle("/api/records/wrong", s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			s.removeWrong(w, r)
			return
		}
		s.wrongList(w, r)
	})))
	mux.Handle("/api/favorites", s.requireAuth(http.HandlerFunc(s.favorites)))
	mux.Handle("GET /api/favorites/groups", s.requireAuth(http.HandlerFunc(s.favoriteGroups)))
	mux.Handle("/api/notes", s.requireAuth(http.HandlerFunc(s.notes)))
	mux.Handle("GET /api/notes/groups", s.requireAuth(http.HandlerFunc(s.noteGroups)))
	mux.Handle("POST /api/organize", s.requireAuth(http.HandlerFunc(s.organize)))
	mux.Handle("GET /api/custom/batches", s.requireAuth(http.HandlerFunc(s.customBatches)))
	mux.Handle("GET /api/ai/agents", s.requireAuth(http.HandlerFunc(s.aiAgents)))
	mux.Handle("GET /api/ai/agents/{id}", s.requireAuth(http.HandlerFunc(s.aiAgentByID)))
	mux.Handle("PUT /api/ai/agents/{id}", s.requireAdmin(http.HandlerFunc(s.updateAIAgent)))
	mux.Handle("GET /api/ai/agents/{id}/history", s.requireAdmin(http.HandlerFunc(s.aiAgentHistory)))
	mux.Handle("POST /api/ai/agents/{id}/test", s.requireAdmin(http.HandlerFunc(s.testAIAgent)))
	mux.Handle("DELETE /api/ai/explain-cache", s.requireAuth(http.HandlerFunc(s.clearExplainCache)))
	mux.Handle("POST /api/ai/models-proxy", s.requireAdmin(http.HandlerFunc(s.aiModelsProxy)))
	mux.Handle("POST /api/ai/explain", s.requireAuth(http.HandlerFunc(s.aiExplain)))
	mux.Handle("POST /api/ai/ocr", s.requireAuth(http.HandlerFunc(s.aiOCR)))
	mux.Handle("POST /api/ai/structure", s.requireAuth(http.HandlerFunc(s.aiStructure)))
	mux.Handle("POST /api/ai/grade", s.requireAuth(http.HandlerFunc(s.aiGrade)))
	mux.Handle("GET /api/ai/material", s.requireAuth(http.HandlerFunc(s.aiMaterial)))
	mux.Handle("GET /api/skills", s.requireAuth(http.HandlerFunc(s.listSkills)))
	mux.Handle("POST /api/skills", s.requireAuth(http.HandlerFunc(s.addSkill)))
	mux.Handle("DELETE /api/skills/{name}", s.requireAuth(http.HandlerFunc(s.deleteSkill)))
	mux.Handle("POST /api/skills/fetch", s.requireAuth(http.HandlerFunc(s.fetchSkill)))
	mux.Handle("POST /api/custom/import", s.requireAuth(http.HandlerFunc(s.customImport)))
	mux.Handle("GET /api/custom/questions", s.requireAuth(http.HandlerFunc(s.customQuestions)))
	mux.Handle("PUT /api/custom/batch", s.requireAuth(http.HandlerFunc(s.customBatch)))
	mux.Handle("DELETE /api/custom/batch", s.requireAuth(http.HandlerFunc(s.customBatch)))
	mux.Handle("POST /api/custom/batch/merge", s.requireAuth(http.HandlerFunc(s.customBatchMerge)))
	mux.Handle("POST /api/custom/batch/split", s.requireAuth(http.HandlerFunc(s.customBatchSplit)))
	mux.Handle("PUT /api/custom/question", s.requireAuth(http.HandlerFunc(s.customQuestion)))
	mux.Handle("DELETE /api/custom/question", s.requireAuth(http.HandlerFunc(s.customQuestion)))
	mux.Handle("POST /api/custom/questions/group", s.requireAuth(http.HandlerFunc(s.customGroup)))
	mux.Handle("POST /api/custom/questions/auto-group", s.requireAuth(http.HandlerFunc(s.customAutoGroup)))
	mux.Handle("GET /api/custom/practice", s.requireAuth(http.HandlerFunc(s.customPractice)))
	mux.Handle("POST /api/custom/check", s.requireAuth(http.HandlerFunc(s.customCheck)))

	mux.Handle("GET /api/admin/users", s.requireAdmin(http.HandlerFunc(s.listUsers)))
	mux.Handle("POST /api/admin/users", s.requireAdmin(http.HandlerFunc(s.createUser)))
	mux.Handle("PATCH /api/admin/users/{id}", s.requireAdmin(http.HandlerFunc(s.updateUser)))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
	})
	mux.Handle("/", s.staticHandler())

	return s.recoverer(s.requestLogger(s.securityHeaders(s.csrfOriginCheck(mux))))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "数据库不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
