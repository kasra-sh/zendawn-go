package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/secret"
	"github.com/kasra-sh/zendawn-go/tool"
)

func TestHTTPConnectListCall(t *testing.T) {
	var session string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if session == "" {
			session = "ses1"
			w.Header().Set("Mcp-Session-Id", session)
		}
		id := req.ID
		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      &id,
				Result:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"mock","version":"1"}}`),
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      &id,
				Result:  json.RawMessage(`{"tools":[{"name":"echo","description":"Echo","inputSchema":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}]}`),
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      &id,
				Result:  json.RawMessage(`{"content":[{"type":"text","text":"hello"}],"isError":false}`),
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	sec := secret.New([]config.SecretDecl{{Name: "TOK", Source: "env"}})
	sec.LookupEnv = func(k string) (string, bool) {
		if k == "TOK" {
			return "abc", true
		}
		return "", false
	}
	m := NewManager(sec)
	reg := tool.NewRegistry()
	cfg := &config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"mock": {
				Transport: "http",
				URL:       srv.URL,
				Headers:   map[string]string{"Authorization": "Bearer ${TOK}"},
			},
		},
	}
	rep, err := m.ConnectAll(context.Background(), cfg, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if len(rep.Servers) != 1 || !rep.Servers[0].OK || rep.Servers[0].ToolCount != 1 {
		t.Fatalf("report %+v", rep)
	}

	e, ok := reg.Get("mock__echo")
	if !ok {
		t.Fatal("tool not registered")
	}
	res, err := e.Run(context.Background(), "mock__echo", map[string]any{"text": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 || res.Content[0].Text != "hello" {
		t.Fatalf("result %+v", res)
	}
}

func TestOptionalConnectFailure(t *testing.T) {
	m := NewManager(secret.New(nil))
	reg := tool.NewRegistry()
	cfg := &config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"bad": {Transport: "http", URL: "http://127.0.0.1:1", Optional: true},
		},
	}
	rep, err := m.ConnectAll(context.Background(), cfg, reg)
	if err != nil {
		t.Fatalf("optional should soft-fail: %v", err)
	}
	if len(rep.Servers) != 1 || !rep.Servers[0].Skipped || rep.Servers[0].Err == "" {
		t.Fatalf("want skipped with err, got %+v", rep)
	}
	lines := rep.Lines()
	if len(lines) != 1 || !strings.Contains(lines[0], "skipped bad") {
		t.Fatalf("lines %v", lines)
	}
}
