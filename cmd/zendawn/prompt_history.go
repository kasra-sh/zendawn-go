package main

import (
	"strings"

	"github.com/kasra-sh/zendawn-go/session"
)

// pushPromptHistory appends a successful prompt; skips empty and immediate duplicates.
func pushPromptHistory(hist []string, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return hist
	}
	if n := len(hist); n > 0 && hist[n-1] == text {
		return hist
	}
	return append(hist, text)
}

func failedStop(reason string) bool {
	switch {
	case reason == session.StopError, reason == session.StopAborted:
		return true
	case strings.HasPrefix(reason, "error:"):
		return true
	default:
		return false
	}
}

const (
	arrowPass        = "pass"
	arrowScrollUp    = "scrollUp"
	arrowScrollDown  = "scrollDown"
	arrowHistoryUp   = "historyUp"
	arrowHistoryDown = "historyDown"
)

// edgeArrowAction decides bare/alt arrow handling for the composer + conversation.
// Bare up/down at edges scroll the conversation (wheel often arrives as arrows);
// interior lines pass through to the textarea. Alt+up/down always browse history.
func edgeArrowAction(key string, line, lineCount int) string {
	switch key {
	case "alt+up":
		return arrowHistoryUp
	case "alt+down":
		return arrowHistoryDown
	case "up":
		if line <= 0 {
			return arrowScrollUp
		}
		return arrowPass
	case "down":
		if lineCount <= 1 || line >= lineCount-1 {
			return arrowScrollDown
		}
		return arrowPass
	default:
		return arrowPass
	}
}

// historyStep applies up (-1) or down (+1) over hist.
// idx -1 means "not browsing" (draft). Returns new idx, text to show, and draft
// (unchanged unless entering browse from draft).
func historyStep(hist []string, idx int, draft, current string, delta int) (newIdx int, text string, newDraft string) {
	newDraft = draft
	if len(hist) == 0 {
		return -1, current, draft
	}
	if idx < 0 {
		if delta >= 0 {
			return -1, current, draft
		}
		newDraft = current
		newIdx = len(hist) - 1
		return newIdx, hist[newIdx], newDraft
	}
	newIdx = idx + delta
	if newIdx < 0 {
		newIdx = 0
		return newIdx, hist[0], newDraft
	}
	if newIdx >= len(hist) {
		return -1, newDraft, newDraft
	}
	return newIdx, hist[newIdx], newDraft
}
