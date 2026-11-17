package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kasra-sh/zendawn-go/zerr"
)

// LoadFile loads a YAML or JSON agent document from path.
func LoadFile(path string) (*Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		return ParseJSON(b)
	}
	return ParseYAML(b)
}

// ParseYAML parses a YAML document.
func ParseYAML(b []byte) (*Document, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", zerr.CodeInvalidConfig, err)
	}
	jb, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	return ParseJSON(jb)
}

// ParseJSON parses a JSON document (also used after YAML→JSON).
func ParseJSON(b []byte) (*Document, error) {
	var probe map[string]any
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("%s: %w", zerr.CodeInvalidConfig, err)
	}

	doc := &Document{}
	if _, ok := probe["agents"]; ok {
		if err := json.Unmarshal(b, doc); err != nil {
			return nil, fmt.Errorf("%s: %w", zerr.CodeInvalidConfig, err)
		}
		if doc.Version != 1 {
			return nil, zerr.New(zerr.CodeInvalidConfig, "version must be 1", false)
		}
		for id, a := range doc.Agents {
			if a.ID == "" {
				a.ID = id
			}
			normalizeTools(a)
		}
		if err := Validate(doc); err != nil {
			return nil, err
		}
		return doc, nil
	}

	var agent Agent
	if err := json.Unmarshal(b, &agent); err != nil {
		return nil, fmt.Errorf("%s: %w", zerr.CodeInvalidConfig, err)
	}
	if agent.ID == "" || agent.Instructions == "" || agent.Model.Provider == "" || agent.Model.ID == "" {
		return nil, zerr.New(zerr.CodeInvalidConfig, "single-agent document requires id, instructions, model", false)
	}
	normalizeTools(&agent)
	doc.Single = &agent
	// Optional document-level fields on single-agent YAML/JSON.
	var extras struct {
		Providers     map[string]ProviderConfig `json:"providers"`
		Models        []ModelCatalogEntry       `json:"models"`
		ModelProfiles []ModelProfile            `json:"modelProfiles"`
		Runtime       *RuntimeConfig            `json:"runtime"`
		SkillsDirs    []string                  `json:"skillsDirs"`
		Secrets       []SecretDecl              `json:"secrets"`
		MCP           *MCPConfig                `json:"mcp"`
	}
	_ = json.Unmarshal(b, &extras)
	doc.Providers = extras.Providers
	doc.Models = extras.Models
	doc.ModelProfiles = extras.ModelProfiles
	doc.Runtime = extras.Runtime
	doc.SkillsDirs = extras.SkillsDirs
	doc.Secrets = extras.Secrets
	doc.MCP = extras.MCP
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func normalizeTools(a *Agent) {
	if a.Tools == nil {
		return
	}
	switch t := a.Tools.(type) {
	case []any:
		ss := make([]string, 0, len(t))
		for _, v := range t {
			if s, ok := v.(string); ok {
				ss = append(ss, s)
			}
		}
		a.Tools = ss
	}
}

// Agent returns an agent by id.
func (d *Document) Agent(id string) (*Agent, error) {
	if d.Single != nil {
		if id == "" || id == d.Single.ID {
			return d.Single, nil
		}
		return nil, zerr.New(zerr.CodeUnknownAgent, "agent not found: "+id, false)
	}
	a, ok := d.Agents[id]
	if !ok {
		return nil, zerr.New(zerr.CodeUnknownAgent, "agent not found: "+id, false)
	}
	return a, nil
}

// IsMulti reports a multi-agent document.
func (d *Document) IsMulti() bool {
	return d.Agents != nil
}
