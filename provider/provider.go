package provider

import (
	"context"

	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/tool"
)

// Capabilities advertised by a provider.
type Capabilities struct {
	Tools     bool
	Streaming bool
	Images    bool
}

// AttachmentResolver loads session attachment bytes for multimodal projection.
type AttachmentResolver func(id string) (filename, mimeType, relPath string, data []byte, err error)

// Request is a normalized model request.
type Request struct {
	Model             string
	Instructions      string
	Messages          []session.Item
	Tools             []tool.Definition
	Params            map[string]any
	Stream            bool
	AgentID           string // for conformance tracking
	ResolveAttachment AttachmentResolver
}

// Response is a normalized non-stream (or completed stream) response.
type Response struct {
	ID           string
	Message      session.Item
	Usage        Usage
	FinishReason string
}

// Usage token counts and optional cost/context from the provider response.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64 // present when provider reports cost (e.g. OpenRouter)
	CostKnown    bool    // true when CostUSD came from the response
	ContextLimit int     // present when provider reports context window on the response
}

// StreamHandler receives stream events; implementations may call it during Complete.
type StreamHandler func(eventType string, data map[string]any)

// Provider calls a model backend.
type Provider interface {
	ID() string
	Capabilities() Capabilities
	Complete(ctx context.Context, req Request, onEvent StreamHandler) (Response, error)
}
