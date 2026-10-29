package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MCPTokenPrefix makes MCP credentials visually distinct from session tokens.
const MCPTokenPrefix = "kgm_"

var ErrInvalidMCPToken = errors.New("invalid mcp token")

// MCPToken describes an issued credential. The plaintext token is never stored.
type MCPToken struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Scope      string    `json:"scope"`
	CreatedAt  string    `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	LastUsedAt string    `json:"lastUsedAt,omitempty"`
}

func hashMCPToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateMCPToken issues a new bearer token for the given user. The caller is
// responsible for showing the plaintext to the operator exactly once.
func (m *Manager) CreateMCPToken(ctx context.Context, userID int64, name, scope string, ttl time.Duration) (string, MCPToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "mcp"
	}
	if scope == "" {
		scope = "read"
	}
	if ttl <= 0 {
		ttl = 180 * 24 * time.Hour
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", MCPToken{}, err
	}
	token := MCPTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(ttl)
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO mcp_tokens (user_id, name, token_hash, scope, expires_at) VALUES (?, ?, ?, ?, ?)`,
		userID, name, hashMCPToken(token), scope, expires.Unix())
	if err != nil {
		return "", MCPToken{}, fmt.Errorf("create mcp token: %w", err)
	}
	id, _ := res.LastInsertId()
	return token, MCPToken{ID: id, Name: name, Scope: scope, ExpiresAt: expires}, nil
}

// LookupMCPToken resolves a plaintext token to its owner and scope.
func (m *Manager) LookupMCPToken(ctx context.Context, token string) (*User, string, error) {
	token = strings.TrimSpace(token)
	if token == "" || !strings.HasPrefix(token, MCPTokenPrefix) {
		return nil, "", ErrInvalidMCPToken
	}
	var tokenID int64
	var scope string
	var u User
	err := m.db.QueryRowContext(ctx, `
		SELECT t.id, t.scope, u.id, u.username, u.role
		FROM mcp_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ?
		  AND t.revoked_at IS NULL
		  AND t.expires_at > ?
		  AND u.status = 1`,
		hashMCPToken(token), time.Now().Unix(),
	).Scan(&tokenID, &scope, &u.ID, &u.Username, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrInvalidMCPToken
	}
	if err != nil {
		return nil, "", err
	}
	// Avoid a write on every tool call; minute-level freshness is enough.
	_, _ = m.db.ExecContext(ctx, `
		UPDATE mcp_tokens SET last_used_at = datetime('now', 'localtime')
		WHERE id = ? AND (last_used_at IS NULL OR last_used_at < datetime('now', 'localtime', '-1 minute'))`, tokenID)
	return &u, scope, nil
}

// RevokeMCPToken disables a token owned by the given user.
func (m *Manager) RevokeMCPToken(ctx context.Context, userID, tokenID int64) error {
	_, err := m.db.ExecContext(ctx,
		`UPDATE mcp_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		time.Now().Unix(), tokenID, userID)
	return err
}
