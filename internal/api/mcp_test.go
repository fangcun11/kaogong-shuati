package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"kaogong/internal/config"
)

func seedMCPContent(t *testing.T, s *Server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO papers (id, subject, subjectName, category, name, questionCount, difficulty)
		 VALUES (1, 'xingce', '公务员·行测', '真题', '2025年测试卷', 2, 0.5)`,
		`INSERT INTO questions (id, questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis)
		 VALUES (1, 9001, 1, '资料分析', 1, '1+1=?', '<p>1+1=?<img src="/img/q1.png"></p>', '["1","2"]', 'B', 1, 3, '<p>因为 1+1=2</p>'),
		        (2, 9002, 1, '言语理解', 1, '2+2=?', '<p>2+2=?</p>', '["3","4"]', 'B', 1, 2, '<p>因为 2+2=4</p>')`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func issueMCPToken(t *testing.T, s *Server, userID int64, name string, ttl time.Duration) string {
	t.Helper()
	token, _, err := s.auth.CreateMCPToken(context.Background(), userID, name, "read", ttl)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func mcpPost(t *testing.T, client *http.Client, endpoint, token string, payload any) (int, []byte) {
	t.Helper()
	var body []byte
	switch v := payload.(type) {
	case nil:
	case string:
		body = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		body = b
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, data
}

func rpcEnvelope(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return out
}

// toolJSON calls a tool and decodes the JSON text it returned.
func toolJSON(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	result, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %v", env)
	}
	if result["isError"] == true {
		t.Fatalf("tool error: %v", result["content"])
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("missing content: %v", result)
	}
	text := content[0].(map[string]any)["text"].(string)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("tool text is not JSON: %q", text)
	}
	return out
}

func callTool(t *testing.T, client *http.Client, endpoint, token, name string, args any) map[string]any {
	t.Helper()
	status, data := mcpPost(t, client, endpoint, token, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if status != http.StatusOK {
		t.Fatalf("%s transport status = %d", name, status)
	}
	return toolJSON(t, rpcEnvelope(t, data))
}

func TestMCPDisabledByDefault(t *testing.T) {
	ts, s := newTestServer(t)
	token := issueMCPToken(t, s, 1, "t", time.Hour)
	status, _ := mcpPost(t, ts.Client(), ts.URL+"/mcp", token, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "ping",
	})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestMCPRequiresValidToken(t *testing.T) {
	ts, s := newTestServerConfig(t, func(c *config.Config) { c.MCPEnabled = true })
	endpoint := ts.URL + "/mcp"
	ping := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}

	if status, _ := mcpPost(t, ts.Client(), endpoint, "", ping); status != http.StatusUnauthorized {
		t.Fatalf("no token status = %d", status)
	}
	if status, _ := mcpPost(t, ts.Client(), endpoint, "kgm_wrong", ping); status != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d", status)
	}

	expired := issueMCPToken(t, s, 1, "expired", time.Hour)
	if _, err := s.db.Exec(`UPDATE mcp_tokens SET expires_at = 0 WHERE name = 'expired'`); err != nil {
		t.Fatal(err)
	}
	if status, _ := mcpPost(t, ts.Client(), endpoint, expired, ping); status != http.StatusUnauthorized {
		t.Fatalf("expired token status = %d", status)
	}

	session := issueMCPToken(t, s, 1, "revoked", time.Hour)
	if _, err := s.db.Exec(`UPDATE mcp_tokens SET revoked_at = 1 WHERE name = 'revoked'`); err != nil {
		t.Fatal(err)
	}
	if status, _ := mcpPost(t, ts.Client(), endpoint, session, ping); status != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d", status)
	}
}

func TestMCPReadOnlyTools(t *testing.T) {
	ts, s := newTestServerConfig(t, func(c *config.Config) { c.MCPEnabled = true })
	seedMCPContent(t, s)
	token := issueMCPToken(t, s, 1, "laptop", time.Hour)
	client := ts.Client()
	endpoint := ts.URL + "/mcp"

	// Handshake.
	status, data := mcpPost(t, client, endpoint, token, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("initialize status = %d", status)
	}
	initResult := rpcEnvelope(t, data)["result"].(map[string]any)
	if initResult["protocolVersion"] != "2025-06-18" {
		t.Fatalf("protocolVersion = %v", initResult["protocolVersion"])
	}
	if status, body := mcpPost(t, client, endpoint, token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); status != http.StatusAccepted || len(body) != 0 {
		t.Fatalf("notification status=%d body=%q", status, body)
	}

	// Tool registry.
	_, data = mcpPost(t, client, endpoint, token, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	tools := rpcEnvelope(t, data)["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, raw := range tools {
		names[raw.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"study_overview", "list_questions", "get_question", "weakness_report", "compose_practice"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %v", want, names)
		}
	}
	if len(names) != 5 {
		t.Fatalf("tools = %v, want exactly 5", names)
	}

	// Overview.
	overview := callTool(t, client, endpoint, token, "study_overview", map[string]any{})
	subjects := overview["subjects"].([]any)
	if len(subjects) != 1 {
		t.Fatalf("subjects = %v", subjects)
	}
	first := subjects[0].(map[string]any)
	if first["subjectName"] != "公务员·行测" || first["questions"].(float64) != 2 {
		t.Fatalf("subject = %v", first)
	}
	if overview["attempts"].(float64) != 0 || overview["accuracyPercent"].(float64) != 0 {
		t.Fatalf("overview = %v", overview)
	}

	// Question detail.
	detail := callTool(t, client, endpoint, token, "get_question", map[string]any{"id": 9001})
	if detail["stem"] != "1+1=?" || detail["answer"] != "B" {
		t.Fatalf("detail = %v", detail)
	}
	if detail["analysis"] != "因为 1+1=2" {
		t.Fatalf("analysis = %v", detail["analysis"])
	}
	images := detail["images"].([]any)
	if len(images) != 1 || !strings.HasSuffix(images[0].(string), "/img/q1.png") || !strings.HasPrefix(images[0].(string), "http://") {
		t.Fatalf("images = %v", images)
	}
	noAnswer := callTool(t, client, endpoint, token, "get_question", map[string]any{"id": "9001", "includeAnswer": false})
	if _, ok := noAnswer["answer"]; ok {
		t.Fatalf("answer leaked: %v", noAnswer)
	}

	// Compose drops answers and analysis on purpose.
	compose := callTool(t, client, endpoint, token, "compose_practice", map[string]any{"subject": "公务员·行测", "n": 2})
	questions := compose["questions"].([]any)
	if len(questions) != 2 {
		t.Fatalf("compose count = %v", compose["count"])
	}
	if compose["answersOmitted"] != true {
		t.Fatalf("answersOmitted = %v", compose["answersOmitted"])
	}
	q0 := questions[0].(map[string]any)
	if _, ok := q0["answer"]; ok {
		t.Fatalf("compose leaked answer: %v", q0)
	}
	if _, ok := q0["analysis"]; ok {
		t.Fatalf("compose leaked analysis: %v", q0)
	}

	// Unseen before any records.
	unseen := callTool(t, client, endpoint, token, "list_questions", map[string]any{"state": "unseen", "subject": "公务员·行测"})
	if got := len(unseen["items"].([]any)); got != 2 {
		t.Fatalf("unseen = %d, want 2", got)
	}

	// Record a wrong answer, then check the wrong book and weakness report.
	if _, err := s.db.Exec(`INSERT INTO practice_records
		(user_id, question_id, subject, chapter, question_type, selected, is_correct, group_key)
		VALUES (1, '9001', '公务员·行测', '资料分析', 1, '[]', 0, '行测')`); err != nil {
		t.Fatal(err)
	}
	wrong := callTool(t, client, endpoint, token, "list_questions", map[string]any{"state": "wrong"})
	items := wrong["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["questionId"].(float64) != 9001 {
		t.Fatalf("wrong items = %v", items)
	}
	if wrong["total"].(float64) != 1 {
		t.Fatalf("wrong total = %v", wrong["total"])
	}
	filtered := callTool(t, client, endpoint, token, "list_questions", map[string]any{"state": "wrong", "subject": "公务员·申论"})
	if got := len(filtered["items"].([]any)); got != 0 {
		t.Fatalf("subject filter returned %d items", got)
	}

	unseen = callTool(t, client, endpoint, token, "list_questions", map[string]any{"state": "unseen", "subject": "公务员·行测"})
	items = unseen["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["questionId"].(float64) != 9002 {
		t.Fatalf("unseen after record = %v", items)
	}

	weak := callTool(t, client, endpoint, token, "weakness_report", map[string]any{"minAttempts": 1})
	weakItems := weak["items"].([]any)
	if len(weakItems) != 1 {
		t.Fatalf("weak items = %v", weakItems)
	}
	chapter := weakItems[0].(map[string]any)
	if chapter["chapter"] != "资料分析" || chapter["accuracyPercent"].(float64) != 0 {
		t.Fatalf("weak chapter = %v", chapter)
	}

	// Tokens are scoped to their user.
	if _, err := s.db.Exec(`INSERT INTO users (id, username, password_hash, role) VALUES (2, 'other', 'x', 'user')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO practice_records
		(user_id, question_id, subject, chapter, question_type, selected, is_correct, group_key)
		VALUES (2, '9002', '公务员·行测', '言语理解', 1, '[]', 0, '行测')`); err != nil {
		t.Fatal(err)
	}
	otherToken := issueMCPToken(t, s, 2, "other", time.Hour)
	mine := callTool(t, client, endpoint, token, "list_questions", map[string]any{"state": "wrong"})
	if got := len(mine["items"].([]any)); got != 1 {
		t.Fatalf("user 1 sees %d wrong items", got)
	}
	theirs := callTool(t, client, endpoint, otherToken, "list_questions", map[string]any{"state": "wrong"})
	theirItems := theirs["items"].([]any)
	if len(theirItems) != 1 || theirItems[0].(map[string]any)["questionId"].(float64) != 9002 {
		t.Fatalf("user 2 wrong items = %v", theirItems)
	}
}

func TestMCPBearerBypassesCSRFOriginCheck(t *testing.T) {
	ts, s := newTestServerConfig(t, func(c *config.Config) { c.MCPEnabled = true })
	token := issueMCPToken(t, s, 1, "t", time.Hour)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://agent.example")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}
