// Package mcp implements the subset of the Model Context Protocol needed to
// expose read-only tools over the stateless Streamable HTTP transport.
//
// It deliberately depends only on the standard library: the official Go SDK
// currently requires Go 1.25 while this project targets Go 1.24, and the
// subset we need (initialize / tools/list / tools/call) is small and stable.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
)

// DefaultProtocolVersion is used when a client asks for a version we do not
// explicitly recognize.
const DefaultProtocolVersion = "2025-06-18"

var supportedVersions = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true,
	"2026-07-28": true,
}

const maxBodyBytes = 1 << 20

// Handler executes one tool call. The returned error becomes an isError tool
// result rather than a JSON-RPC transport error so the agent can recover.
type Handler func(ctx context.Context, arguments json.RawMessage) (*Result, error)

// Tool is a named capability exposed to MCP clients.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     Handler
}

// Result is what a tool returns. Text should be compact JSON or a short
// human-readable message; it is sent as a text content block.
type Result struct {
	Text    string
	IsError bool
}

// JSONResult marshals v into a text result.
func JSONResult(v any) (*Result, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &Result{Text: string(b)}, nil
}

// ErrorResult reports a tool-level failure.
func ErrorResult(format string, args ...any) *Result {
	return &Result{Text: fmt.Sprintf(format, args...), IsError: true}
}

// Server is an HTTP handler speaking stateless Streamable HTTP.
type Server struct {
	name         string
	version      string
	instructions string

	mu     sync.RWMutex
	tools  []Tool
	byName map[string]Tool
}

func NewServer(name, version string) *Server {
	return &Server{name: name, version: version, byName: map[string]Tool{}}
}

// SetInstructions adds the optional guidance shown to clients after initialize.
func (s *Server) SetInstructions(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instructions = text
}

// AddTool registers a tool. Registering the same name twice panics: tool names
// are part of the public contract.
func (s *Server) AddTool(t Tool) {
	if t.Name == "" || t.Handler == nil {
		panic("mcp: tool requires a name and handler")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byName[t.Name]; exists {
		panic("mcp: duplicate tool " + t.Name)
	}
	if t.InputSchema == nil {
		t.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	s.tools = append(s.tools, t)
	s.byName[t.Name] = t
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// ServeHTTP handles one JSON-RPC message. The endpoint is stateless: every
// request carries its own credentials and no session id is issued.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("MCP-Protocol-Version", DefaultProtocolVersion)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "MCP endpoint accepts POST only", http.StatusMethodNotAllowed)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		media, _, err := mime.ParseMediaType(ct)
		if err != nil || media != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}
	if len(body) > maxBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		s.write(w, http.StatusBadRequest, response{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		s.write(w, http.StatusOK, response{
			JSONRPC: "2.0",
			ID:      idOrNull(req.ID),
			Error:   &rpcError{Code: -32600, Message: "invalid request"},
		})
		return
	}

	// Notifications never receive a body.
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		s.initialize(w, req)
	case "ping":
		s.write(w, http.StatusOK, response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
	case "tools/list":
		s.listTools(w, req)
	case "tools/call":
		s.callTool(w, r, req)
	default:
		s.write(w, http.StatusOK, response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		})
	}
}

func (s *Server) initialize(w http.ResponseWriter, req request) {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(req.Params, &params)
	version := DefaultProtocolVersion
	if supportedVersions[params.ProtocolVersion] {
		version = params.ProtocolVersion
	}
	w.Header().Set("MCP-Protocol-Version", version)

	s.mu.RLock()
	result := map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{"name": s.name, "version": s.version},
	}
	if s.instructions != "" {
		result["instructions"] = s.instructions
	}
	s.mu.RUnlock()
	s.write(w, http.StatusOK, response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (s *Server) listTools(w http.ResponseWriter, req request) {
	s.mu.RLock()
	list := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		list = append(list, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	s.mu.RUnlock()
	s.write(w, http.StatusOK, response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"tools": list},
	})
}

func (s *Server) callTool(w http.ResponseWriter, r *http.Request, req request) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		s.write(w, http.StatusOK, response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "invalid params: tool name is required"},
		})
		return
	}
	s.mu.RLock()
	tool, ok := s.byName[params.Name]
	s.mu.RUnlock()
	if !ok {
		s.write(w, http.StatusOK, response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "unknown tool: " + params.Name},
		})
		return
	}

	args := params.Arguments
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	result, err := tool.Handler(r.Context(), args)
	if err != nil {
		result = ErrorResult("%v", err)
	}
	if result == nil {
		result = ErrorResult("tool returned no result")
	}
	out := map[string]any{
		"content": []map[string]any{{"type": "text", "text": result.Text}},
	}
	if result.IsError {
		out["isError"] = true
	}
	s.write(w, http.StatusOK, response{JSONRPC: "2.0", ID: req.ID, Result: out})
}

func (s *Server) write(w http.ResponseWriter, status int, resp response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func idOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}
