package provider

import (
	"context"
	"sync"
	"time"

	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/zerr"
)

// ScriptEntry is one mock model response or error.
type ScriptEntry struct {
	Message      *session.Item `json:"message"`
	FinishReason string        `json:"finishReason"`
	Error        *zerr.Error   `json:"error"`
}

// Mock is a scripted provider for conformance.
type Mock struct {
	mu       sync.Mutex
	scripts  []ScriptEntry
	byAgent  map[string][]ScriptEntry
	agentIdx map[string]int
	idx      int

	Calls []Request
}

// NewMock creates a mock provider.
func NewMock(script []ScriptEntry) *Mock {
	return &Mock{
		scripts:  script,
		byAgent:  map[string][]ScriptEntry{},
		agentIdx: map[string]int{},
	}
}

// SetChildScripts sets per-agent scripts for child sessions.
func (m *Mock) SetChildScripts(byAgent map[string][]ScriptEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byAgent = byAgent
	m.agentIdx = map[string]int{}
}

func (m *Mock) ID() string { return "mock" }

func (m *Mock) Capabilities() Capabilities {
	return Capabilities{Tools: true, Streaming: false, Images: false}
}

func (m *Mock) Complete(ctx context.Context, req Request, onEvent StreamHandler) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, req)

	var entry ScriptEntry
	if script, ok := m.byAgent[req.AgentID]; ok && len(script) > 0 {
		i := m.agentIdx[req.AgentID]
		if i >= len(script) {
			return Response{}, zerr.New(zerr.CodeInternal, "mock script exhausted for agent "+req.AgentID, false)
		}
		entry = script[i]
		m.agentIdx[req.AgentID] = i + 1
	} else {
		if m.idx >= len(m.scripts) {
			return Response{}, zerr.New(zerr.CodeInternal, "mock script exhausted", false)
		}
		entry = m.scripts[m.idx]
		m.idx++
	}

	if entry.Error != nil {
		return Response{}, entry.Error
	}
	if entry.Message == nil {
		return Response{}, zerr.New(zerr.CodeInternal, "empty mock message", false)
	}

	msg := *entry.Message
	msg.Type = session.ItemMessage
	msg.Role = session.RoleAssistant
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if onEvent != nil {
		onEvent("provider.message_completed", map[string]any{"message": msg})
	}
	return Response{
		Message:      msg,
		FinishReason: entry.FinishReason,
	}, nil
}
