package runtime

import (
	"fmt"

	"github.com/kasra-sh/zendawn-go/skill"
)

// ActivateResult is the outcome of loading/unloading a skill.
type ActivateResult struct {
	ID      string
	Guessed bool
	Query   string // original query when Guessed
	Unload  bool
}

// ActivateSkill resolves name against bound skills (with fuzzy guess) and load/unload.
func (r *Runtime) ActivateSkill(sessionID, name string, unload bool) (ActivateResult, error) {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return ActivateResult{}, NewError(CodeInternal, "session not found", false)
	}
	s.Lock()
	bound := append([]string{}, s.BoundSkills...)
	s.Unlock()

	id, guessed, err := skill.ResolveName(name, bound)
	if err != nil {
		return ActivateResult{}, NewError(CodeUnknownSkill, err.Error(), false)
	}
	packs := r.skillsForSession(sessionID)
	if packs == nil || packs[id] == nil {
		return ActivateResult{}, NewError(CodeUnknownSkill, "unknown_skill: "+id, false)
	}

	s.Lock()
	defer s.Unlock()
	if s.ActiveSkills == nil {
		s.ActiveSkills = map[string]bool{}
	}
	if unload {
		delete(s.ActiveSkills, id)
	} else {
		s.ActiveSkills[id] = true
	}
	return ActivateResult{ID: id, Guessed: guessed, Query: name, Unload: unload}, nil
}

// FormatActivateLine is a short system line for hosts.
func FormatActivateLine(ar ActivateResult) string {
	verb := "loaded"
	if ar.Unload {
		verb = "unloaded"
	}
	line := fmt.Sprintf("skill %s %s", verb, ar.ID)
	if ar.Guessed && ar.Query != ar.ID {
		line = fmt.Sprintf("skill: guessed %q → %q; %s", ar.Query, ar.ID, verb)
	}
	return line
}
