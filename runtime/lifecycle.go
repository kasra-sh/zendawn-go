package runtime

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/hook"
	"github.com/kasra-sh/zendawn-go/paths"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/skill"
)

func (r *Runtime) skillDirsFor(agent *config.Agent) []string {
	var dirs []string
	dirs = append(dirs, r.Doc.SkillsDirs...)
	dirs = append(dirs, agent.SkillsDirs...)
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if filepath.IsAbs(d) {
			out = append(out, d)
			continue
		}
		base := r.ConfigDir
		if base == "" {
			base = r.WorkDir
		}
		out = append(out, filepath.Join(base, d))
	}
	return out
}

func (r *Runtime) loadAgentSkills(agent *config.Agent) (map[string]*skill.Pack, []string, error) {
	return skill.LoadBound(r.WorkDir, r.skillDirsFor(agent), agent.Skills)
}

func (r *Runtime) mergeInstructions(s *session.Session, agent *config.Agent, mode string) string {
	var b strings.Builder
	b.WriteString(agent.Instructions)
	s.Lock()
	agentsMD := s.AgentsMD
	rules := s.RulesText
	bound := append([]string{}, s.BoundSkills...)
	active := map[string]bool{}
	for k, v := range s.ActiveSkills {
		active[k] = v
	}
	s.Unlock()
	if agentsMD != "" {
		b.WriteString("\n\n[AGENTS.md]\n")
		b.WriteString(agentsMD)
	}
	if rules != "" {
		b.WriteString("\n\n[local rules]\n")
		b.WriteString(rules)
	}
	b.WriteString(ModeInstructionSuffix(mode))
	wd := s.WorkDir
	if wd == "" {
		wd = r.WorkDir
	}
	if wd != "" {
		b.WriteString("\n\n[workspace] Working directory: ")
		b.WriteString(wd)
		b.WriteString(". Do not run pwd/cwd shell commands to discover this.")
	}
	appendPlanContext(&b, s, mode)
	packs := r.skillsForSession(s.ID)
	if packs != nil && len(bound) > 0 {
		b.WriteString(skill.CatalogBlock(packs, bound))
		b.WriteString(skill.BodiesBlock(packs, bound, active))
	}
	b.WriteString(MCPToolsHint(r.Tools))
	return b.String()
}

func (r *Runtime) skillsForSession(sessionID string) map[string]*skill.Pack {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionSkills[sessionID]
}

func applyModeState(s *session.Session, mode string) {
	mode = NormalizeMode(mode)
	s.Mode = mode
	s.ModeRules = ModePermissionRules(mode)
}

func (r *Runtime) enterModeHooks(ctx context.Context, agent *config.Agent, s *session.Session, mode string, ignore bool) error {
	mode = NormalizeMode(mode)
	policy := config.ResolvePolicy(r.Doc, agent)
	data := map[string]any{
		"mode":         mode,
		"instructions": agent.Instructions + ModeInstructionSuffix(mode),
		"params":       policy.Params,
	}
	switch mode {
	case session.ModePlan:
		return r.invokeHooks(ctx, agent, hook.BeforePlan, s, 0, data, ignore)
	case session.ModeAgent:
		return r.invokeHooks(ctx, agent, hook.BeforeExecution, s, 0, data, ignore)
	default:
		return nil // ask: no phase hook
	}
}

// SetMode switches ask|plan|agent when idle and fires phase hooks.
func (r *Runtime) SetMode(ctx context.Context, sessionID, mode string) error {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return NewError(CodeInternal, "session not found", false)
	}
	s.Lock()
	if s.Busy {
		s.Unlock()
		return NewError(CodeInternal, "cannot change mode while busy", false)
	}
	prev := s.Mode
	if prev == "" {
		prev = session.ModeAgent
	}
	agentID := s.ActiveAgent
	s.Unlock()

	mode = NormalizeMode(mode)
	if prev == mode {
		return nil
	}

	agent, err := r.Doc.Agent(agentID)
	if err != nil {
		return err
	}
	policy := config.ResolvePolicy(r.Doc, agent)

	leave := func(m string) error {
		switch m {
		case session.ModePlan:
			r.finalizePlanBody(s)
			if err := r.invokeHooks(ctx, agent, hook.AfterPlan, s, 0, map[string]any{"mode": m}, policy.IgnoreHookErrors); err != nil {
				return err
			}
			// Leaving plan → agent: commit when todos exist; otherwise abandon so Tab is not stuck.
			if mode == session.ModeAgent {
				if planBufferHasTodos(s) {
					if err := commitPlanFromBuffer(s, s.ID); err != nil {
						return NewError(CodeInvalidPlan, err.Error(), false)
					}
					_ = paths.WritePlan(s.ID, s.PlanMarkdown)
				} else {
					abandonPlanBuffer(s)
				}
			} else {
				_ = paths.WritePlan(s.ID, s.PlanMarkdown)
			}
		case session.ModeAgent:
			return r.invokeHooks(ctx, agent, hook.AfterExecution, s, 0, map[string]any{"mode": m}, policy.IgnoreHookErrors)
		}
		return nil
	}
	enter := func(m string) error {
		data := map[string]any{
			"mode":         m,
			"instructions": agent.Instructions + ModeInstructionSuffix(m),
			"params":       policy.Params,
		}
		switch m {
		case session.ModePlan:
			if err := r.invokeHooks(ctx, agent, hook.BeforePlan, s, 0, data, policy.IgnoreHookErrors); err != nil {
				return err
			}
			s.Lock()
			has := sessionHasExistingPlan(s)
			s.Unlock()
			if has {
				s.Lock()
				s.PendingPlanAsk = true
				s.Unlock()
				r.emit(NewEvent(EventPlanAsk, s.ID, map[string]any{
					"choices": []string{"edit", "new"},
				}))
			}
		case session.ModeAgent:
			if err := r.invokeHooks(ctx, agent, hook.BeforeExecution, s, 0, data, policy.IgnoreHookErrors); err != nil {
				return err
			}
		}
		return nil
	}

	if err := leave(prev); err != nil {
		if e, ok := err.(*Error); ok {
			return e
		}
		return NewError(CodeHookError, err.Error(), false)
	}
	s.Lock()
	applyModeState(s, mode)
	s.Unlock()
	if err := enter(mode); err != nil {
		return NewError(CodeHookError, err.Error(), false)
	}
	return nil
}

// ResolvePlanAsk answers edit vs new when PendingPlanAsk is set.
func (r *Runtime) ResolvePlanAsk(sessionID string, d session.PlanAskDecision) error {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return NewError(CodeInternal, "session not found", false)
	}
	s.Lock()
	if !s.PendingPlanAsk {
		s.Unlock()
		return nil
	}
	s.PendingPlanAsk = false
	s.Unlock()

	switch d {
	case session.PlanAskNew:
		archiveAndClearPlan(s)
	case session.PlanAskEdit:
		// keep buffer
	default:
		return NewError(CodeInvalidRequest, "plan ask must be edit|new", false)
	}
	r.emit(NewEvent(EventPlanResolved, s.ID, map[string]any{"decision": string(d)}))
	select {
	case s.PlanAskCh <- d:
	default:
	}
	return nil
}

func (r *Runtime) finalizePlanBody(s *session.Session) {
	s.Lock()
	defer s.Unlock()
	if strings.TrimSpace(s.PlanMarkdown) != "" {
		return
	}
	for i := len(s.Transcript) - 1; i >= 0; i-- {
		it := s.Transcript[i]
		if it.Type != session.ItemMessage || it.Role != session.RoleAssistant {
			continue
		}
		var b strings.Builder
		for _, p := range it.Content {
			if p.Type == session.PartText {
				b.WriteString(p.Text)
			}
		}
		if t := strings.TrimSpace(b.String()); t != "" {
			s.PlanMarkdown = t
			return
		}
	}
}

// Close runs leave-phase + after_session and removes the session.
func (r *Runtime) Close(ctx context.Context, sessionID string) error {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return NewError(CodeInternal, "session not found", false)
	}
	s.Lock()
	if s.Busy {
		s.Unlock()
		return NewError(CodeInternal, "cannot close busy session", false)
	}
	mode := s.Mode
	agentID := s.ActiveAgent
	s.Unlock()

	agent, err := r.Doc.Agent(agentID)
	if err != nil {
		return err
	}
	policy := config.ResolvePolicy(r.Doc, agent)

	switch mode {
	case session.ModePlan:
		r.finalizePlanBody(s)
		_ = r.invokeHooks(ctx, agent, hook.AfterPlan, s, 0, map[string]any{"mode": mode}, policy.IgnoreHookErrors)
		_ = paths.WritePlan(s.ID, s.PlanMarkdown)
	case session.ModeAgent, "":
		_ = r.invokeHooks(ctx, agent, hook.AfterExecution, s, 0, map[string]any{"mode": session.ModeAgent}, policy.IgnoreHookErrors)
	}
	_ = r.invokeHooks(ctx, agent, hook.AfterSession, s, 0, nil, policy.IgnoreHookErrors)

	r.mu.Lock()
	delete(r.sessions, sessionID)
	delete(r.sessionSkills, sessionID)
	delete(r.attachments, sessionID)
	r.mu.Unlock()
	return nil
}
