package runtime

import "time"

// Event types
const (
	EventDrainStarted         = "drain.started"
	EventDrainCompleted       = "drain.completed"
	EventStepStarted          = "step.started"
	EventStepCompleted        = "step.completed"
	EventPermissionAsk        = "permission.ask"
	EventPermissionResolved   = "permission.resolved"
	EventSecretAsk            = "secret.ask"
	EventSecretResolved       = "secret.resolved"
	EventProviderTextDelta    = "provider.text_delta"
	EventProviderToolDelta    = "provider.tool_call_delta"
	EventProviderToolDone     = "provider.tool_call_completed"
	EventProviderMsgCompleted = "provider.message_completed"
	EventProviderError        = "provider.error"
	EventToolStarted          = "tool.started"
	EventToolCompleted        = "tool.completed"
	EventToolProgress         = "tool.progress"
	EventSubagentStarted      = "subagent.started"
	EventSubagentCompleted    = "subagent.completed"
	EventHandoffCompleted     = "handoff.completed"
	EventPlanAsk              = "plan.ask"
	EventPlanResolved         = "plan.resolved"
	EventUsage                = "usage.updated"
	EventModelChanged         = "model.changed"
	EventAttachmentUploaded   = "attachment.uploaded"
	EventAttachmentRegistered = "attachment.registered"
	EventCompactionUpdated    = "compaction.updated"
	EventError                = "error"
)

// Event is a runtime stream event.
type Event struct {
	Type      string         `json:"type"`
	SessionID string         `json:"sessionId"`
	TS        time.Time      `json:"ts"`
	Data      map[string]any `json:"data"`
}

// NewEvent builds an event with UTC timestamp.
func NewEvent(typ, sessionID string, data map[string]any) Event {
	if data == nil {
		data = map[string]any{}
	}
	return Event{Type: typ, SessionID: sessionID, TS: time.Now().UTC(), Data: data}
}
