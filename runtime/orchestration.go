package runtime

import (
	"context"
	"fmt"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/skill"
	"github.com/kasra-sh/zendawn-go/tool"
)

func (r *Runtime) registerBuiltins() {
	r.Tools.Register(tool.Definition{
		Name:        "task",
		Description: "Run an allowlisted sub-agent in a child session and return its final report.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agentId": map[string]any{"type": "string"},
				"prompt":  map[string]any{"type": "string"},
			},
			"required":             []any{"agentId", "prompt"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "subagent", ResourceFrom: "agentId"},
	}, r.execTask)

	r.Tools.Register(tool.Definition{
		Name:        "handoff",
		Description: "Transfer control to an allowlisted agent in this same session.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agentId": map[string]any{"type": "string"},
				"reason":  map[string]any{"type": "string"},
			},
			"required":             []any{"agentId", "reason"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "handoff", ResourceFrom: "agentId"},
	}, r.execHandoff)

	r.Tools.Register(tool.Definition{
		Name: "skill",
		Description: "Load or unload a bound skill into the session context. " +
			"Call load when the user asks to use a skill, names a skill, or uses a name similar to a bound skill id. " +
			"Also call load proactively when a catalog skill marked [autoInvoke] would clearly help the request even if the user did not name it. " +
			"Do not proactive-load skills marked [manual]; only load those when the user clearly requests them.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "Skill id (exact or close match)"},
				"action": map[string]any{"type": "string", "enum": []any{"load", "unload"}, "description": "Default load"},
			},
			"required":             []any{"name"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "skill", ResourceFrom: "name"},
	}, r.execSkill)

	r.registerTodoTool()
}

func (r *Runtime) execSkill(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
	name, _ := args["name"].(string)
	action, _ := args["action"].(string)
	if action == "" {
		action = "load"
	}
	sessionID, _ := ctx.Value(ctxKeySessionID).(string)
	ar, err := r.ActivateSkill(sessionID, name, action == "unload")
	if err != nil {
		return tool.JSONResult("unknown_skill", map[string]any{"error": CodeUnknownSkill, "name": name, "detail": err.Error()}, true, CodeUnknownSkill), nil
	}
	return tool.TextResult(FormatActivateLine(ar), false, ""), nil
}

func (r *Runtime) execTask(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
	agentID, _ := args["agentId"].(string)
	prompt, _ := args["prompt"].(string)

	parentID, _ := ctx.Value(ctxKeySessionID).(string)
	parent, ok := r.GetSession(parentID)
	if !ok {
		return tool.TextResult("no parent session", true, CodeInternal), nil
	}

	parentAgent, err := r.Doc.Agent(parent.ActiveAgent)
	if err != nil {
		return tool.TextResult(err.Error(), true, CodeUnknownAgent), nil
	}
	allowed := false
	for _, id := range parentAgent.Subagents {
		if id == agentID {
			allowed = true
			break
		}
	}
	if !allowed {
		return tool.JSONResult("unknown_agent", map[string]any{"error": CodeUnknownAgent, "agentId": agentID}, true, CodeUnknownAgent), nil
	}
	if _, err := r.Doc.Agent(agentID); err != nil {
		return tool.JSONResult("unknown_agent", map[string]any{"error": CodeUnknownAgent, "agentId": agentID}, true, CodeUnknownAgent), nil
	}

	policy := config.ResolvePolicy(r.Doc, parentAgent)
	if parent.Depth+1 > policy.MaxSubagentDepth {
		return tool.JSONResult("max_depth", map[string]any{"error": CodeMaxDepth}, true, CodeMaxDepth), nil
	}

	childAgent, _ := r.Doc.Agent(agentID)
	childID := r.nextID("ses")
	child := session.New(childID, childAgent.ID)
	child.Provider = childAgent.Model.Provider
	child.ModelID = childAgent.Model.ID
	child.Depth = parent.Depth + 1
	child.ParentID = parent.ID
	child.WorkDir = r.WorkDir
	applyModeState(child, session.ModeAgent)
	childPacks, childBound, err := r.loadAgentSkills(childAgent)
	if err != nil {
		return tool.TextResult(err.Error(), true, CodeUnknownSkill), nil
	}
	wc := skill.LoadWorkspaceContext(r.WorkDir)
	child.BoundSkills = childBound
	child.AgentsMD = wc.AgentsMD
	child.RulesText = wc.RulesText
	r.mu.Lock()
	r.sessions[childID] = child
	r.sessionSkills[childID] = childPacks
	r.mu.Unlock()

	r.emit(NewEvent(EventSubagentStarted, parent.ID, map[string]any{
		"parentSessionId": parent.ID,
		"childSessionId":  childID,
		"agentId":         agentID,
	}))

	child.Lock()
	child.Busy = true
	child.AppendItem(session.Item{
		ID: r.nextID("msg"), Type: session.ItemMessage, Role: session.RoleUser,
		Content: []session.ContentPart{{Type: session.PartText, Text: prompt}},
	})
	child.Unlock()

	childCtx := context.WithValue(ctx, ctxKeySessionID, childID)
	reason := r.drain(childCtx, child)

	r.emit(NewEvent(EventSubagentCompleted, parent.ID, map[string]any{
		"childSessionId": childID,
		"stopReason":     reason,
	}))

	lastText := lastAssistantText(child)
	isErr := reason == session.StopError || reason == session.StopAborted
	return tool.JSONResult(lastText, map[string]any{
		"stopReason":     reason,
		"childSessionId": childID,
		"agentId":        agentID,
	}, isErr, ""), nil
}

func (r *Runtime) execHandoff(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
	agentID, _ := args["agentId"].(string)
	reason, _ := args["reason"].(string)

	sessionID, _ := ctx.Value(ctxKeySessionID).(string)
	s, ok := r.GetSession(sessionID)
	if !ok {
		return tool.TextResult("no session", true, CodeInternal), nil
	}

	cur, err := r.Doc.Agent(s.ActiveAgent)
	if err != nil {
		return tool.TextResult(err.Error(), true, CodeUnknownAgent), nil
	}
	allowed := false
	for _, id := range cur.Handoffs {
		if id == agentID {
			allowed = true
			break
		}
	}
	if !allowed {
		return tool.JSONResult("unknown_agent", map[string]any{"error": CodeUnknownAgent, "agentId": agentID}, true, CodeUnknownAgent), nil
	}
	if _, err := r.Doc.Agent(agentID); err != nil {
		return tool.JSONResult("unknown_agent", map[string]any{"error": CodeUnknownAgent, "agentId": agentID}, true, CodeUnknownAgent), nil
	}

	from := s.ActiveAgent
	s.Lock()
	s.ActiveAgent = agentID
	target, _ := r.Doc.Agent(agentID)
	s.Provider = target.Model.Provider
	s.ModelID = target.Model.ID
	s.Unlock()

	r.emit(NewEvent(EventHandoffCompleted, s.ID, map[string]any{
		"from": from, "to": agentID, "reason": reason,
	}))

	return tool.JSONResult(
		fmt.Sprintf("Handed off from %s to %s", from, agentID),
		map[string]any{"from": from, "to": agentID, "reason": reason},
		false, "",
	), nil
}

func lastAssistantText(s *session.Session) string {
	s.Lock()
	defer s.Unlock()
	for i := len(s.Transcript) - 1; i >= 0; i-- {
		it := s.Transcript[i]
		if it.Type == session.ItemMessage && it.Role == session.RoleAssistant {
			return session.TextParts(it.Content)
		}
	}
	return ""
}

type ctxKey string

const ctxKeySessionID ctxKey = "sessionID"
