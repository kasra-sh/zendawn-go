package tool

import (
	"testing"
)

func TestBuildSnapshotInjectsMCPWhenExplicitToolsList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Definition{Name: "read", Description: "r", InputSchema: map[string]any{"type": "object"}}, nil)
	reg.Register(Definition{Name: "shell", Description: "s", InputSchema: map[string]any{"type": "object"}}, nil)
	reg.Register(Definition{Name: "tavily__search", Description: "t", InputSchema: map[string]any{"type": "object"}}, nil)

	snap, err := BuildSnapshot(reg, []string{"read", "shell"}, false, false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range snap.Tools {
		got[e.Def.Name] = true
	}
	if !got["read"] || !got["shell"] {
		t.Fatalf("missing coding tools: %+v", got)
	}
	if !got["tavily__search"] {
		t.Fatalf("MCP tool not injected: %+v", got)
	}
}

func TestIsMCPToolName(t *testing.T) {
	if !IsMCPToolName("tavily__search") {
		t.Fatal("expected true")
	}
	if IsMCPToolName("shell") || IsMCPToolName("__x") || IsMCPToolName("a__") {
		t.Fatal("expected false")
	}
}
