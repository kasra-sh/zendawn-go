package runtime

import (
	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider/openrouter"
)

// ModelSpec is runtime metadata for a catalog entry (not persisted to YAML).
type ModelSpec = openrouter.Spec

// MergeOpenRouterCatalog builds a catalog with API models first (API order),
// then YAML-only entries. Overlapping IDs keep YAML class/sizeClass tags.
// providerID is the registry provider name (e.g. "openai"), not the OpenRouter slug.
func MergeOpenRouterCatalog(yamlModels []config.ModelCatalogEntry, apiModels []openrouter.Model, providerID string) (
	merged []config.ModelCatalogEntry,
	specs map[string]ModelSpec,
) {
	yamlByID := map[string]config.ModelCatalogEntry{}
	for _, m := range yamlModels {
		if m.Provider == providerID {
			yamlByID[m.ID] = m
		}
	}

	specs = make(map[string]ModelSpec, len(apiModels))
	seen := make(map[string]bool, len(apiModels))
	merged = make([]config.ModelCatalogEntry, 0, len(apiModels)+len(yamlModels))

	for _, am := range apiModels {
		if am.ID == "" || seen[am.ID] {
			continue
		}
		entry := config.ModelCatalogEntry{Provider: providerID, ID: am.ID}
		if y, ok := yamlByID[am.ID]; ok {
			entry.Class = y.Class
			entry.SizeClass = y.SizeClass
		}
		merged = append(merged, entry)
		seen[am.ID] = true
		key := providerID + "/" + am.ID
		specs[key] = openrouter.SpecFromModel(am)
	}

	for _, ym := range yamlModels {
		if seen[ym.ID] {
			continue
		}
		merged = append(merged, ym)
		seen[ym.ID] = true
	}
	return merged, specs
}

// ApplyModelsCatalog replaces Doc.Models, stores specs, and sets the session
// to the first catalog entry. Seeds ContextLimits from specs when available.
func (r *Runtime) ApplyModelsCatalog(sessionID string, merged []config.ModelCatalogEntry, specs map[string]ModelSpec) (provName, modelID string, err error) {
	if len(merged) == 0 {
		return "", "", NewError(CodeUnknownModel, "empty models catalog", false)
	}
	s, ok := r.GetSession(sessionID)
	if !ok {
		return "", "", NewError(CodeInternal, "session not found", false)
	}

	r.mu.Lock()
	r.Doc.Models = merged
	r.modelSpecs = specs
	r.mu.Unlock()

	first := merged[0]
	s.Lock()
	s.Provider = first.Provider
	s.ModelID = first.ID
	key := first.Provider + "/" + first.ID
	if specs != nil {
		if sp, ok := specs[key]; ok && sp.ContextLength > 0 {
			if s.ContextLimits == nil {
				s.ContextLimits = map[string]int{}
			}
			s.ContextLimits[key] = sp.ContextLength
			s.LastContextLimit = sp.ContextLength
		}
	}
	// Also seed limits for other API models.
	if specs != nil {
		if s.ContextLimits == nil {
			s.ContextLimits = map[string]int{}
		}
		for k, sp := range specs {
			if sp.ContextLength > 0 {
				s.ContextLimits[k] = sp.ContextLength
			}
		}
	}
	s.Unlock()
	return first.Provider, first.ID, nil
}

// ModelSpecFor returns stored runtime specs for provider/id, if any.
func (r *Runtime) ModelSpecFor(providerName, modelID string) (ModelSpec, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.modelSpecs == nil {
		return ModelSpec{}, false
	}
	sp, ok := r.modelSpecs[providerName+"/"+modelID]
	return sp, ok
}
