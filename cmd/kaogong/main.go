package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"kaogong/internal/api"
	"kaogong/internal/auth"
	"kaogong/internal/config"
	"kaogong/internal/store"
)

func main() {
	cfg := config.Load()
	addr := flag.String("addr", cfg.Addr, "HTTP listen address")
	dbPath := flag.String("db", cfg.DBPath, "SQLite database path")
	webDir := flag.String("web", cfg.WebDir, "static web directory")
	importContent := flag.Bool("import", cfg.ImportContent, "import tiku.db / materials.db / tiku-index.db")
	flag.Parse()

	cfg.Addr = *addr
	cfg.DBPath = *dbPath
	cfg.WebDir = *webDir
	cfg.ImportContent = *importContent
	if err := ensureSecret(&cfg); err != nil {
		log.Fatalf("load encryption secret: %v", err)
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	empty, err := store.IsContentEmpty(db)
	if err != nil {
		log.Fatalf("check content: %v", err)
	}
	if cfg.ImportContent || empty {
		if err := store.EnsureTiku(cfg.TikuPath); err != nil {
			log.Printf("warning: %v", err)
		}
		if _, statErr := os.Stat(cfg.TikuPath); statErr == nil {
			log.Printf("importing legacy content from %s ...", cfg.TikuPath)
			stats, err := store.ImportLegacy(ctx, db, cfg.TikuPath, cfg.MaterialsPath, cfg.IndexPath)
			if err != nil {
				log.Fatalf("import legacy content: %v", err)
			}
			log.Printf("imported papers=%d questions=%d materials=%d categories=%d q_materials=%d q_material_map=%d",
				stats.Papers, stats.Questions, stats.Materials, stats.Categories, stats.QMaterials, stats.QMaterialMap)
		} else if empty {
			log.Printf("warning: content database is empty and %s was not found", cfg.TikuPath)
		}
	}

	if err := ensureAdmin(ctx, db, cfg); err != nil {
		log.Fatal(err)
	}

	authManager := auth.NewManager(db, cfg.SessionTTL)
	apiServer := api.New(cfg, db, authManager)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("kaogong Go server listening on http://%s", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Println("server stopped")
}

func ensureSecret(cfg *config.Config) error {
	if cfg.Secret != "" {
		return nil
	}
	path := filepath.Join(filepath.Dir(cfg.DBPath), "secret.key")
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		cfg.Secret = string(data)
		return nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	secret := base64.RawStdEncoding.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return err
	}
	cfg.Secret = secret
	return nil
}

func ensureAdmin(ctx context.Context, db *sql.DB, cfg config.Config) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.AdminPassword == "" {
		return errors.New("no users exist; set KAOGONG_ADMIN_PASSWORD (and optionally KAOGONG_ADMIN_USER) to create the first admin")
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'admin')`,
		cfg.AdminUser, hash)
	if err != nil {
		return err
	}
	log.Printf("created initial admin user %q", cfg.AdminUser)
	return nil
}
