package session

import "time"

// Item types
const (
	ItemMessage    = "message"
	ItemToolResult = "tool_result"
	ItemHandoff    = "handoff"
	ItemError      = "error"
)

// Roles
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Content part types
const (
	PartText     = "text"
	PartImage    = "image"
	PartFile     = "file"
	PartToolCall = "tool_call"
	PartJSON     = "json"
)

// Item is a transcript entry.
type Item struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"createdAt"`
	Role      string    `json:"role,omitempty"`

	// message
	Content []ContentPart `json:"content,omitempty"`

	// tool_result
	ToolCallID string `json:"toolCallId,omitempty"`
	Name       string `json:"name,omitempty"`
	IsError    bool   `json:"isError,omitempty"`

	// handoff
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"`

	// error
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// ContentPart is a message or tool-result content part.
type ContentPart struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	URI          string `json:"uri,omitempty"`
	Data         string `json:"data,omitempty"`
	MimeType     string `json:"mimeType,omitempty"`
	Filename     string `json:"filename,omitempty"`
	AttachmentID string `json:"attachmentId,omitempty"`

	ToolCallID string `json:"toolCallId,omitempty"`
	Name       string `json:"name,omitempty"`
	Arguments  any    `json:"arguments,omitempty"`

	Value any `json:"value,omitempty"`
}

// TextParts concatenates text parts.
func TextParts(parts []ContentPart) string {
	var s string
	for _, p := range parts {
		if p.Type == PartText {
			s += p.Text
		}
	}
	return s
}

// ToolCalls extracts tool_call parts from an assistant message.
func ToolCalls(parts []ContentPart) []ContentPart {
	var out []ContentPart
	for _, p := range parts {
		if p.Type == PartToolCall {
			out = append(out, p)
		}
	}
	return out
}
