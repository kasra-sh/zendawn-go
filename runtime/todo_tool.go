package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kasra-sh/zendawn-go/paths"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/skill"
	"github.com/kasra-sh/zendawn-go/tool"
)

func (r *Runtime) registerTodoTool() {
	r.Tools.Register(tool.Definition{
		Name:        "todo",
		Description: "List, complete, or get the next plan todo (session plan app-context).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []any{"list", "complete", "next"},
				},
				"id": map[string]any{"type": "string", "description": "Todo id for complete"},
			},
			"required":             []any{"action"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "todo", ResourceFrom: "action"},
	}, r.execTodo)
}

func (r *Runtime) execTodo(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
	action, _ := args["action"].(string)
	id, _ := args["id"].(string)
	sessionID, _ := ctx.Value(ctxKeySessionID).(string)
	s, ok := r.GetSession(sessionID)
	if !ok {
		return tool.TextResult("no session", true, CodeInternal), nil
	}

	s.Lock()
	mode := s.Mode
	s.Unlock()
	if mode == session.ModePlan && action != "list" {
		return tool.TextResult("in plan mode only todo list is allowed", true, CodePermissionDenied), nil
	}

	switch action {
	case "list":
		s.Lock()
		todos := append([]session.PlanTodo{}, s.PlanTodos...)
		s.Unlock()
		b, _ := json.Marshal(todos)
		return tool.TextResult(string(b), false, ""), nil
	case "next":
		s.Lock()
		var next *session.PlanTodo
		for i := range s.PlanTodos {
			if !s.PlanTodos[i].Done {
				t := s.PlanTodos[i]
				next = &t
				break
			}
		}
		s.Unlock()
		if next == nil {
			return tool.TextResult("all plan todos complete", false, ""), nil
		}
		return tool.TextResult(fmt.Sprintf("id=%s — %s", next.ID, next.Content), false, ""), nil
	case "complete":
		if strings.TrimSpace(id) == "" {
			return tool.TextResult("id required", true, CodeToolInvalidArgs), nil
		}
		s.Lock()
		ids := make([]string, 0, len(s.PlanTodos))
		for _, t := range s.PlanTodos {
			ids = append(ids, t.ID)
		}
		s.Unlock()
		resolved, guessed, err := skill.ResolveName(id, ids)
		if err != nil {
			return tool.TextResult(err.Error(), true, CodeInvalidPlan), nil
		}
		s.Lock()
		found := false
		allDone := true
		for i := range s.PlanTodos {
			if s.PlanTodos[i].ID == resolved {
				s.PlanTodos[i].Done = true
				found = true
			}
			if !s.PlanTodos[i].Done {
				allDone = false
			}
		}
		s.Unlock()
		if !found {
			return tool.TextResult("unknown todo "+id, true, CodeInvalidPlan), nil
		}
		body := rewriteCurrentPlanFile(s)
		_ = paths.WritePlan(s.ID, body)
		msg := fmt.Sprintf("completed %s", resolved)
		if guessed {
			msg = fmt.Sprintf("guessed %q → %q; completed", id, resolved)
		}
		if allDone {
			msg += "; all plan todos complete"
		}
		return tool.TextResult(msg, false, ""), nil
	default:
		return tool.TextResult("unknown action", true, CodeToolInvalidArgs), nil
	}
}
