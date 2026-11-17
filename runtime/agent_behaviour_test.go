package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/permission"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/tool"
	"github.com/kasra-sh/zendawn-go/tool/local"
)

func TestModeInstructionSuffixAgentNotToolForced(t *testing.T) {
	s := ModeInstructionSuffix(session.ModeAgent)
	if strings.Contains(s, "Implement using tools") {
		t.Fatalf("agent suffix still tool-forced: %s", s)
	}
	lower := strings.ToLower(s)
	for _, want := range []string{"prefer", "chat", "ambiguous", "clarif"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("agent suffix missing %q: %s", want, s)
		}
	}
}

func TestMergeInstructionsIncludesWorkspace(t *testing.T) {
	wd := t.TempDir()
	rt := New(singleDoc(&config.Agent{
		ID:           "a",
		Instructions: "BASE",
		Model:        config.ModelConfig{Provider: "mock", ID: "m"},
	}))
	rt.WorkDir = wd
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	merged := rt.mergeInstructions(s, &config.Agent{ID: "a", Instructions: "BASE"}, session.ModeAgent)
	if !strings.Contains(merged, "[workspace]") {
		t.Fatalf("missing [workspace]:\n%s", merged)
	}
	if !strings.Contains(merged, wd) {
		t.Fatalf("missing workdir %q:\n%s", wd, merged)
	}
	if !strings.Contains(merged, "Do not run pwd") {
		t.Fatalf("missing pwd guidance:\n%s", merged)
	}
}

func TestRejectedToolGuidanceShell(t *testing.T) {
	text := rejectedToolGuidance("shell", "pwd")
	lower := strings.ToLower(text)
	if strings.Contains(lower, "writing") || strings.Contains(lower, "create a file") {
		t.Fatalf("shell reject should not use write wording: %s", text)
	}
	if !strings.Contains(lower, "do not retry") || !strings.Contains(lower, "chat") {
		t.Fatalf("shell reject missing guidance: %s", text)
	}
}

func TestAskRejectShellUsesToolGuidance(t *testing.T) {
	wd := t.TempDir()
	agent := &config.Agent{
		ID:    "a",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
		Tools: []string{"shell"},
		Permissions: []config.PermissionRule{
			{Action: "shell", Resource: "*", Effect: permission.EffectAsk},
		},
	}
	rt := New(singleDoc(agent))
	rt.WorkDir = wd
	if err := local.Register(rt.Tools, local.Options{WorkDir: wd}); err != nil {
		t.Fatal(err)
	}
	rt.Ask = func(sessionID, action, resource, toolCallID string) session.AskDecision {
		return session.AskReject
	}
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}

	snap, err := tool.BuildSnapshot(rt.Tools, []string{"shell"}, false, false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	ag, _ := rt.Doc.Agent("a")
	policy := config.ResolvePolicy(rt.Doc, ag)

	call := session.ContentPart{
		Type: session.PartToolCall, ToolCallID: "tc1", Name: "shell",
		Arguments: map[string]any{"command": "pwd"},
	}
	item, _, _ := rt.runToolCall(context.Background(), s, ag, policy, snap, call)
	body := session.TextParts(item.Content)
	if !item.IsError || !strings.Contains(body, "user rejected") {
		t.Fatalf("shell reject item: isError=%v body=%q", item.IsError, body)
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "writing") || strings.Contains(lower, "create a file") {
		t.Fatalf("shell reject used write wording: %s", body)
	}
	if !strings.Contains(lower, "do not retry") {
		t.Fatalf("shell reject missing retry guidance: %s", body)
	}
}
