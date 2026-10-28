package ai

import (
	"path/filepath"
	"testing"

	"kaogong/internal/store"
)

func TestAPIKeyEncryptedAtRest(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ai_agents
		(id, name, role, description, system_prompt, skill, base_url, api_key_enc, model, temperature, max_tokens, enabled, reasoning_effort)
		VALUES (1, 'test', 'test-role', '', '', '', 'https://example.com/v1', '', 'model', 0.2, 1000, 1, 'low')`); err != nil {
		t.Fatal(err)
	}
	m := NewManager(db, "test-master-secret", t.TempDir())
	if _, err := m.Update(1, map[string]any{"api_key": "sk-very-secret"}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(`SELECT api_key_enc FROM ai_agents WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || stored == "sk-very-secret" {
		t.Fatalf("api key not encrypted: %q", stored)
	}
	got, err := m.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-very-secret" {
		t.Fatalf("decrypted key = %q", got.APIKey)
	}
	view := m.View(*got)
	if view.APIKey != "" || view.APIKeyMasked == "" {
		t.Fatalf("view leaked key or missing mask: %+v", view)
	}
}
