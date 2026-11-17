package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kasra-sh/zendawn-go/tool"
	"github.com/kasra-sh/zendawn-go/zerr"
)

// Client is a tools-only MCP client over a transport.
type Client struct {
	tr     Transport
	server string
}

// NewClient wraps a transport.
func NewClient(serverID string, tr Transport) *Client {
	return &Client{tr: tr, server: serverID}
}

// Initialize performs MCP handshake.
func (c *Client) Initialize(ctx context.Context) error {
	params := initializeParams{
		ProtocolVersion: protocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo:      implInfo{Name: "zendawn", Version: "0.1"},
	}
	_, err := c.tr.Call(ctx, "initialize", params)
	if err != nil {
		return err
	}
	if n, ok := c.tr.(Notifier); ok {
		_ = n.Notify(ctx, "notifications/initialized", map[string]any{})
	}
	return nil
}

// Notifier sends JSON-RPC notifications (no response).
type Notifier interface {
	Notify(ctx context.Context, method string, params any) error
}

// ListTools returns remote tool definitions.
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
	raw, err := c.tr.Call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var res toolsListResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// CallTool invokes tools/call.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (tool.Result, error) {
	raw, err := c.tr.Call(ctx, "tools/call", callParams{Name: name, Arguments: args})
	if err != nil {
		return tool.TextResult(err.Error(), true, zerr.CodeMCPToolError), nil
	}
	var res callResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return tool.TextResult(err.Error(), true, zerr.CodeMCPToolError), nil
	}
	var parts []tool.ContentPart
	var texts []string
	for _, p := range res.Content {
		if p.Type == "text" || p.Text != "" {
			parts = append(parts, tool.ContentPart{Type: "text", Text: p.Text})
			texts = append(texts, p.Text)
		}
	}
	if len(parts) == 0 {
		msg := "mcp tool returned no text content"
		if res.IsError {
			msg = "mcp tool error (no text)"
		}
		parts = []tool.ContentPart{{Type: "text", Text: msg}}
	}
	code := ""
	if res.IsError {
		code = zerr.CodeMCPToolError
	}
	_ = texts
	return tool.Result{IsError: res.IsError, Code: code, Content: parts}, nil
}

// Close closes the transport.
func (c *Client) Close() error {
	if c.tr == nil {
		return nil
	}
	return c.tr.Close()
}

// ToolName builds the namespaced registry name.
func ToolName(serverID, mcpName string) string {
	return serverID + "__" + mcpName
}

// ParseToolName splits a namespaced name.
func ParseToolName(registered string) (serverID, mcpName string, ok bool) {
	i := strings.Index(registered, "__")
	if i <= 0 || i >= len(registered)-2 {
		return "", "", false
	}
	return registered[:i], registered[i+2:], true
}

// ConnectError wraps connect failures.
func ConnectError(serverID string, err error) error {
	return zerr.New(zerr.CodeMCPConnectFailed, fmt.Sprintf("%s: %v", serverID, err), false)
}
