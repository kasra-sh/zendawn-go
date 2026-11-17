package config

import (
	"path/filepath"
	"strings"
)

// ResolvePolicy merges defaults → runtime → agent → matching profiles.
func ResolvePolicy(doc *Document, agent *Agent) ResolvedPolicy {
	p := defaults()

	if doc != nil && doc.Runtime != nil {
		applyRuntime(&p, doc.Runtime)
	}
	applyAgent(&p, agent)

	if doc != nil && (len(doc.Models) > 0 || len(doc.ModelProfiles) > 0) {
		entry := findCatalog(doc.Models, agent.Model.Provider, agent.Model.ID)
		for _, prof := range doc.ModelProfiles {
			if profileMatches(prof.Match, agent.Model, entry) {
				applyClosed(&p, prof.Policy)
			}
		}
	}

	// Sampling params: profile-merged params, then agent params win.
	params := map[string]any{}
	for k, v := range p.Params {
		params[k] = v
	}
	for k, v := range agent.Model.Params {
		params[k] = v
	}
	p.Params = params
	return p
}

func defaults() ResolvedPolicy {
	return ResolvedPolicy{
		MaxSteps:           25,
		ToolParallelism:    "sequential",
		Stream:             true,
		ToolTimeoutMs:      30000,
		AskTimeoutMs:       0,
		AskTimeoutDecision: "reject",
		MaxSubagentDepth:   2,
		CompactionMode:     "none",
		CompactionLaxRatio: 0.8,
		CompactionMinChars: 4000,
		Params:             map[string]any{},
		Retries: RetriesResolved{
			MaxAttempts:       3,
			BackoffMs:         500,
			BackoffMultiplier: 2.0,
			RetryableCodes:    []string{"rate_limit", "timeout", "unavailable", "overloaded"},
		},
	}
}

func applyRuntime(p *ResolvedPolicy, r *RuntimeConfig) {
	if r.ToolTimeoutMs != nil {
		p.ToolTimeoutMs = *r.ToolTimeoutMs
	}
	if r.MaxSubagentDepth != nil {
		p.MaxSubagentDepth = *r.MaxSubagentDepth
	}
	if r.AskTimeoutMs != nil {
		p.AskTimeoutMs = *r.AskTimeoutMs
	}
	if r.AskTimeoutDecision != "" {
		p.AskTimeoutDecision = r.AskTimeoutDecision
	}
	if r.Retries != nil {
		applyRetries(&p.Retries, r.Retries)
	}
	if r.Hooks != nil {
		p.IgnoreHookErrors = r.Hooks.IgnoreErrors
	}
	if r.CompactionMode != "" {
		p.CompactionMode = r.CompactionMode
	}
	if r.Compaction != nil {
		if r.Compaction.LaxThreshold != nil {
			p.CompactionLaxRatio = *r.Compaction.LaxThreshold
		}
		if r.Compaction.MinChars != nil {
			p.CompactionMinChars = *r.Compaction.MinChars
		}
	}
}

func applyAgent(p *ResolvedPolicy, a *Agent) {
	if a.MaxSteps != nil {
		p.MaxSteps = *a.MaxSteps
	}
	if a.ToolParallelism != "" {
		p.ToolParallelism = a.ToolParallelism
	}
	if a.Stream != nil {
		p.Stream = *a.Stream
	}
	if a.ToolTimeoutMs != nil {
		p.ToolTimeoutMs = *a.ToolTimeoutMs
	}
	if a.AskTimeoutMs != nil {
		p.AskTimeoutMs = *a.AskTimeoutMs
	}
	if a.AskTimeoutDecision != "" {
		p.AskTimeoutDecision = a.AskTimeoutDecision
	}
	if a.Retries != nil {
		applyRetries(&p.Retries, a.Retries)
	}
	if a.MaxSubagentDepth != nil {
		p.MaxSubagentDepth = *a.MaxSubagentDepth
	}
	if a.CompactionMode != "" {
		p.CompactionMode = a.CompactionMode
	}
}

func applyClosed(p *ResolvedPolicy, c ClosedPolicy) {
	if c.MaxSteps != nil {
		p.MaxSteps = *c.MaxSteps
	}
	if c.ToolParallelism != "" {
		p.ToolParallelism = c.ToolParallelism
	}
	if c.Stream != nil {
		p.Stream = *c.Stream
	}
	if c.ToolTimeoutMs != nil {
		p.ToolTimeoutMs = *c.ToolTimeoutMs
	}
	if c.AskTimeoutMs != nil {
		p.AskTimeoutMs = *c.AskTimeoutMs
	}
	if c.AskTimeoutDecision != "" {
		p.AskTimeoutDecision = c.AskTimeoutDecision
	}
	if c.Retries != nil {
		applyRetries(&p.Retries, c.Retries)
	}
	if c.MaxSubagentDepth != nil {
		p.MaxSubagentDepth = *c.MaxSubagentDepth
	}
	if c.CompactionMode != "" {
		p.CompactionMode = c.CompactionMode
	}
	if c.Params != nil {
		if p.Params == nil {
			p.Params = map[string]any{}
		}
		for k, v := range c.Params {
			p.Params[k] = v
		}
	}
}

func applyRetries(dst *RetriesResolved, src *RetriesConfig) {
	if src.MaxAttempts != nil {
		dst.MaxAttempts = *src.MaxAttempts
	}
	if src.BackoffMs != nil {
		dst.BackoffMs = *src.BackoffMs
	}
	if src.BackoffMultiplier != nil {
		dst.BackoffMultiplier = *src.BackoffMultiplier
	}
	if src.RetryableCodes != nil {
		dst.RetryableCodes = append([]string{}, src.RetryableCodes...)
	}
}

func findCatalog(models []ModelCatalogEntry, provider, id string) *ModelCatalogEntry {
	for i := range models {
		if models[i].Provider == provider && models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}

func profileMatches(m ModelProfileMatch, model ModelConfig, entry *ModelCatalogEntry) bool {
	if m.Provider != "" && m.Provider != model.Provider {
		return false
	}
	if m.Model != "" && !globMatch(m.Model, model.ID) {
		return false
	}
	if m.Class != "" {
		if entry == nil || entry.Class != m.Class {
			return false
		}
	}
	if m.SizeClass != "" {
		if entry == nil || entry.SizeClass != m.SizeClass {
			return false
		}
	}
	return true
}

func globMatch(pattern, value string) bool {
	p := strings.ReplaceAll(pattern, "**", "*")
	ok, err := filepath.Match(p, value)
	if err == nil && ok {
		return true
	}
	// star spanning
	return matchStar(p, value)
}

func matchStar(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	i, j := 0, 0
	star, match := -1, 0
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
