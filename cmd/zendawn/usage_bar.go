package main

import (
	"fmt"
	"math"
	"strings"
)

const usageBarWidth = 10

// formatTokenCount renders e.g. 25400 → "25.4K", 200000 → "200K", 1048576 → "1.0M".
func formatTokenCount(n int) string {
	if n < 0 {
		n = 0
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n >= 1_000_000 {
		m := float64(n) / 1_000_000.0
		if m >= 100 {
			return fmt.Sprintf("%.0fM", m)
		}
		return fmt.Sprintf("%.1fM", m)
	}
	k := float64(n) / 1000.0
	if k >= 100 || math.Mod(k, 1) < 0.05 {
		return fmt.Sprintf("%.0fK", k)
	}
	return fmt.Sprintf("%.1fK", k)
}

// formatUsageBar returns "[████░░░░░░] 25K/200K" or "[··········] 25K/?" when limit unknown.
func formatUsageBar(used, limit int) string {
	usedLabel := formatTokenCount(used)
	if limit <= 0 {
		return fmt.Sprintf("[%s] %s/?", strings.Repeat("·", usageBarWidth), usedLabel)
	}
	filled := int(math.Round(float64(used) / float64(limit) * float64(usageBarWidth)))
	if filled < 0 {
		filled = 0
	}
	if filled > usageBarWidth {
		filled = usageBarWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", usageBarWidth-filled)
	return fmt.Sprintf("[%s] %s/%s", bar, usedLabel, formatTokenCount(limit))
}

// formatSessionUsageLine builds the footer usage strip.
func formatSessionUsageLine(used, limit, totalIn, totalOut int, costUSD float64, costKnown bool, modelLabel string) string {
	total := totalIn + totalOut
	cost := "—"
	if costKnown {
		cost = fmt.Sprintf("$%.3f", costUSD)
	}
	return fmt.Sprintf("%s · Σ %s tok · %s · %s",
		formatUsageBar(used, limit),
		formatTokenCount(total),
		cost,
		modelLabel,
	)
}

