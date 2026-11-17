package zerr

// Error is a normalized zendawn error.
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

// Standard codes
const (
	CodeRateLimit                  = "rate_limit"
	CodeTimeout                    = "timeout"
	CodeUnavailable                = "unavailable"
	CodeOverloaded                 = "overloaded"
	CodeAuthFailed                 = "auth_failed"
	CodeInvalidRequest             = "invalid_request"
	CodeInvalidConfig              = "invalid_config"
	CodeInvalidPlan                = "invalid_plan"
	CodeUnknownModel               = "unknown_model"
	CodeUnknownTool                = "unknown_tool"
	CodeUnknownHook                = "unknown_hook"
	CodeUnknownSkill               = "unknown_skill"
	CodeUnknownAgent               = "unknown_agent"
	CodeMaxDepth                   = "max_depth"
	CodeProviderCapabilityMismatch = "provider_capability_mismatch"
	CodeProviderExhausted          = "provider_exhausted"
	CodePermissionDenied           = "permission_denied"
	CodeToolTimeout                = "tool_timeout"
	CodeToolPanic                  = "tool_panic"
	CodeToolInvalidArgs            = "tool_invalid_args"
	CodeHookError                  = "hook_error"
	CodeCompactionFailed           = "compaction_failed"
	CodeMissingSecret              = "missing_secret"
	CodeMCPConnectFailed           = "mcp_connect_failed"
	CodeMCPToolError               = "mcp_tool_error"
	CodeAborted                    = "aborted"
	CodeInternal                   = "internal"
)

// New creates an error.
func New(code, message string, retryable bool) *Error {
	return &Error{Code: code, Message: message, Retryable: retryable}
}
