package runtime

import (
	"fmt"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/session"
)

// resolveSessionPolicy merges policy using the session's active provider/model id.
func (r *Runtime) resolveSessionPolicy(s *session.Session, agent *config.Agent) config.ResolvedPolicy {
	a := *agent
	s.Lock()
	if s.Provider != "" {
		a.Model.Provider = s.Provider
	}
	if s.ModelID != "" {
		a.Model.ID = s.ModelID
	}
	s.Unlock()
	return config.ResolvePolicy(r.Doc, &a)
}

// CycleModel advances the session to the next catalog model whose provider is registered.
// Wraps to the first entry. Returns the new provider and model id.
func (r *Runtime) CycleModel(sessionID string) (provName, modelID string, err error) {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return "", "", NewError(CodeInternal, "session not found", false)
	}
	catalog := r.Doc.Models
	if len(catalog) == 0 {
		// Fall back to current only.
		s.Lock()
		defer s.Unlock()
		return s.Provider, s.ModelID, NewError(CodeUnknownModel, "no models catalog to cycle", false)
	}

	// Build list of reachable entries.
	type entry struct{ p, id string }
	var reachable []entry
	for _, m := range catalog {
		if _, ok := r.Providers[m.Provider]; ok {
			reachable = append(reachable, entry{m.Provider, m.ID})
		}
	}
	if len(reachable) == 0 {
		return "", "", NewError(CodeUnknownModel, "no catalog models with registered providers", false)
	}

	s.Lock()
	curP, curID := s.Provider, s.ModelID
	s.Unlock()

	idx := 0
	for i, e := range reachable {
		if e.p == curP && e.id == curID {
			idx = (i + 1) % len(reachable)
			break
		}
	}
	// If current not in list, start at 0 (idx already 0).
	next := reachable[idx]
	s.Lock()
	s.Provider = next.p
	s.ModelID = next.id
	key := next.p + "/" + next.id
	if s.ContextLimits != nil {
		if lim, ok := s.ContextLimits[key]; ok {
			s.LastContextLimit = lim
		}
	}
	s.Unlock()

	// Do not emit here: CycleModel is often called from Bubble Tea Update;
	// a synchronous OnEvent→p.Send would deadlock. Hosts update UI from the return value.
	return next.p, next.id, nil
}

// applyUsage accumulates provider usage onto the session and emits EventUsage.
func (r *Runtime) applyUsage(s *session.Session, u provider.Usage) {
	s.Lock()
	s.UsageInput += u.InputTokens
	s.UsageOutput += u.OutputTokens
	if u.CostKnown {
		s.UsageCostUSD += u.CostUSD
		s.UsageCostKnown = true
	}
	if u.InputTokens > 0 {
		s.LastInputTokens = u.InputTokens
	}
	key := s.Provider + "/" + s.ModelID
	if u.ContextLimit > 0 {
		if s.ContextLimits == nil {
			s.ContextLimits = map[string]int{}
		}
		s.ContextLimits[key] = u.ContextLimit
		s.LastContextLimit = u.ContextLimit
	} else if s.ContextLimits != nil {
		if lim, ok := s.ContextLimits[key]; ok {
			s.LastContextLimit = lim
		}
	}
	snap := map[string]any{
		"inputTokens":      s.UsageInput,
		"outputTokens":     s.UsageOutput,
		"costUSD":          s.UsageCostUSD,
		"costKnown":        s.UsageCostKnown,
		"lastInputTokens":  s.LastInputTokens,
		"lastContextLimit": s.LastContextLimit,
		"provider":         s.Provider,
		"model":            s.ModelID,
		"stepInput":        u.InputTokens,
		"stepOutput":       u.OutputTokens,
	}
	s.Unlock()
	r.emit(NewEvent(EventUsage, s.ID, snap))
}

// ActiveModelLabel returns provider/id for display.
func ActiveModelLabel(providerName, modelID string) string {
	if providerName == "" {
		return modelID
	}
	return fmt.Sprintf("%s/%s", providerName, modelID)
}
