package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// WAL permits concurrent readers; writes are serialized by SQLite.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := execSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func execSchema(db *sql.DB) error {
	// modernc/sqlite supports multiple statements through Exec. Split only
	// when a driver rejects the full schema, which keeps the migration file
	// readable.
	if _, err := db.Exec(schema); err == nil {
		return nil
	}
	for _, stmt := range strings.Split(schema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}
	return nil
}

func IsContentEmpty(db *sql.DB) (bool, error) {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM papers").Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
