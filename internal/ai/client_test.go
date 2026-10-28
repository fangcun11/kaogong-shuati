package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallOpenAICompatible(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "test-model" {
			t.Fatalf("model = %v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	m := NewManager(nil, "test-secret", t.TempDir())
	agent := &Agent{BaseURL: srv.URL, APIKey: "sk-test", Model: "test-model", SystemPrompt: "system", MaxTokens: 100}
	got, err := m.Call(agent, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("content = %q", got)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("authorization = %q", gotAuth)
	}
}

func TestCallVisionSendsImagePart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(body)
		if !strings.Contains(string(raw), "data:image/png;base64,abc") {
			t.Fatalf("image part missing: %s", raw)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"image ok"}}]}`))
	}))
	defer srv.Close()
	m := NewManager(nil, "test-secret", t.TempDir())
	agent := &Agent{BaseURL: srv.URL, APIKey: "sk-test", Model: "vision", SystemPrompt: "system"}
	got, err := m.CallVision(agent, "describe", []string{"data:image/png;base64,abc"})
	if err != nil || got != "image ok" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestCallRequiresKeyForNonFreeGateway(t *testing.T) {
	m := NewManager(nil, "test-secret", t.TempDir())
	_, err := m.Call(&Agent{BaseURL: "https://example.invalid/v1", Model: "m"}, "x")
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("expected api_key error, got %v", err)
	}
}
