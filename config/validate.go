package config

import (
	"fmt"
	"strings"

	"github.com/kasra-sh/zendawn-go/zerr"
)

// Validate performs semantic checks.
func Validate(d *Document) error {
	if err := validateProviders(d); err != nil {
		return err
	}
	if err := validateSecrets(d.Secrets); err != nil {
		return err
	}
	if err := validateMCP(d.MCP); err != nil {
		return err
	}
	if err := validateRuntimeCompaction(d.Runtime); err != nil {
		return err
	}
	if d.Single != nil {
		if err := validateAgent(d.Single, d); err != nil {
			return err
		}
		needCatalog := len(d.Models) > 0 || len(d.ModelProfiles) > 0
		if needCatalog && !catalogHas(d.Models, d.Single.Model.Provider, d.Single.Model.ID) {
			return zerr.New(zerr.CodeUnknownModel,
				fmt.Sprintf("model %s/%s not in catalog", d.Single.Model.Provider, d.Single.Model.ID), false)
		}
		return nil
	}
	if d.Version != 1 {
		return zerr.New(zerr.CodeInvalidConfig, "version must be 1", false)
	}
	if len(d.Agents) == 0 {
		return zerr.New(zerr.CodeInvalidConfig, "agents required", false)
	}
	needCatalog := len(d.Models) > 0 || len(d.ModelProfiles) > 0
	for id, a := range d.Agents {
		if a.ID == "" {
			a.ID = id
		}
		if err := validateAgent(a, d); err != nil {
			return err
		}
		if needCatalog {
			if !catalogHas(d.Models, a.Model.Provider, a.Model.ID) {
				return zerr.New(zerr.CodeUnknownModel,
					fmt.Sprintf("model %s/%s not in catalog", a.Model.Provider, a.Model.ID), false)
			}
		}
		for _, sid := range a.Subagents {
			if _, ok := d.Agents[sid]; !ok {
				return zerr.New(zerr.CodeUnknownAgent, "subagent not found: "+sid, false)
			}
		}
		for _, hid := range a.Handoffs {
			if _, ok := d.Agents[hid]; !ok {
				return zerr.New(zerr.CodeUnknownAgent, "handoff target not found: "+hid, false)
			}
		}
	}
	return nil
}

func validateRuntimeCompaction(r *RuntimeConfig) error {
	if r == nil {
		return nil
	}
	if err := validateCompactionMode(r.CompactionMode); err != nil {
		return err
	}
	if r.Compaction == nil {
		return nil
	}
	if r.Compaction.LaxThreshold != nil {
		t := *r.Compaction.LaxThreshold
		if t < 0 || t > 1 {
			return zerr.New(zerr.CodeInvalidConfig, "compaction.laxThreshold must be between 0 and 1", false)
		}
	}
	if r.Compaction.MinChars != nil && *r.Compaction.MinChars < 1 {
		return zerr.New(zerr.CodeInvalidConfig, "compaction.minChars must be >= 1", false)
	}
	return nil
}

func validateSecrets(secrets []SecretDecl) error {
	seen := map[string]bool{}
	for _, s := range secrets {
		if s.Name == "" {
			return zerr.New(zerr.CodeInvalidConfig, "secret name required", false)
		}
		for i, r := range s.Name {
			ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' ||
				(i > 0 && r >= '0' && r <= '9')
			if !ok {
				return zerr.New(zerr.CodeInvalidConfig, "invalid secret name: "+s.Name, false)
			}
		}
		if seen[s.Name] {
			return zerr.New(zerr.CodeInvalidConfig, "duplicate secret: "+s.Name, false)
		}
		seen[s.Name] = true
		switch s.Source {
		case "", "env", "env_or_ask", "ask":
		default:
			return zerr.New(zerr.CodeInvalidConfig, "invalid secret source: "+s.Source, false)
		}
		switch s.Persist {
		case "", "session", "process":
		default:
			return zerr.New(zerr.CodeInvalidConfig, "invalid secret persist: "+s.Persist, false)
		}
	}
	return nil
}

func validateMCP(m *MCPConfig) error {
	if m == nil {
		return nil
	}
	for id, s := range m.Servers {
		if strings.TrimSpace(id) == "" {
			return zerr.New(zerr.CodeInvalidConfig, "mcp server id required", false)
		}
		switch s.Transport {
		case "stdio":
			if strings.TrimSpace(s.Command) == "" {
				return zerr.New(zerr.CodeInvalidConfig, "mcp server "+id+": command required for stdio", false)
			}
		case "http":
			if strings.TrimSpace(s.URL) == "" {
				return zerr.New(zerr.CodeInvalidConfig, "mcp server "+id+": url required for http", false)
			}
		default:
			return zerr.New(zerr.CodeInvalidConfig, "mcp server "+id+": transport must be stdio or http", false)
		}
	}
	return nil
}

func validateProviders(d *Document) error {
	for name := range d.Providers {
		if strings.TrimSpace(name) == "" {
			return zerr.New(zerr.CodeInvalidConfig, "provider id required", false)
		}
	}
	if len(d.Providers) == 0 {
		return nil
	}
	// Catalog entries must reference a known provider when providers map is set.
	for _, m := range d.Models {
		if _, ok := d.Providers[m.Provider]; !ok {
			return zerr.New(zerr.CodeInvalidConfig,
				fmt.Sprintf("models entry %s/%s: provider not in providers map", m.Provider, m.ID), false)
		}
	}
	return nil
}

func validateCompactionMode(mode string) error {
	switch mode {
	case "", "none", "lax", "strict", "aggressive":
		return nil
	default:
		return zerr.New(zerr.CodeInvalidConfig, "invalid compactionMode: "+mode, false)
	}
}

func validateAgent(a *Agent, _ *Document) error {
	if a.Instructions == "" {
		return zerr.New(zerr.CodeInvalidConfig, "instructions required", false)
	}
	if a.Model.Provider == "" || a.Model.ID == "" {
		return zerr.New(zerr.CodeInvalidConfig, "model.provider and model.id required", false)
	}
	if a.MaxSteps != nil && *a.MaxSteps < 1 {
		return zerr.New(zerr.CodeInvalidConfig, "maxSteps must be >= 1", false)
	}
	if err := validateCompactionMode(a.CompactionMode); err != nil {
		return err
	}
	for _, p := range a.Permissions {
		switch p.Effect {
		case "allow", "ask", "deny":
		default:
			return zerr.New(zerr.CodeInvalidConfig, "invalid permission effect", false)
		}
	}
	return nil
}

func catalogHas(models []ModelCatalogEntry, provider, id string) bool {
	for _, m := range models {
		if m.Provider == provider && m.ID == id {
			return true
		}
	}
	return false
}
