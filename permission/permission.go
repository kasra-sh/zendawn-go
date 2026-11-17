package permission

import (
	"path/filepath"
	"strings"

	"github.com/kasra-sh/zendawn-go/tool"
)

// Effect values.
const (
	EffectAllow = "allow"
	EffectAsk   = "ask"
	EffectDeny  = "deny"
)

// Rule is an ordered permission rule.
type Rule struct {
	Action   string `json:"action" yaml:"action"`
	Resource string `json:"resource" yaml:"resource"`
	Effect   string `json:"effect" yaml:"effect"`
}

// Assertion is evaluated against rules.
type Assertion struct {
	Action     string
	Resource   string
	ToolName   string
	ToolCallID string
}

// Resolve returns the effect for an assertion.
// Rules are agent rules then overlays; last match wins; no match → ask.
// Invalid matched effects fail closed to deny. Resources are filepath.Clean'd before match.
// For MCP tools, a winning catch-all action "*" is coerced to ask.
func Resolve(agentRules, overlays []Rule, a Assertion) string {
	a.Resource = filepath.Clean(a.Resource)

	effect := EffectAsk
	winningAction := ""
	matched := false
	apply := func(rules []Rule) {
		for _, r := range rules {
			if Match(r.Action, a.Action) && Match(r.Resource, a.Resource) {
				matched = true
				winningAction = r.Action
				switch r.Effect {
				case EffectAllow, EffectAsk, EffectDeny:
					effect = r.Effect
				default:
					effect = EffectDeny
				}
			}
		}
	}
	apply(agentRules)
	apply(overlays)

	if matched && isMCPAssertion(a) && winningAction == "*" {
		return EffectAsk
	}
	return effect
}

func isMCPAssertion(a Assertion) bool {
	if tool.IsMCPToolName(a.ToolName) {
		return true
	}
	// Fallback when ToolName omitted: MCP actions are the registered names.
	return tool.IsMCPToolName(a.Action)
}

// Match implements glob-style matching: *, ?, and ** as any sequence.
// Per specs/06-permissions.md, ** is the same as * spanning '/'.
func Match(pattern, value string) bool {
	if pattern == "*" || pattern == "**" {
		return true
	}
	p := strings.ReplaceAll(pattern, "**", "*")
	return matchStar([]rune(p), []rune(value))
}

func matchStar(pattern, value []rune) bool {
	if len(pattern) == 1 && pattern[0] == '*' {
		return true
	}
	i, j := 0, 0
	star := -1
	match := 0
	for j < len(value) {
		if i < len(pattern) && (pattern[i] == '?' || pattern[i] == value[j]) {
			i++
			j++
			continue
		}
		if i < len(pattern) && pattern[i] == '*' {
			star = i
			match = j
			i++
			continue
		}
		if star >= 0 {
			i = star + 1
			match++
			j = match
			continue
		}
		return false
	}
	for i < len(pattern) && pattern[i] == '*' {
		i++
	}
	return i == len(pattern)
}
