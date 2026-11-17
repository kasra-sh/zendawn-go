package runtime

import (
	"strings"

	"github.com/kasra-sh/zendawn-go/permission"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/tool"
)

// NormalizeMode returns ask|plan|agent; empty/unknown → agent.
func NormalizeMode(mode string) string {
	switch mode {
	case session.ModeAsk, session.ModePlan, session.ModeAgent:
		return mode
	default:
		return session.ModeAgent
	}
}

// ModeAllowlist returns tools allowed in a mode (empty = no filter for agent).
// MCP tools are added separately via FilterToolsByMode from the registry.
func ModeAllowlist(mode string) []string {
	switch mode {
	case session.ModeAsk:
		return []string{"read", "glob", "grep", "skill"}
	case session.ModePlan:
		return []string{"read", "glob", "grep", "shell", "skill", "todo"}
	default:
		return nil // agent: no extra filter
	}
}

// ModePermissionRules returns rules appended after session overlays.
func ModePermissionRules(mode string) []permission.Rule {
	switch mode {
	case session.ModeAsk:
		return []permission.Rule{
			{Action: "shell", Resource: "*", Effect: permission.EffectDeny},
			{Action: "edit", Resource: "*", Effect: permission.EffectDeny},
			{Action: "subagent", Resource: "*", Effect: permission.EffectDeny},
			{Action: "handoff", Resource: "*", Effect: permission.EffectDeny},
			{Action: "todo", Resource: "*", Effect: permission.EffectDeny},
		}
	case session.ModePlan:
		return []permission.Rule{
			{Action: "edit", Resource: "*", Effect: permission.EffectDeny},
			{Action: "shell", Resource: "*", Effect: permission.EffectAsk},
			{Action: "subagent", Resource: "*", Effect: permission.EffectDeny},
			{Action: "handoff", Resource: "*", Effect: permission.EffectDeny},
			{Action: "todo", Resource: "*", Effect: permission.EffectAllow},
		}
	default:
		return nil
	}
}

// ModeInstructionSuffix is appended to agent instructions.
func ModeInstructionSuffix(mode string) string {
	switch mode {
	case session.ModeAsk:
		return "\n\n[mode: ask] Answer using read-only tools only. Do not propose file edits or shell mutations."
	case session.ModePlan:
		return "\n\n[mode: plan] Produce a concrete plan. You may inspect the repo. Do not edit files. " +
			"The plan MUST end with a ## Todos checklist using lines like `- [ ] id: short-id — description` " +
			"(YAML frontmatter todos are also accepted and will be normalized). At least one todo is required before execution."
	default:
		return "\n\n[mode: agent] You may use tools and edit files when they help; await approval for shell/edits. " +
			"Prefer answering in chat when tools are unnecessary (greetings, clarifications, or content the user can read in-reply). " +
			"If the prompt is ambiguous or underspecified, ask a short clarifying question in chat instead of guessing via tools or shell. " +
			"Do not run shell just to discover cwd; use the provided working directory. " +
			"Do not probe the repo unless the task clearly needs it. " +
			"Break work by plan todos; call todo complete when each is finished."
	}
}

// SetMode updates session mode and mode permission rules without hooks.
// Prefer Runtime.SetMode so phase hooks and plan persistence run.
func SetMode(s *session.Session, mode string) {
	s.Lock()
	applyModeState(s, mode)
	s.Unlock()
}

// MCPToolsHint is injected into instructions so the model calls MCP tools by name.
func MCPToolsHint(reg *tool.Registry) string {
	names := tool.MCPNames(reg)
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[MCP tools] Call these tools directly by name for external services. ")
	b.WriteString("Do not use shell, and do not load a skill, to access them:\n")
	for _, n := range names {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteString("\n")
	}
	return b.String()
}

// FilterToolsByMode intersects names with the mode allowlist.
// In ask/plan, always appends registered MCP tools (injected like builtins).
func FilterToolsByMode(mode string, names []string, toolsAll bool, reg *tool.Registry) ([]string, bool) {
	allow := ModeAllowlist(mode)
	if allow == nil {
		return names, toolsAll
	}
	mcp := tool.MCPNames(reg)
	if toolsAll {
		out := append([]string{}, allow...)
		out = append(out, mcp...)
		return out, false
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, a := range allow {
		if want[a] {
			out = append(out, a)
			seen[a] = true
		}
	}
	for _, n := range mcp {
		if !seen[n] {
			out = append(out, n)
			seen[n] = true
		}
	}
	return out, false
}
