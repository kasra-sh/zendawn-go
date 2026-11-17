package provider

import (
	"encoding/json"
	"testing"
)

func TestParseUsageTokensAndCost(t *testing.T) {
	raw := json.RawMessage(`{"prompt_tokens":120,"completion_tokens":40,"total_tokens":160,"cost":0.042}`)
	u := ParseUsage(raw, nil)
	if u.InputTokens != 120 || u.OutputTokens != 40 || u.TotalTokens != 160 {
		t.Fatalf("tokens: %+v", u)
	}
	if !u.CostKnown || u.CostUSD != 0.042 {
		t.Fatalf("cost: %+v", u)
	}
}

func TestParseUsageContextLimit(t *testing.T) {
	raw := json.RawMessage(`{"prompt_tokens":25000,"completion_tokens":1,"context_length":200000}`)
	u := ParseUsage(raw, nil)
	if u.ContextLimit != 200000 || u.InputTokens != 25000 {
		t.Fatalf("context: %+v", u)
	}
	top := map[string]any{"context_window": float64(128000)}
	u2 := ParseUsage(json.RawMessage(`{"prompt_tokens":10}`), top)
	if u2.ContextLimit != 128000 {
		t.Fatalf("top-level context: %+v", u2)
	}
}

func TestParseUsageMissingCost(t *testing.T) {
	u := ParseUsage(json.RawMessage(`{"prompt_tokens":1,"completion_tokens":2}`), nil)
	if u.CostKnown {
		t.Fatal("cost should be unknown")
	}
}
