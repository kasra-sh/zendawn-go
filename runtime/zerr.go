package runtime

import "github.com/kasra-sh/zendawn-go/zerr"

// Re-export common error helpers for runtime call sites.
type Error = zerr.Error

var NewError = zerr.New

const (
	CodeRateLimit                  = zerr.CodeRateLimit
	CodeTimeout                    = zerr.CodeTimeout
	CodeUnavailable                = zerr.CodeUnavailable
	CodeOverloaded                 = zerr.CodeOverloaded
	CodeAuthFailed                 = zerr.CodeAuthFailed
	CodeInvalidRequest             = zerr.CodeInvalidRequest
	CodeInvalidConfig              = zerr.CodeInvalidConfig
	CodeInvalidPlan                = zerr.CodeInvalidPlan
	CodeUnknownModel               = zerr.CodeUnknownModel
	CodeUnknownTool                = zerr.CodeUnknownTool
	CodeUnknownHook                = zerr.CodeUnknownHook
	CodeUnknownSkill               = zerr.CodeUnknownSkill
	CodeUnknownAgent               = zerr.CodeUnknownAgent
	CodeMaxDepth                   = zerr.CodeMaxDepth
	CodeProviderCapabilityMismatch = zerr.CodeProviderCapabilityMismatch
	CodeProviderExhausted          = zerr.CodeProviderExhausted
	CodePermissionDenied           = zerr.CodePermissionDenied
	CodeToolTimeout                = zerr.CodeToolTimeout
	CodeToolPanic                  = zerr.CodeToolPanic
	CodeToolInvalidArgs            = zerr.CodeToolInvalidArgs
	CodeHookError                  = zerr.CodeHookError
	CodeCompactionFailed           = zerr.CodeCompactionFailed
	CodeMissingSecret              = zerr.CodeMissingSecret
	CodeMCPConnectFailed           = zerr.CodeMCPConnectFailed
	CodeMCPToolError               = zerr.CodeMCPToolError
	CodeAborted                    = zerr.CodeAborted
	CodeInternal                   = zerr.CodeInternal
)
