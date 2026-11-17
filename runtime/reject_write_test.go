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

func TestRejectedWriteGuidanceShort(t *testing.T) {
	text, large, known := rejectedWriteGuidance("edit", "gibberish.txt", map[string]any{"content": "hi\nthere"})
	if large || !known {
		t.Fatalf("large=%v known=%v", large, known)
	}
	if !strings.Contains(strings.ToLower(text), "do not retry") || !strings.Contains(text, "small enough") {
		t.Fatalf("guidance: %s", text)
	}
	if strings.Contains(text, "Content is large") {
		t.Fatal("short content should not ask about large paste")
	}
}

func TestRejectedWriteGuidanceLarge(t *testing.T) {
	big := strings.Repeat("x", largeWriteContentRunes+1)
	text, large, known := rejectedWriteGuidance("edit", "big.txt", map[string]any{"content": big})
	if !large || !known {
		t.Fatalf("large=%v known=%v", large, known)
	}
	if !strings.Contains(text, "Content is large") || !strings.Contains(text, "ask the user") {
		t.Fatalf("guidance: %s", text)
	}
}

func TestAskRejectStickyDenySamePath(t *testing.T) {
	wd := t.TempDir()
	agent := &config.Agent{
		ID:    "a",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
		Tools: []string{"write"},
		Permissions: []config.PermissionRule{
			{Action: "edit", Resource: "*", Effect: permission.EffectAsk},
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

	snap, err := tool.BuildSnapshot(rt.Tools, []string{"write"}, false, false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	ag, _ := rt.Doc.Agent("a")
	policy := config.ResolvePolicy(rt.Doc, ag)

	call := session.ContentPart{
		Type: session.PartToolCall, ToolCallID: "tc1", Name: "write",
		Arguments: map[string]any{"path": "a.txt", "content": "hello"},
	}
	item, _, _ := rt.runToolCall(context.Background(), s, ag, policy, snap, call)
	body := session.TextParts(item.Content)
	if !item.IsError || !strings.Contains(body, "user rejected") {
		t.Fatalf("first reject item: isError=%v body=%q", item.IsError, body)
	}
	if !strings.Contains(body, "small enough") {
		t.Fatalf("short write should allow chat delivery hint: %s", body)
	}

	asked := false
	rt.Ask = func(sessionID, action, resource, toolCallID string) session.AskDecision {
		asked = true
		return session.AskOnce
	}
	call2 := session.ContentPart{
		Type: session.PartToolCall, ToolCallID: "tc2", Name: "write",
		Arguments: map[string]any{"path": "a.txt", "content": "again"},
	}
	item2, _, _ := rt.runToolCall(context.Background(), s, ag, policy, snap, call2)
	if asked {
		t.Fatal("same path should not re-ask after sticky deny")
	}
	if !item2.IsError {
		t.Fatal("expected deny error")
	}

	asked = false
	call3 := session.ContentPart{
		Type: session.PartToolCall, ToolCallID: "tc3", Name: "write",
		Arguments: map[string]any{"path": "b.txt", "content": "other"},
	}
	_, _, _ = rt.runToolCall(context.Background(), s, ag, policy, snap, call3)
	if !asked {
		t.Fatal("other path should still ask")
	}
}
