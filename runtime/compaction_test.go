package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/session"
)

func TestCompactionStrictSummarizerThenMain(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZENDAWN_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	minChars := 1
	agent := &config.Agent{
		ID:             "a",
		Instructions:   "test",
		Model:          config.ModelConfig{Provider: "mock", ID: "m"},
		CompactionMode: "strict",
	}
	doc := &config.Document{
		Single:  agent,
		Runtime: &config.RuntimeConfig{Compaction: &config.CompactionConfig{MinChars: &minChars}},
	}
	rt := New(doc)
	rt.WorkDir = filepath.Join(tmp, "ws")
	_ = os.MkdirAll(rt.WorkDir, 0o755)

	mock := provider.NewMock([]provider.ScriptEntry{
		{Message: &session.Item{Content: []session.ContentPart{{Type: session.PartText, Text: "sum: did stuff"}}}, FinishReason: "stop"},
		{Message: &session.Item{Content: []session.ContentPart{{Type: session.PartText, Text: "final answer"}}}, FinishReason: "stop"},
	})
	rt.RegisterProvider(mock)

	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	s.Lock()
	s.AppendItem(session.Item{
		ID: "u1", Type: session.ItemMessage, Role: session.RoleUser, CreatedAt: time.Now().UTC(),
		Content: []session.ContentPart{{Type: session.PartText, Text: "first ask"}},
	})
	s.AppendItem(session.Item{
		ID: "a1", Type: session.ItemMessage, Role: session.RoleAssistant, CreatedAt: time.Now().UTC(),
		Content: []session.ContentPart{{Type: session.PartText, Text: "first reply"}},
	})
	s.Unlock()

	reason, err := rt.Submit(context.Background(), s.ID, session.UserInput{Text: "latest ask"})
	if err != nil {
		t.Fatal(err)
	}
	if reason != session.StopCompleted {
		t.Fatalf("reason %s", reason)
	}

	if len(mock.Calls) != 2 {
		t.Fatalf("Complete calls=%d want 2 (summarizer+main)", len(mock.Calls))
	}
	sumCall := mock.Calls[0]
	if sumCall.AgentID != "a::compaction" {
		t.Fatalf("summarizer agent id %q", sumCall.AgentID)
	}
	if len(sumCall.Tools) != 0 {
		t.Fatal("summarizer must have no tools")
	}
	mainCall := mock.Calls[1]
	if mainCall.AgentID != "a" {
		t.Fatalf("main agent id %q", mainCall.AgentID)
	}
	if len(mainCall.Messages) < 2 {
		t.Fatalf("main messages %#v", mainCall.Messages)
	}
	first := mainCall.Messages[0]
	if first.Role != session.RoleSystem || !strings.Contains(session.TextParts(first.Content), "sum: did stuff") {
		t.Fatalf("expected summary system note, got %#v", first)
	}
	last := mainCall.Messages[len(mainCall.Messages)-1]
	if session.TextParts(last.Content) != "latest ask" {
		t.Fatalf("latest user not verbatim: %#v", last)
	}

	hist := s.History()
	// seeded user+assistant + submit user + assistant reply
	if len(hist) != 4 {
		t.Fatalf("durable transcript len=%d want 4", len(hist))
	}
	if s.CompactionSummary != "sum: did stuff" || s.CompactionThrough != 2 {
		t.Fatalf("state summary=%q through=%d", s.CompactionSummary, s.CompactionThrough)
	}

	var sawUpdated bool
	for _, ev := range rt.Events() {
		if ev.Type == EventCompactionUpdated {
			sawUpdated = true
			break
		}
	}
	if !sawUpdated {
		t.Fatal("expected compaction.updated event")
	}
}

func TestCompactionModeNoneSingleComplete(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZENDAWN_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	rt := New(singleDoc(&config.Agent{
		ID: "a", Instructions: "test",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}))
	rt.WorkDir = filepath.Join(tmp, "ws")
	_ = os.MkdirAll(rt.WorkDir, 0o755)
	mock := provider.NewMock([]provider.ScriptEntry{
		{Message: &session.Item{Content: []session.ContentPart{{Type: session.PartText, Text: "ok"}}}, FinishReason: "stop"},
	})
	rt.RegisterProvider(mock)

	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	s.Lock()
	s.AppendItem(session.Item{
		Type: session.ItemMessage, Role: session.RoleUser, CreatedAt: time.Now().UTC(),
		Content: []session.ContentPart{{Type: session.PartText, Text: "a"}},
	})
	s.AppendItem(session.Item{
		Type: session.ItemMessage, Role: session.RoleAssistant, CreatedAt: time.Now().UTC(),
		Content: []session.ContentPart{{Type: session.PartText, Text: "b"}},
	})
	s.Unlock()

	if _, err := rt.Submit(context.Background(), s.ID, session.UserInput{Text: "c"}); err != nil {
		t.Fatal(err)
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("calls=%d want 1", len(mock.Calls))
	}
}

func TestResolvePolicyCompactionDefaults(t *testing.T) {
	doc := singleDoc(&config.Agent{
		ID: "a", Instructions: "x",
		Model:          config.ModelConfig{Provider: "mock", ID: "m"},
		CompactionMode: "lax",
	})
	p := config.ResolvePolicy(doc, doc.Single)
	if p.CompactionMode != "lax" {
		t.Fatalf("mode %q", p.CompactionMode)
	}
	if p.CompactionLaxRatio != 0.8 || p.CompactionMinChars != 4000 {
		t.Fatalf("thresholds %v %v", p.CompactionLaxRatio, p.CompactionMinChars)
	}
}
