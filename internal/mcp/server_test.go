package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer() *Server {
	s := NewServer("test", "0.1.0")
	s.AddTool(Tool{
		Name:        "echo",
		Description: "echo the argument",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
		},
		Handler: func(ctx context.Context, arguments json.RawMessage) (*Result, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(arguments, &in); err != nil {
				return nil, err
			}
			return JSONResult(map[string]any{"echo": in.Text})
		},
	})
	return s
}

func postRaw(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func decodeRPC(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestInitializeNegotiatesProtocolVersion(t *testing.T) {
	s := testServer()
	rec := postRaw(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decodeRPC(t, rec)
	result := out["result"].(map[string]any)
	if result["protocolVersion"] != "2025-03-26" {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
	caps := result["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Fatal("missing tools capability")
	}
	if rec.Header().Get("MCP-Protocol-Version") != "2025-03-26" {
		t.Fatalf("header = %q", rec.Header().Get("MCP-Protocol-Version"))
	}
}

func TestInitializeFallsBackForUnknownVersion(t *testing.T) {
	rec := postRaw(t, testServer(), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if got := decodeRPC(t, rec)["result"].(map[string]any)["protocolVersion"]; got != DefaultProtocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", got, DefaultProtocolVersion)
	}
}

func TestNotificationReturnsAcceptedWithoutBody(t *testing.T) {
	rec := postRaw(t, testServer(), `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

func TestToolsListAndCall(t *testing.T) {
	s := testServer()
	list := postRaw(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	out := decodeRPC(t, list)
	tools := out["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "echo" {
		t.Fatalf("tools = %v", tools)
	}

	call := postRaw(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"}}}`)
	out = decodeRPC(t, call)
	result := out["result"].(map[string]any)
	content := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content = %v", content)
	}
	text := content[0].(map[string]any)["text"].(string)
	if text != `{"echo":"hi"}` {
		t.Fatalf("text = %q", text)
	}
	if _, ok := result["isError"]; ok {
		t.Fatal("unexpected isError")
	}
}

func TestToolExecutionErrorIsReportedToAgent(t *testing.T) {
	s := NewServer("test", "1")
	s.AddTool(Tool{
		Name: "boom",
		Handler: func(ctx context.Context, arguments json.RawMessage) (*Result, error) {
			return nil, context.DeadlineExceeded
		},
	})
	rec := postRaw(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"boom"}}`)
	result := decodeRPC(t, rec)["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("result = %v", result)
	}
}

func TestUnknownToolAndMethod(t *testing.T) {
	rec := postRaw(t, testServer(), `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope"}}`)
	errObj := decodeRPC(t, rec)["error"].(map[string]any)
	if errObj["code"].(float64) != -32602 {
		t.Fatalf("code = %v", errObj["code"])
	}

	rec = postRaw(t, testServer(), `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	errObj = decodeRPC(t, rec)["error"].(map[string]any)
	if errObj["code"].(float64) != -32601 {
		t.Fatalf("code = %v", errObj["code"])
	}
}

func TestTransportErrors(t *testing.T) {
	s := testServer()
	get := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, get)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", rec.Code)
	}

	bad := postRaw(t, s, `{not json`)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("parse status = %d", bad.Code)
	}
	if parseErr := decodeRPC(t, bad)["error"].(map[string]any); parseErr["code"].(float64) != -32700 {
		t.Fatalf("parse error = %v", parseErr)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content-type status = %d", rec.Code)
	}
}

func TestAddDuplicateToolPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	s := NewServer("test", "1")
	tool := Tool{Name: "x", Handler: func(context.Context, json.RawMessage) (*Result, error) { return &Result{}, nil }}
	s.AddTool(tool)
	s.AddTool(tool)
}
