package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/secret"
	"github.com/kasra-sh/zendawn-go/tool"
	"github.com/kasra-sh/zendawn-go/zerr"
)

// Manager connects MCP servers and registers tools.
type Manager struct {
	clients map[string]*Client
	secrets *secret.Store
}

// NewManager creates an empty manager.
func NewManager(secrets *secret.Store) *Manager {
	return &Manager{
		clients: map[string]*Client{},
		secrets: secrets,
	}
}

// ConnectAll connects document MCP servers and registers tools into reg.
// Optional servers that fail are recorded as Skipped; non-optional failures return err.
func (m *Manager) ConnectAll(ctx context.Context, mcpCfg *config.MCPConfig, reg *tool.Registry) (ConnectReport, error) {
	rep := ConnectReport{}
	if mcpCfg == nil || len(mcpCfg.Servers) == 0 {
		return rep, nil
	}
	for id, srv := range mcpCfg.Servers {
		n, err := m.connectOne(ctx, id, srv, reg)
		if err != nil {
			st := ServerStatus{ID: id, Err: err.Error()}
			if srv.Optional {
				st.Skipped = true
				rep.Servers = append(rep.Servers, st)
				continue
			}
			rep.Servers = append(rep.Servers, st)
			return rep, err
		}
		rep.Servers = append(rep.Servers, ServerStatus{ID: id, OK: true, ToolCount: n})
	}
	return rep, nil
}

func (m *Manager) connectOne(ctx context.Context, id string, srv config.MCPServer, reg *tool.Registry) (int, error) {
	reason := "mcp:" + id
	tr, err := m.openTransport(ctx, id, srv, reason)
	if err != nil {
		return 0, ConnectError(id, err)
	}
	client := NewClient(id, tr)
	if err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return 0, ConnectError(id, err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		return 0, ConnectError(id, err)
	}
	m.clients[id] = client
	for _, td := range tools {
		name := ToolName(id, td.Name)
		if _, exists := reg.Get(name); exists {
			_ = client.Close()
			delete(m.clients, id)
			return 0, ConnectError(id, fmt.Errorf("tool name collision: %s", name))
		}
		def := tool.Definition{
			Name:        name,
			Description: td.Description,
			InputSchema: td.InputSchema,
			Permission:  tool.PermSpec{Action: name},
		}
		if def.Description == "" {
			def.Description = "MCP tool " + name
		}
		if def.InputSchema == nil {
			def.InputSchema = map[string]any{"type": "object"}
		}
		mcpToolName := td.Name
		serverID := id
		reg.Register(def, func(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
			c := m.clients[serverID]
			if c == nil {
				return tool.TextResult("mcp client closed", true, zerr.CodeMCPToolError), nil
			}
			return c.CallTool(ctx, mcpToolName, args)
		})
	}
	return len(tools), nil
}

func (m *Manager) openTransport(ctx context.Context, id string, srv config.MCPServer, reason string) (Transport, error) {
	_ = ctx
	switch srv.Transport {
	case "stdio":
		args := srv.Args
		if m.secrets != nil {
			var err error
			args, err = m.secrets.ExpandSlice(srv.Args, reason)
			if err != nil {
				return nil, err
			}
		}
		var env []string
		if len(srv.Env) > 0 {
			expanded := srv.Env
			if m.secrets != nil {
				var err error
				expanded, err = m.secrets.ExpandMap(srv.Env, reason)
				if err != nil {
					return nil, err
				}
			}
			env = os.Environ()
			for k, v := range expanded {
				env = append(env, k+"="+v)
			}
		}
		return NewStdio(srv.Command, args, env)
	case "http":
		url := srv.URL
		headers := srv.Headers
		if m.secrets != nil {
			var err error
			url, err = m.secrets.Expand(url, reason)
			if err != nil {
				return nil, err
			}
			headers, err = m.secrets.ExpandMap(headers, reason)
			if err != nil {
				return nil, err
			}
		}
		return NewHTTP(url, headers), nil
	default:
		return nil, fmt.Errorf("unknown transport %q", srv.Transport)
	}
}

// Close shuts down all clients.
func (m *Manager) Close() error {
	var errs []string
	for id, c := range m.clients {
		if err := c.Close(); err != nil {
			errs = append(errs, id+": "+err.Error())
		}
		delete(m.clients, id)
	}
	if len(errs) > 0 {
		return fmt.Errorf("mcp close: %s", strings.Join(errs, "; "))
	}
	return nil
}
