package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

const SessionCookie = "kg_session"

var ErrInvalidSession = errors.New("invalid session")

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Manager struct {
	db  *sql.DB
	ttl time.Duration
}

func NewManager(db *sql.DB, ttl time.Duration) *Manager {
	return &Manager{db: db, ttl: ttl}
}

func newToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

func (m *Manager) Create(ctx context.Context, userID int64, userAgent, ip string) (string, time.Time, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(m.ttl)
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at, user_agent, ip) VALUES (?, ?, ?, ?, ?)`,
		hash, userID, expires.Unix(), trim(userAgent, 300), trim(ip, 100),
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (m *Manager) Lookup(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrInvalidSession
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	var u User
	err := m.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?
		  AND s.revoked_at IS NULL
		  AND s.expires_at > ?
		  AND u.status = 1`,
		hash, time.Now().Unix(),
	).Scan(&u.ID, &u.Username, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (m *Manager) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(token))
	_, err := m.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE token_hash = ?`,
		time.Now().Unix(), hex.EncodeToString(sum[:]))
	return err
}

func (m *Manager) Cleanup(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at <= ? OR revoked_at IS NOT NULL`,
		time.Now().Unix())
	return err
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
