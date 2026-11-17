package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kasra-sh/zendawn-go/session"
)

const ansiReset = "\x1b[0m"

var (
	accentAsk   = lipgloss.Color("42")  // green
	accentPlan  = lipgloss.Color("214") // orange/yellow
	accentAgent = lipgloss.Color("33")  // blue
	panelBG     = lipgloss.Color("236")
	metaMuted   = lipgloss.Color("245")
)

// modeRailAccent is the left-rail color for the active mode.
func modeRailAccent(mode string) lipgloss.Color {
	switch mode {
	case session.ModeAsk:
		return accentAsk
	case session.ModePlan:
		return accentPlan
	default:
		return accentAgent
	}
}

// modeTag is the filled ASK/PLAN/AGENT badge.
func modeTag(mode string) string {
	if mode == "" {
		mode = session.ModeAgent
	}
	label := strings.ToUpper(modeLabel(mode))
	var bg, fg lipgloss.Color
	switch mode {
	case session.ModeAsk:
		bg, fg = accentAsk, lipgloss.Color("16")
	case session.ModePlan:
		bg, fg = accentPlan, lipgloss.Color("16")
	default:
		bg, fg = accentAgent, lipgloss.Color("231")
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(fg).
		Background(bg).
		Padding(0, 1).
		Render(label)
}

// sealPanelRow clears sticky SGR so the next View line does not inherit panelBG.
func sealPanelRow(row string) string {
	return row + ansiReset
}

// dialogPanel renders an OpenCode-style slab: per-line fill + explicit left rail glyph.
func dialogPanel(width int, accent lipgloss.Color, content string) string {
	if width < 10 {
		width = 10
	}
	innerW := panelInnerWidth(width)
	lineStyle := lipgloss.NewStyle().
		Background(panelBG).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Width(innerW)

	raw := content
	if raw == "" {
		raw = " "
	}
	parts := strings.Split(raw, "\n")
	railStyle := lipgloss.NewStyle().Foreground(accent).Background(panelBG)
	rows := make([]string, 0, len(parts)+2)
	// slight vertical padding like OpenCode
	blank := lineStyle.Render(" ")
	rows = append(rows, sealPanelRow(lipgloss.JoinHorizontal(lipgloss.Top, railStyle.Render("▌"), blank)))
	for _, p := range parts {
		rows = append(rows, sealPanelRow(lipgloss.JoinHorizontal(lipgloss.Top, railStyle.Render("▌"), lineStyle.Render(p))))
	}
	rows = append(rows, sealPanelRow(lipgloss.JoinHorizontal(lipgloss.Top, railStyle.Render("▌"), blank)))
	return strings.Join(rows, "\n")
}

// shortModelID drops the registry provider prefix (first segment) for footer display.
func shortModelID(label string) string {
	if i := strings.Index(label, "/"); i >= 0 && i+1 < len(label) {
		return label[i+1:]
	}
	return label
}

// fillSlab is a composer-style grey panel without the ▌ rail: blank + content + blank.
// Width is terminal columns minus one so a full-width line does not hard-wrap.
func fillSlab(width int, content string) string {
	if width < 10 {
		width = 10
	}
	barW := width - 1
	if barW < 10 {
		barW = 10
	}
	lineStyle := lipgloss.NewStyle().
		Background(panelBG).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Width(barW)
	if content == "" {
		content = " "
	}
	// Long lines truncate instead of wrapping the slab taller than 3 rows.
	content = lipgloss.NewStyle().MaxWidth(barW - 2).Render(content)
	blank := lineStyle.Render(" ")
	return strings.Join([]string{
		sealPanelRow(blank),
		sealPanelRow(lineStyle.Render(content)),
		sealPanelRow(blank),
	}, "\n")
}

// headerBar is a flush panelBG slab (no rail): brand, mode badge, agent · workdir.
func headerBar(width int, mode, agent, workdir string) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Background(panelBG).
		Render("zendawn")
	meta := lipgloss.NewStyle().
		Foreground(metaMuted).
		Background(panelBG).
		Render(" " + agent + " · " + workdir)
	gap := lipgloss.NewStyle().Background(panelBG).Render(" ")
	line := lipgloss.JoinHorizontal(lipgloss.Center, title, gap, modeTag(mode), meta)
	return fillSlab(width, line)
}

// composerMeta is modeTag badge + muted model (mode is never plain tinted text).
func composerMeta(mode, modelLabel string) string {
	model := lipgloss.NewStyle().Foreground(metaMuted).Render(shortModelID(modelLabel))
	sep := lipgloss.NewStyle().Foreground(metaMuted).Render(" · ")
	prod := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("zendawn")
	return lipgloss.JoinHorizontal(lipgloss.Center, modeTag(mode), sep, model, sep, prod)
}

// panelInnerWidth is usable text width inside the filled area (outer minus rail).
func panelInnerWidth(outer int) int {
	// rail(1) + we still want a comfortable line width
	w := outer - 1
	if w < 10 {
		return 10
	}
	return w
}
