// Package compact implements projection-only transcript compaction.
package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kasra-sh/zendawn-go/session"
)

// Modes
const (
	ModeNone       = "none"
	ModeLax        = "lax"
	ModeStrict     = "strict"
	ModeAggressive = "aggressive"
)

// State is the rolling compact view (not part of durable transcript).
type State struct {
	Summary string
	Through int // exclusive end index into transcript covered by Summary
}

// Thresholds for trigger checks.
type Thresholds struct {
	LaxRatio float64
	MinChars int
}

// DefaultThresholds matches runtime defaults.
func DefaultThresholds() Thresholds {
	return Thresholds{LaxRatio: 0.8, MinChars: 4000}
}

// NormalizeMode returns a known mode or ModeNone.
func NormalizeMode(mode string) string {
	switch mode {
	case ModeLax, ModeStrict, ModeAggressive:
		return mode
	default:
		return ModeNone
	}
}

// LastUserMessageIndex returns the index of the last user message, or -1.
func LastUserMessageIndex(items []session.Item) int {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Type == session.ItemMessage && items[i].Role == session.RoleUser {
			return i
		}
	}
	return -1
}

// EstimateChars sums text / tool JSON / handoff string lengths.
func EstimateChars(items []session.Item) int {
	n := 0
	for _, it := range items {
		n += itemChars(it)
	}
	return n
}

func itemChars(it session.Item) int {
	switch it.Type {
	case session.ItemMessage:
		n := 0
		for _, p := range it.Content {
			switch p.Type {
			case session.PartText:
				n += len(p.Text)
			case session.PartToolCall:
				b, _ := json.Marshal(p.Arguments)
				n += len(p.Name) + len(b) + len(p.ToolCallID)
			case session.PartFile, session.PartImage:
				n += len(p.Filename) + len(p.URI) + len(p.AttachmentID) + 32
			}
		}
		return n
	case session.ItemToolResult:
		return len(it.Name) + len(session.TextParts(it.Content))
	case session.ItemHandoff:
		return len(it.From) + len(it.To) + len(it.Reason) + 32
	case session.ItemError:
		return len(it.Code) + len(it.Message)
	default:
		return 0
	}
}

// ShouldCompact reports whether a compaction pass should run this step.
// alreadyCoveredThrough is the current State.Through; if the only uncovered
// content is the latest user message (and nothing before it to fold), skip.
func ShouldCompact(mode string, items []session.Item, st State, th Thresholds, inputTokens, contextLimit int) bool {
	mode = NormalizeMode(mode)
	if mode == ModeNone {
		return false
	}
	lastUser := LastUserMessageIndex(items)
	if lastUser < 0 {
		return false
	}
	// Nothing before the latest user message to fold.
	if lastUser == 0 {
		return false
	}
	// Already summarized everything before the latest user message.
	if st.Through >= lastUser {
		return false
	}

	switch mode {
	case ModeLax:
		if contextLimit <= 0 {
			return false
		}
		ratio := float64(inputTokens) / float64(contextLimit)
		thresh := th.LaxRatio
		if thresh <= 0 {
			thresh = 0.8
		}
		return ratio >= thresh
	case ModeStrict, ModeAggressive:
		minChars := th.MinChars
		if minChars < 1 {
			minChars = 4000
		}
		return EstimateChars(items) >= minChars
	default:
		return false
	}
}

// CoverThrough returns the exclusive end index to fold into the summary:
// everything before the latest user message.
func CoverThrough(items []session.Item) int {
	lastUser := LastUserMessageIndex(items)
	if lastUser < 0 {
		return 0
	}
	return lastUser
}

// DumpText renders items as a plain-text dump for the summarizer.
func DumpText(items []session.Item) string {
	var b strings.Builder
	for i, it := range items {
		fmt.Fprintf(&b, "[%d] %s", i, formatItem(it))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatItem(it session.Item) string {
	switch it.Type {
	case session.ItemMessage:
		body := session.TextParts(it.Content)
		tcs := session.ToolCalls(it.Content)
		if len(tcs) > 0 {
			var names []string
			for _, tc := range tcs {
				args, _ := json.Marshal(tc.Arguments)
				names = append(names, fmt.Sprintf("%s(%s)", tc.Name, string(args)))
			}
			if body != "" {
				return fmt.Sprintf("%s: %s | tools: %s", it.Role, body, strings.Join(names, "; "))
			}
			return fmt.Sprintf("%s tools: %s", it.Role, strings.Join(names, "; "))
		}
		return fmt.Sprintf("%s: %s", it.Role, body)
	case session.ItemToolResult:
		errTag := ""
		if it.IsError {
			errTag = " ERROR"
		}
		return fmt.Sprintf("tool_result%s %s: %s", errTag, it.Name, session.TextParts(it.Content))
	case session.ItemHandoff:
		return fmt.Sprintf("handoff %s -> %s: %s", it.From, it.To, it.Reason)
	case session.ItemError:
		return fmt.Sprintf("error %s: %s", it.Code, it.Message)
	default:
		return it.Type
	}
}

// SummarizerInstructions returns the system prompt for the summarizer call.
func SummarizerInstructions(mode string) string {
	switch NormalizeMode(mode) {
	case ModeAggressive:
		return strings.TrimSpace(`
You compress conversation history into ultra-compact telegraphic notes (caveman / telegram style).
Rules:
- Preserve every distinct user request as a short bullet; do not drop or invent user asks.
- Keep critical paths, decisions, errors, open tasks.
- Drop chatter, politeness, and redundant tool noise.
- Output ONLY the summary text, no preamble.
`)
	case ModeStrict:
		return strings.TrimSpace(`
You summarize conversation history for an AI coding agent, keeping important parts.
Rules:
- Preserve user requests largely intact (paraphrase lightly only if long).
- Keep decisions, open tasks, file paths, errors, and key tool outcomes.
- Drop filler and redundant repetition.
- Output ONLY the summary text, no preamble.
`)
	default: // lax
		return strings.TrimSpace(`
You compress older conversation turns for an AI coding agent that is running out of context.
Rules:
- Keep decisions, open tasks, file paths, errors, and unresolved user asks.
- Drop chatter and verbose tool payloads when a short outcome suffices.
- Be concise but clear.
- Output ONLY the summary text, no preamble.
`)
	}
}

// BuildSummarizerUserPrompt builds the user message for the summarizer.
func BuildSummarizerUserPrompt(priorSummary string, toFold []session.Item) string {
	var b strings.Builder
	if priorSummary != "" {
		b.WriteString("Prior summary:\n")
		b.WriteString(priorSummary)
		b.WriteString("\n\n")
	}
	b.WriteString("Transcript to fold into the summary:\n")
	b.WriteString(DumpText(toFold))
	return b.String()
}

// Project returns the message list for the provider: optional summary system
// note plus transcript items from state.Through onward. Durable items are not modified.
func Project(items []session.Item, st State) []session.Item {
	if st.Summary == "" || st.Through <= 0 {
		out := make([]session.Item, len(items))
		copy(out, items)
		return out
	}
	through := st.Through
	if through > len(items) {
		through = len(items)
	}
	tail := items[through:]
	out := make([]session.Item, 0, 1+len(tail))
	out = append(out, session.Item{
		ID:        "compaction_summary",
		Type:      session.ItemMessage,
		Role:      session.RoleSystem,
		CreatedAt: time.Now().UTC(),
		Content: []session.ContentPart{{
			Type: session.PartText,
			Text: "## Conversation summary\n" + st.Summary,
		}},
	})
	out = append(out, tail...)
	return out
}
