package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/kasra-sh/zendawn-go/session"
)

func TestSidebarVisibleBreakpoint(t *testing.T) {
	if sidebarVisibleForWidth(91) {
		t.Fatal("width 91 should hide sidebar")
	}
	if !sidebarVisibleForWidth(92) {
		t.Fatal("width 92 should show sidebar")
	}
	if !sidebarVisibleForWidth(120) {
		t.Fatal("width 120 should show sidebar")
	}
}

func TestMainColumnWidth(t *testing.T) {
	if got := mainColumnWidth(80); got != 80 {
		t.Fatalf("narrow: got %d want 80", got)
	}
	if got := mainColumnWidth(92); got != 92-sidebarWidth {
		t.Fatalf("wide: got %d want %d", got, 92-sidebarWidth)
	}
	if got := mainColumnWidth(120); got != 120-sidebarWidth {
		t.Fatalf("wide: got %d want %d", got, 120-sidebarWidth)
	}
}

func TestSidebarContextLines(t *testing.T) {
	got := renderSidebar(20, "abcdef0123456789", "coder", "agent", "openai/gpt-4o-mini",
		"/tmp/workdir", 25000, 100000, 1000, 500, 1.25, true)
	for _, want := range []string{
		"Context",
		"25K / 100K",
		"% used",
		"$1.25 spent",
		"Model",
		"gpt-4o-mini",
		"Mode",
		"AGENT",
		"coder",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in sidebar:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "│") {
		t.Fatal("missing left rule")
	}
}

func TestSidebarExactWidth(t *testing.T) {
	got := renderSidebar(24, "ses_abcdef012345", "coder", "agent", "openai/gpt-4o-mini",
		"/tmp/workdir", 0, 100000, 3900, 0, 0.02, true)
	if w := lipgloss.Width(got); w != sidebarWidth {
		t.Fatalf("sidebar width %d want %d", w, sidebarWidth)
	}
	for _, want := range []string{"Context", "AGENT"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q (crushed?):\n%s", want, got)
		}
	}
}

func TestJoinedLayoutFitsTerminal(t *testing.T) {
	helpRaw := "/=skill · enter send · s-enter/^j nl · ↑↓/pg scroll · a-↑↓ hist · tab mode · ^p model · ^a stop · ^c quit"
	for _, term := range []int{100, 120} {
		if !sidebarVisibleForWidth(term) {
			t.Fatalf("term %d should show sidebar", term)
		}
		mainW := mainColumnWidth(term)
		header := headerBar(mainW, session.ModeAgent, "coder", "/tmp/workdir")
		help := lipgloss.NewStyle().MaxWidth(mainW).Render(helpRaw)
		left := lipgloss.JoinVertical(lipgloss.Left, header, help)
		left = lipgloss.NewStyle().Width(mainW).MaxWidth(mainW).Render(left)
		side := renderSidebar(lipgloss.Height(left), "ses_abcdef012345", "coder", "agent",
			"openai/gpt-4o-mini", "/tmp/workdir", 0, 100000, 3900, 0, 0.02, true)
		joined := lipgloss.JoinHorizontal(lipgloss.Top, left, side)
		if w := lipgloss.Width(joined); w > term {
			t.Fatalf("term %d: joined width %d exceeds terminal", term, w)
		}
		if !strings.Contains(joined, "Context") || !strings.Contains(joined, "AGENT") {
			t.Fatalf("term %d: sidebar labels crushed:\n%s", term, joined)
		}
		if !strings.Contains(joined, "zendawn") {
			t.Fatalf("term %d: missing header brand:\n%s", term, joined)
		}
	}
}

func TestFormatContextPercentAndCost(t *testing.T) {
	if got := formatContextPercent(25, 100); got != "25.00% used" {
		t.Fatalf("got %q", got)
	}
	if got := formatContextPercent(10, 0); got != "—% used" {
		t.Fatalf("got %q", got)
	}
	if got := formatContextPercent(4700, 1048576); got != "0.45% used" {
		t.Fatalf("got %q", got)
	}
	if got := formatContextTokens(4700, 1048576); got != "4.7K / 1.0M" {
		t.Fatalf("tokens: got %q", got)
	}
	if got := formatContextTokens(10, 0); got != "10 / ?" {
		t.Fatalf("unknown limit: got %q", got)
	}
	if got := formatContextCost(1.5, true); got != "$1.50 spent" {
		t.Fatalf("got %q", got)
	}
	if got := formatContextCost(0, false); got != "$— spent" {
		t.Fatalf("got %q", got)
	}
}
