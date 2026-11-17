package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/kasra-sh/zendawn-go/session"
)

func TestShortModelID(t *testing.T) {
	if got := shortModelID("openai/gpt-4o-mini"); got != "gpt-4o-mini" {
		t.Fatalf("got %q", got)
	}
	if got := shortModelID("openai/google/gemma-4-31b-it:free"); got != "google/gemma-4-31b-it:free" {
		t.Fatalf("got %q", got)
	}
	if got := shortModelID("solo"); got != "solo" {
		t.Fatalf("got %q", got)
	}
}

func TestDialogPanelHasRail(t *testing.T) {
	out := dialogPanel(40, accentAgent, "hi")
	if !strings.Contains(out, "hi") {
		t.Fatalf("missing content: %q", out)
	}
	if !strings.Contains(out, "▌") {
		t.Fatalf("missing rail glyph: %q", out)
	}
	assertRowsSealed(t, out)
}

func assertRowsSealed(t *testing.T, out string) {
	t.Helper()
	for i, row := range strings.Split(out, "\n") {
		if !strings.HasSuffix(row, ansiReset) {
			t.Fatalf("row %d not sealed with reset: %q", i, row)
		}
	}
}

func TestComposerMetaUsesModeTag(t *testing.T) {
	got := composerMeta(session.ModeAgent, "openai/gpt-4o-mini")
	if !strings.Contains(got, "AGENT") {
		t.Fatalf("want AGENT badge, got %q", got)
	}
	if !strings.Contains(got, "gpt-4o-mini") {
		t.Fatalf("missing model: %q", got)
	}
	// Must not be plain lowercase mode-as-only-signal without badge styling context —
	// badge renders uppercase AGENT.
	if strings.Contains(got, " agent ") {
		t.Fatalf("unexpected plain agent text: %q", got)
	}
}

func TestHeaderBarFullWidthPanel(t *testing.T) {
	got := headerBar(40, session.ModeAgent, "coder", "/tmp")
	if !strings.Contains(got, "zendawn") || !strings.Contains(got, "AGENT") {
		t.Fatalf("missing brand/badge: %q", got)
	}
	if strings.Contains(got, "▌") {
		t.Fatalf("header must not have side rail: %q", got)
	}
	if !strings.Contains(got, "coder") || !strings.Contains(got, "/tmp") {
		t.Fatalf("missing meta: %q", got)
	}
	if h := lipgloss.Height(got); h != 3 {
		t.Fatalf("want 3-row slab, got height %d", h)
	}
	assertRowsSealed(t, got)
}

func TestFillSlabRowsSealed(t *testing.T) {
	got := fillSlab(40, "hello")
	if !strings.Contains(got, "hello") {
		t.Fatalf("missing content: %q", got)
	}
	if h := lipgloss.Height(got); h != 3 {
		t.Fatalf("want height 3, got %d", h)
	}
	assertRowsSealed(t, got)
}
