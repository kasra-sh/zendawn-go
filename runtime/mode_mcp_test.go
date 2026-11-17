package runtime

import (
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/tool"
)

func TestFilterToolsByModeIncludesMCPInAskEvenIfNotListed(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.Definition{Name: "read", Description: "r", InputSchema: map[string]any{"type": "object"}}, nil)
	reg.Register(tool.Definition{Name: "tavily__search", Description: "s", InputSchema: map[string]any{"type": "object"}}, nil)
	reg.Register(tool.Definition{Name: "shell", Description: "sh", InputSchema: map[string]any{"type": "object"}}, nil)

	// Explicit coding list without MCP names — MCP must still appear in ask mode.
	names, toolsAll := FilterToolsByMode(session.ModeAsk, []string{"read", "shell"}, false, reg)
	if toolsAll {
		t.Fatal("ask mode should not keep toolsAll")
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "read") || !strings.Contains(joined, "tavily__search") {
		t.Fatalf("want read+mcp in ask allowlist, got %v", names)
	}
	if strings.Contains(joined, "shell") {
		t.Fatalf("shell must not be in ask allowlist: %v", names)
	}
}

func TestMCPToolsHint(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.Definition{Name: "tavily__a", Description: "a", InputSchema: map[string]any{"type": "object"}}, nil)
	hint := MCPToolsHint(reg)
	if !strings.Contains(hint, "tavily__a") || !strings.Contains(hint, "[MCP tools]") {
		t.Fatalf("hint=%q", hint)
	}
	if MCPToolsHint(tool.NewRegistry()) != "" {
		t.Fatal("empty registry should yield empty hint")
	}
}
