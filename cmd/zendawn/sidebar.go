package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	minMainCol   = 50
	sidebarWidth = 42
	sidebarGap   = 1 // left rule column
)

// sidebarVisibleForWidth is true when there is room for min main + sidebar.
func sidebarVisibleForWidth(width int) bool {
	return width >= minMainCol+sidebarWidth
}

// mainColumnWidth is the terminal width available for header + main (excludes sidebar).
func mainColumnWidth(termWidth int) int {
	if !sidebarVisibleForWidth(termWidth) {
		return termWidth
	}
	w := termWidth - sidebarWidth
	if w < minMainCol {
		return minMainCol
	}
	return w
}

func formatContextPercent(used, limit int) string {
	if limit <= 0 {
		return "—% used"
	}
	pct := float64(used) / float64(limit) * 100
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%.2f%% used", pct)
}

func formatContextTokens(used, limit int) string {
	usedLabel := formatTokenCount(used)
	if limit <= 0 {
		return usedLabel + " / ?"
	}
	return usedLabel + " / " + formatTokenCount(limit)
}

func formatContextCost(cost float64, known bool) string {
	if !known {
		return "$— spent"
	}
	return fmt.Sprintf("$%.2f spent", cost)
}

func shortSessionID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "…"
}

func shortWorkdir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "—"
	}
	home, err := filepath.Abs(dir)
	if err == nil {
		dir = home
	}
	base := filepath.Base(dir)
	parent := filepath.Base(filepath.Dir(dir))
	if parent == "." || parent == "/" || parent == "" {
		return base
	}
	return parent + "/" + base
}

// renderSidebar builds the OpenCode-style right column (pure; no TUI deps beyond styles).
func renderSidebar(height int, sessionID, agent, mode, modelLabel, workdir string, used, limit, totalIn, totalOut int, cost float64, costKnown bool) string {
	if height < 1 {
		height = 1
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	value := lipgloss.NewStyle().Foreground(metaMuted)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	title := fmt.Sprintf("%s · %s", shortSessionID(sessionID), agent)
	if agent == "" {
		title = shortSessionID(sessionID)
	}

	var b strings.Builder
	b.WriteString(label.Render(title))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(label.Render("Context"))
	b.WriteByte('\n')
	b.WriteString(value.Render(formatContextTokens(used, limit)))
	b.WriteByte('\n')
	b.WriteString(value.Render(formatContextPercent(used, limit)))
	b.WriteByte('\n')
	b.WriteString(value.Render(formatContextCost(cost, costKnown)))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(label.Render("Model"))
	b.WriteByte('\n')
	b.WriteString(value.Render(shortModelID(modelLabel)))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(label.Render("Mode"))
	b.WriteByte('\n')
	b.WriteString(value.Render(strings.ToUpper(modeLabel(mode))))

	innerW := sidebarWidth - sidebarGap - 1
	if innerW < 10 {
		innerW = 10
	}
	body := lipgloss.NewStyle().
		Width(innerW).
		MaxWidth(innerW).
		Render(b.String())

	foot := dim.Render(shortWorkdir(workdir)) + "\n" + dim.Render("• zendawn")
	foot = lipgloss.NewStyle().Width(innerW).MaxWidth(innerW).Render(foot)

	col := lipgloss.JoinVertical(lipgloss.Left, body, "", foot)
	col = lipgloss.Place(innerW, height, lipgloss.Left, lipgloss.Top, col)

	rule := lipgloss.NewStyle().
		Foreground(lipgloss.Color("238")).
		Height(height).
		Render(strings.TrimRight(strings.Repeat("│\n", height), "\n"))

	joined := lipgloss.JoinHorizontal(lipgloss.Top, rule, " ", col)
	return lipgloss.NewStyle().
		Width(sidebarWidth).
		MaxWidth(sidebarWidth).
		Height(height).
		MaxHeight(height).
		Render(joined)
}
