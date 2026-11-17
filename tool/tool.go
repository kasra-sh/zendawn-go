package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// IsMCPToolName reports zendawn MCP registry names ({serverId}__{toolName}).
func IsMCPToolName(name string) bool {
	i := strings.Index(name, "__")
	return i > 0 && i < len(name)-2
}

// MCPNames returns sorted MCP tool names registered in r.
func MCPNames(r *Registry) []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, n := range r.Names() {
		if IsMCPToolName(n) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// Definition describes a tool for the model and runtime.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Permission  PermSpec       `json:"permission"`
	TimeoutMs   int            `json:"timeoutMs,omitempty"`
}

// PermSpec maps tool calls to permission assertions.
type PermSpec struct {
	Action       string `json:"action,omitempty"`
	ResourceFrom string `json:"resourceFrom,omitempty"`
}

// Result is a tool execution result.
type Result struct {
	IsError bool          `json:"isError"`
	Content []ContentPart `json:"content"`
	Code    string        `json:"code,omitempty"`
}

// ContentPart mirrors session content for tool results.
type ContentPart struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Value any    `json:"value,omitempty"`
}

// Executor runs a tool.
type Executor func(ctx context.Context, name string, args map[string]any) (Result, error)

// Entry is a registered tool.
type Entry struct {
	Def Definition
	Run Executor
}

// Registry maps tool names to entries.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]Entry
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]Entry{}}
}

// Register adds or replaces a tool.
func (r *Registry) Register(def Definition, run Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if def.Permission.Action == "" {
		def.Permission.Action = def.Name
	}
	r.byName[def.Name] = Entry{Def: def, Run: run}
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byName[name]
	return e, ok
}

// Names returns all registered names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byName))
	for n := range r.byName {
		out = append(out, n)
	}
	return out
}

// Snapshot is a frozen set of tools for one step.
type Snapshot struct {
	Tools []Entry
}

// Lookup finds a tool in the snapshot.
func (s Snapshot) Lookup(name string) (Entry, bool) {
	for _, t := range s.Tools {
		if t.Def.Name == name {
			return t, true
		}
	}
	return Entry{}, false
}

// Schemas returns provider-facing definitions.
func (s Snapshot) Schemas() []Definition {
	out := make([]Definition, len(s.Tools))
	for i, t := range s.Tools {
		out[i] = t.Def
	}
	return out
}

// BuildSnapshot selects tools for an agent.
// toolsAll: tools:"*"; names: explicit list; includeTask/Handoff/Skill from allowlists.
// Registered MCP tools ({serverId}__{toolName}) are always included.
func BuildSnapshot(reg *Registry, names []string, toolsAll, includeTask, includeHandoff, includeSkill, includeTodo bool) (Snapshot, error) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	want := map[string]bool{}
	if toolsAll {
		for n := range reg.byName {
			if n == "task" || n == "handoff" || n == "skill" || n == "todo" {
				continue
			}
			want[n] = true
		}
	} else {
		for _, n := range names {
			want[n] = true
		}
	}
	if includeTask {
		want["task"] = true
	}
	if includeHandoff {
		want["handoff"] = true
	}
	if includeSkill {
		want["skill"] = true
	}
	if includeTodo {
		want["todo"] = true
	}
	// MCP tools are injected like builtins — not gated by the agent tools list.
	for n := range reg.byName {
		if IsMCPToolName(n) {
			want[n] = true
		}
	}

	var tools []Entry
	for n := range want {
		e, ok := reg.byName[n]
		if !ok {
			return Snapshot{}, fmt.Errorf("unknown_tool: %s", n)
		}
		tools = append(tools, e)
	}
	return Snapshot{Tools: tools}, nil
}

// ValidateArgs checks required properties on an object schema.
func ValidateArgs(schema map[string]any, args map[string]any) error {
	if schema == nil {
		return nil
	}
	req, _ := schema["required"].([]any)
	for _, r := range req {
		key, _ := r.(string)
		if key == "" {
			continue
		}
		if _, ok := args[key]; !ok {
			return fmt.Errorf("missing required %s", key)
		}
	}
	return nil
}

// ArgsMap coerces tool arguments to map[string]any.
func ArgsMap(arguments any) (map[string]any, error) {
	switch v := arguments.(type) {
	case map[string]any:
		return v, nil
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			return nil, err
		}
		return m, nil
	case nil:
		return map[string]any{}, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		return m, nil
	}
}

// ResourceFromArgs extracts permission resource.
func ResourceFromArgs(spec PermSpec, args map[string]any) string {
	if spec.ResourceFrom == "" {
		return "*"
	}
	v, ok := args[spec.ResourceFrom]
	if !ok {
		return "*"
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// TextResult builds a simple text result.
func TextResult(text string, isError bool, code string) Result {
	return Result{
		IsError: isError,
		Code:    code,
		Content: []ContentPart{{Type: "text", Text: text}},
	}
}

// JSONResult builds text + json result.
func JSONResult(text string, value any, isError bool, code string) Result {
	return Result{
		IsError: isError,
		Code:    code,
		Content: []ContentPart{
			{Type: "text", Text: text},
			{Type: "json", Value: value},
		},
	}
}
