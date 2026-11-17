package config

// Document is either a single-agent or multi-agent config.
type Document struct {
	// Multi-agent
	Version       int                       `json:"version,omitempty" yaml:"version,omitempty"`
	Agents        map[string]*Agent         `json:"agents,omitempty" yaml:"agents,omitempty"`
	Providers     map[string]ProviderConfig `json:"providers,omitempty" yaml:"providers,omitempty"`
	Models        []ModelCatalogEntry       `json:"models,omitempty" yaml:"models,omitempty"`
	ModelProfiles []ModelProfile            `json:"modelProfiles,omitempty" yaml:"modelProfiles,omitempty"`
	Runtime       *RuntimeConfig            `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	SkillsDirs    []string                  `json:"skillsDirs,omitempty" yaml:"skillsDirs,omitempty"`
	Secrets       []SecretDecl              `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	MCP           *MCPConfig                `json:"mcp,omitempty" yaml:"mcp,omitempty"`

	// Single-agent (flattened)
	Single *Agent `json:"-" yaml:"-"`
}

// SecretDecl declares a named secret for vault / secret.ask.
type SecretDecl struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Source      string `json:"source,omitempty" yaml:"source,omitempty"`   // env | env_or_ask | ask
	Persist     string `json:"persist,omitempty" yaml:"persist,omitempty"` // session | process
}

// MCPConfig holds optional MCP servers.
type MCPConfig struct {
	Servers map[string]MCPServer `json:"servers,omitempty" yaml:"servers,omitempty"`
}

// MCPServer is one MCP server entry.
type MCPServer struct {
	Transport string            `json:"transport" yaml:"transport"` // stdio | http
	Command   string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args      []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	URL       string            `json:"url,omitempty" yaml:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Optional  bool              `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// ProviderConfig describes an OpenAI-compatible backend.
type ProviderConfig struct {
	BaseURL   string `json:"baseUrl,omitempty" yaml:"baseUrl,omitempty"`
	APIKeyEnv string `json:"apiKeyEnv,omitempty" yaml:"apiKeyEnv,omitempty"`
	APIKey    string `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
}

// Agent is agent configuration.
type Agent struct {
	ID                 string              `json:"id,omitempty" yaml:"id,omitempty"`
	Name               string              `json:"name,omitempty" yaml:"name,omitempty"`
	Instructions       string              `json:"instructions" yaml:"instructions"`
	Model              ModelConfig         `json:"model" yaml:"model"`
	Tools              any                 `json:"tools,omitempty" yaml:"tools,omitempty"` // []string or "*"
	Subagents          []string            `json:"subagents,omitempty" yaml:"subagents,omitempty"`
	Handoffs           []string            `json:"handoffs,omitempty" yaml:"handoffs,omitempty"`
	Permissions        []PermissionRule    `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	MaxSteps           *int                `json:"maxSteps,omitempty" yaml:"maxSteps,omitempty"`
	ToolParallelism    string              `json:"toolParallelism,omitempty" yaml:"toolParallelism,omitempty"`
	ToolTimeoutMs      *int                `json:"toolTimeoutMs,omitempty" yaml:"toolTimeoutMs,omitempty"`
	AskTimeoutMs       *int                `json:"askTimeoutMs,omitempty" yaml:"askTimeoutMs,omitempty"`
	AskTimeoutDecision string              `json:"askTimeoutDecision,omitempty" yaml:"askTimeoutDecision,omitempty"`
	Retries            *RetriesConfig      `json:"retries,omitempty" yaml:"retries,omitempty"`
	MaxSubagentDepth   *int                `json:"maxSubagentDepth,omitempty" yaml:"maxSubagentDepth,omitempty"`
	Hooks              map[string][]string `json:"hooks,omitempty" yaml:"hooks,omitempty"`
	Sandbox            *SandboxConfig      `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
	Stream             *bool               `json:"stream,omitempty" yaml:"stream,omitempty"`
	CompactionMode     string              `json:"compactionMode,omitempty" yaml:"compactionMode,omitempty"` // none|lax|strict|aggressive
	Skills             []string            `json:"skills,omitempty" yaml:"skills,omitempty"`
	SkillsDirs         []string            `json:"skillsDirs,omitempty" yaml:"skillsDirs,omitempty"`
	DefaultMode        string              `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"` // ask|plan|agent
}

// PermissionRule mirrors permission.Rule for config decoding.
type PermissionRule struct {
	Action   string `json:"action" yaml:"action"`
	Resource string `json:"resource" yaml:"resource"`
	Effect   string `json:"effect" yaml:"effect"`
}

// ModelConfig selects a provider model.
type ModelConfig struct {
	Provider  string         `json:"provider" yaml:"provider"`
	ID        string         `json:"id" yaml:"id"`
	APIKey    string         `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
	APIKeyEnv string         `json:"apiKeyEnv,omitempty" yaml:"apiKeyEnv,omitempty"`
	Params    map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
}

// ModelCatalogEntry tags a model.
type ModelCatalogEntry struct {
	Provider  string `json:"provider" yaml:"provider"`
	ID        string `json:"id" yaml:"id"`
	Class     string `json:"class,omitempty" yaml:"class,omitempty"`
	SizeClass string `json:"sizeClass,omitempty" yaml:"sizeClass,omitempty"`
}

// ModelProfile overlays policy for matching models.
type ModelProfile struct {
	Match  ModelProfileMatch `json:"match" yaml:"match"`
	Policy ClosedPolicy      `json:"policy" yaml:"policy"`
}

// ModelProfileMatch AND-matches model attributes.
type ModelProfileMatch struct {
	Provider  string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model     string `json:"model,omitempty" yaml:"model,omitempty"`
	Class     string `json:"class,omitempty" yaml:"class,omitempty"`
	SizeClass string `json:"sizeClass,omitempty" yaml:"sizeClass,omitempty"`
}

// ClosedPolicy is the closed set of overridable loop policy keys.
type ClosedPolicy struct {
	MaxSteps           *int           `json:"maxSteps,omitempty" yaml:"maxSteps,omitempty"`
	ToolParallelism    string         `json:"toolParallelism,omitempty" yaml:"toolParallelism,omitempty"`
	Stream             *bool          `json:"stream,omitempty" yaml:"stream,omitempty"`
	ToolTimeoutMs      *int           `json:"toolTimeoutMs,omitempty" yaml:"toolTimeoutMs,omitempty"`
	AskTimeoutMs       *int           `json:"askTimeoutMs,omitempty" yaml:"askTimeoutMs,omitempty"`
	AskTimeoutDecision string         `json:"askTimeoutDecision,omitempty" yaml:"askTimeoutDecision,omitempty"`
	Retries            *RetriesConfig `json:"retries,omitempty" yaml:"retries,omitempty"`
	MaxSubagentDepth   *int           `json:"maxSubagentDepth,omitempty" yaml:"maxSubagentDepth,omitempty"`
	CompactionMode     string         `json:"compactionMode,omitempty" yaml:"compactionMode,omitempty"`
	Params             map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
}

// CompactionConfig holds runtime thresholds for projection compaction.
type CompactionConfig struct {
	LaxThreshold *float64 `json:"laxThreshold,omitempty" yaml:"laxThreshold,omitempty"`
	MinChars     *int     `json:"minChars,omitempty" yaml:"minChars,omitempty"`
}

// RetriesConfig is provider retry policy.
type RetriesConfig struct {
	MaxAttempts       *int     `json:"maxAttempts,omitempty" yaml:"maxAttempts,omitempty"`
	BackoffMs         *int     `json:"backoffMs,omitempty" yaml:"backoffMs,omitempty"`
	BackoffMultiplier *float64 `json:"backoffMultiplier,omitempty" yaml:"backoffMultiplier,omitempty"`
	RetryableCodes    []string `json:"retryableCodes,omitempty" yaml:"retryableCodes,omitempty"`
}

// RuntimeConfig is document-level runtime defaults.
type RuntimeConfig struct {
	ToolTimeoutMs      *int              `json:"toolTimeoutMs,omitempty" yaml:"toolTimeoutMs,omitempty"`
	Retries            *RetriesConfig    `json:"retries,omitempty" yaml:"retries,omitempty"`
	Hooks              *RuntimeHooks     `json:"hooks,omitempty" yaml:"hooks,omitempty"`
	MaxSubagentDepth   *int              `json:"maxSubagentDepth,omitempty" yaml:"maxSubagentDepth,omitempty"`
	AskTimeoutMs       *int              `json:"askTimeoutMs,omitempty" yaml:"askTimeoutMs,omitempty"`
	AskTimeoutDecision string            `json:"askTimeoutDecision,omitempty" yaml:"askTimeoutDecision,omitempty"`
	CompactionMode     string            `json:"compactionMode,omitempty" yaml:"compactionMode,omitempty"`
	Compaction         *CompactionConfig `json:"compaction,omitempty" yaml:"compaction,omitempty"`
}

// RuntimeHooks holds hook runtime flags.
type RuntimeHooks struct {
	IgnoreErrors bool `json:"ignoreErrors,omitempty" yaml:"ignoreErrors,omitempty"`
}

// SandboxConfig names a sandbox hook.
type SandboxConfig struct {
	Hook string `json:"hook,omitempty" yaml:"hook,omitempty"`
}

// ResolvedPolicy is the fully merged policy for a drain segment.
type ResolvedPolicy struct {
	MaxSteps           int
	ToolParallelism    string
	Stream             bool
	ToolTimeoutMs      int
	AskTimeoutMs       int
	AskTimeoutDecision string
	Retries            RetriesResolved
	MaxSubagentDepth   int
	CompactionMode     string
	CompactionLaxRatio float64
	CompactionMinChars int
	Params             map[string]any
	IgnoreHookErrors   bool
}

// RetriesResolved is concrete retry settings.
type RetriesResolved struct {
	MaxAttempts       int
	BackoffMs         int
	BackoffMultiplier float64
	RetryableCodes    []string
}

// ToolNames returns the agent tool list; nil means empty; special "*" via ToolsIsAll.
func (a *Agent) ToolNames() []string {
	if a.Tools == nil {
		return nil
	}
	switch t := a.Tools.(type) {
	case string:
		if t == "*" {
			return nil
		}
		return nil
	case []any:
		out := make([]string, 0, len(t))
		for _, v := range t {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
}

// ToolsIsAll reports tools: "*".
func (a *Agent) ToolsIsAll() bool {
	s, ok := a.Tools.(string)
	return ok && s == "*"
}
