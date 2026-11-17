package provider

import (
	"encoding/json"
)

// ParseUsage extracts usage fields from a flexible OpenAI-compatible usage object
// (and optional top-level response map for context limit).
func ParseUsage(usageRaw json.RawMessage, topLevel map[string]any) Usage {
	var u Usage
	if len(usageRaw) > 0 && string(usageRaw) != "null" {
		var m map[string]any
		if err := json.Unmarshal(usageRaw, &m); err == nil {
			u.InputTokens = intFrom(m, "prompt_tokens", "input_tokens")
			u.OutputTokens = intFrom(m, "completion_tokens", "output_tokens")
			u.TotalTokens = intFrom(m, "total_tokens")
			if u.TotalTokens == 0 && (u.InputTokens > 0 || u.OutputTokens > 0) {
				u.TotalTokens = u.InputTokens + u.OutputTokens
			}
			if c, ok := floatFrom(m, "cost", "total_cost"); ok {
				u.CostUSD = c
				u.CostKnown = true
			}
			u.ContextLimit = intFrom(m, "context_length", "context_window", "max_tokens", "context_limit")
		}
	}
	if u.ContextLimit == 0 && topLevel != nil {
		u.ContextLimit = intFrom(topLevel, "context_length", "context_window", "context_limit")
		if u.ContextLimit == 0 {
			if mm, ok := topLevel["model"].(map[string]any); ok {
				u.ContextLimit = intFrom(mm, "context_length", "context_window")
			}
		}
	}
	return u
}

func intFrom(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			case json.Number:
				i, _ := n.Int64()
				return int(i)
			}
		}
	}
	return 0
}

func floatFrom(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return n, true
			case int:
				return float64(n), true
			case json.Number:
				f, err := n.Float64()
				if err == nil {
					return f, true
				}
			}
		}
	}
	return 0, false
}
