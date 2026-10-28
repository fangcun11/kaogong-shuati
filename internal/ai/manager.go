package ai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Agent struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Role            string  `json:"role"`
	Description     string  `json:"description"`
	SystemPrompt    string  `json:"system_prompt"`
	Skill           string  `json:"skill"`
	BaseURL         string  `json:"base_url"`
	APIKey          string  `json:"-"`
	Model           string  `json:"model"`
	Temperature     float64 `json:"temperature"`
	MaxTokens       int     `json:"max_tokens"`
	Enabled         int     `json:"enabled"`
	ReasoningEffort string  `json:"reasoning_effort"`
	UpdatedAt       string  `json:"updated_at"`
}

type AgentView struct {
	Agent
	APIKeyMasked string `json:"api_key_masked,omitempty"`
	APIKey       string `json:"api_key"`
	SkillLoaded  any    `json:"skill_loaded,omitempty"`
}

type Manager struct {
	db     *sql.DB
	secret []byte
}

func NewManager(db *sql.DB, secret, webDir string) *Manager {
	if secret == "" {
		secret = "kaogong-local-development-secret"
	}
	sum := sha256.Sum256([]byte(secret))
	m := &Manager{db: db, secret: sum[:]}
	_ = m.SeedDefaults(filepath.Join(webDir, "ai-agents.default.json"))
	return m
}

func (m *Manager) SeedDefaults(path string) error {
	var n int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM ai_agents`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var agents []Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		return err
	}
	for _, a := range agents {
		enc, err := m.encrypt(a.APIKey)
		if err != nil {
			return err
		}
		_, err = m.db.Exec(`
			INSERT OR IGNORE INTO ai_agents
			(id, name, role, description, system_prompt, skill, base_url, api_key_enc, model, temperature, max_tokens, enabled, reasoning_effort)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Role, a.Description, a.SystemPrompt, a.Skill, a.BaseURL, enc,
			a.Model, a.Temperature, a.MaxTokens, a.Enabled, "low")
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) List() ([]AgentView, error) {
	rows, err := m.db.Query(`
		SELECT id, name, role, description, system_prompt, skill, base_url, api_key_enc,
		       model, temperature, max_tokens, enabled, reasoning_effort, updated_at
		FROM ai_agents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentView, 0)
	for rows.Next() {
		a, err := m.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m.view(a))
	}
	return out, rows.Err()
}

func (m *Manager) Get(id int64) (*Agent, error) {
	row := m.db.QueryRow(`
		SELECT id, name, role, description, system_prompt, skill, base_url, api_key_enc,
		       model, temperature, max_tokens, enabled, reasoning_effort, updated_at
		FROM ai_agents WHERE id = ?`, id)
	return m.scanOne(row)
}

func (m *Manager) GetByRole(role string) (*Agent, error) {
	row := m.db.QueryRow(`
		SELECT id, name, role, description, system_prompt, skill, base_url, api_key_enc,
		       model, temperature, max_tokens, enabled, reasoning_effort, updated_at
		FROM ai_agents WHERE role = ?`, role)
	return m.scanOne(row)
}

func (m *Manager) scanOne(row *sql.Row) (*Agent, error) {
	var a Agent
	var enc string
	err := row.Scan(&a.ID, &a.Name, &a.Role, &a.Description, &a.SystemPrompt, &a.Skill, &a.BaseURL, &enc,
		&a.Model, &a.Temperature, &a.MaxTokens, &a.Enabled, &a.ReasoningEffort, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	a.APIKey, _ = m.decrypt(enc)
	return &a, nil
}

func (m *Manager) scan(rows *sql.Rows) (Agent, error) {
	var a Agent
	var enc string
	err := rows.Scan(&a.ID, &a.Name, &a.Role, &a.Description, &a.SystemPrompt, &a.Skill, &a.BaseURL, &enc,
		&a.Model, &a.Temperature, &a.MaxTokens, &a.Enabled, &a.ReasoningEffort, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	a.APIKey, _ = m.decrypt(enc)
	return a, nil
}

func (m *Manager) View(a Agent) AgentView {
	return m.view(a)
}

func (m *Manager) view(a Agent) AgentView {
	v := AgentView{Agent: a}
	if a.APIKey != "" {
		k := a.APIKey
		if len(k) > 8 {
			v.APIKeyMasked = k[:4] + "…" + k[len(k)-4:]
		} else {
			v.APIKeyMasked = "****"
		}
	}
	v.APIKey = ""
	return v
}

var allowedFields = map[string]bool{
	"name": true, "description": true, "system_prompt": true, "skill": true,
	"base_url": true, "model": true, "temperature": true, "max_tokens": true,
	"enabled": true, "reasoning_effort": true,
}

func (m *Manager) Update(id int64, fields map[string]any) (bool, error) {
	old, err := m.Get(id)
	if err != nil {
		return false, err
	}
	sets := make([]string, 0)
	args := make([]any, 0)
	promptChanged := false
	for k, v := range fields {
		if !allowedFields[k] {
			continue
		}
		sets = append(sets, k+" = ?")
		args = append(args, v)
		if (k == "system_prompt" || k == "skill") && fmt.Sprint(v) != fmt.Sprint(fieldValue(old, k)) {
			promptChanged = true
		}
	}
	if key, ok := fields["api_key"].(string); ok && strings.TrimSpace(key) != "" {
		enc, err := m.encrypt(key)
		if err != nil {
			return false, err
		}
		sets = append(sets, "api_key_enc = ?")
		args = append(args, enc)
	}
	if len(sets) == 0 {
		return false, nil
	}
	sets = append(sets, "updated_at = datetime('now','localtime')")
	args = append(args, id)
	_, err = m.db.Exec(`UPDATE ai_agents SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return false, err
	}
	if promptChanged {
		_, _ = m.db.Exec(`INSERT INTO prompt_history (agent_id, system_prompt, skill, note) VALUES (?, ?, ?, ?)`,
			id, fmt.Sprint(fields["system_prompt"]), fmt.Sprint(fields["skill"]), "saved from API")
	}
	return promptChanged, nil
}

func fieldValue(a *Agent, key string) any {
	switch key {
	case "system_prompt":
		return a.SystemPrompt
	case "skill":
		return a.Skill
	default:
		return ""
	}
}

func (m *Manager) History(id int64) ([]map[string]any, error) {
	rows, err := m.db.Query(`SELECT id, system_prompt, skill, note, saved_at FROM prompt_history WHERE agent_id = ? ORDER BY id DESC LIMIT 30`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var hid int64
		var prompt, skill, note, saved string
		if rows.Scan(&hid, &prompt, &skill, &note, &saved) == nil {
			out = append(out, map[string]any{"id": hid, "system_prompt": prompt, "skill": skill, "note": note, "saved_at": saved})
		}
	}
	return out, rows.Err()
}

func (m *Manager) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(m.secret)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawStdEncoding.EncodeToString(out), nil
}

func (m *Manager) decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(m.secret)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted key")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	out, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
