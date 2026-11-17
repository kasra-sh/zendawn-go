package session

import (
	"context"
	"sync"
	"time"

	"github.com/kasra-sh/zendawn-go/permission"
)

// Stop reasons (closed set).
const (
	StopCompleted     = "completed"
	StopMaxSteps      = "max_steps"
	StopAborted       = "aborted"
	StopAwaitingInput = "awaiting_input"
	StopError         = "error"
)

// Interaction modes (OpenCode / Claude CLI style).
const (
	ModeAsk   = "ask"
	ModePlan  = "plan"
	ModeAgent = "agent"
)

// Session holds conversation state.
type Session struct {
	mu sync.Mutex

	ID          string
	ActiveAgent string
	ModelID     string
	Provider    string
	Busy        bool
	Depth       int
	ParentID    string
	Mode        string // ask | plan | agent
	WorkDir     string

	Transcript []Item
	Overlays   []permission.Rule
	ModeRules  []permission.Rule // applied after Overlays (last match wins)

	Queued   []UserInput // pending submit inputs
	Steer    []UserInput // pending steer inputs for next step boundary
	Aborting bool
	// DrainCancel cancels the in-flight drain context (provider HTTP, retries).
	DrainCancel context.CancelFunc

	// Ask handling
	AskCh       chan AskDecision
	AskTimedOut bool

	ActiveSkills map[string]bool // skill id → active
	BoundSkills  []string        // effective skill ids (config ∪ auto-discover)

	// Plan app-context (not chat transcript).
	PlanMarkdown   string       // current working buffer
	PlanTodos      []PlanTodo   // working todos (runtime source of truth)
	Plans          []StoredPlan // recent plans, newest last (cap 5)
	CurrentPlanID  string
	PendingPlanAsk bool // edit vs new when entering plan with existing plan
	PlanAskCh      chan PlanAskDecision

	// Workspace context cached at create (AGENTS.md + local rules).
	AgentsMD  string
	RulesText string

	// Usage totals (from provider response objects).
	UsageInput       int
	UsageOutput      int
	UsageCostUSD     float64
	UsageCostKnown   bool // true if any step reported cost
	LastInputTokens  int
	LastContextLimit int            // last known context window for active model
	ContextLimits    map[string]int // provider/id → last known limit from responses

	// Projection compaction (durable Transcript is never rewritten).
	CompactionSummary string
	CompactionThrough int // exclusive end index covered by CompactionSummary

	LastStopReason string
	CreatedAt      time.Time
}

// PlanTodo is one checklist item on the current plan.
type PlanTodo struct {
	ID      string
	Content string
	Done    bool
}

// StoredPlan is an archived/current plan snapshot in session app state.
type StoredPlan struct {
	ID      string
	Title   string
	Body    string
	Todos   []PlanTodo
	Updated time.Time
}

// PlanAskDecision is edit-current vs start-new when entering plan mode.
type PlanAskDecision string

const (
	PlanAskEdit PlanAskDecision = "edit"
	PlanAskNew  PlanAskDecision = "new"
)

// UserInput is structured host input for submit/steer (text + attachments).
type UserInput struct {
	Text          string
	AttachmentIDs []string
}

// AskDecision is a permission ask resolution.
type AskDecision string

const (
	AskOnce   AskDecision = "once"
	AskAlways AskDecision = "always"
	AskReject AskDecision = "reject"
)

// New creates an idle session.
func New(id, agentID string) *Session {
	return &Session{
		ID:           id,
		ActiveAgent:  agentID,
		Mode:         ModeAgent,
		CreatedAt:    time.Now().UTC(),
		AskCh:        make(chan AskDecision, 1),
		PlanAskCh:    make(chan PlanAskDecision, 1),
		ActiveSkills: map[string]bool{},
	}
}

// EffectiveOverlays returns session overlays then mode rules.
func (s *Session) EffectiveOverlays() []permission.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]permission.Rule, 0, len(s.Overlays)+len(s.ModeRules))
	out = append(out, s.Overlays...)
	out = append(out, s.ModeRules...)
	return out
}

// AppendItem appends a transcript item (caller holds lock or is sole owner).
func (s *Session) AppendItem(it Item) {
	if it.CreatedAt.IsZero() {
		it.CreatedAt = time.Now().UTC()
	}
	s.Transcript = append(s.Transcript, it)
}

// History returns a copy of the transcript.
func (s *Session) History() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, len(s.Transcript))
	copy(out, s.Transcript)
	return out
}

// PopFailedTurn removes the last user message and any items after it (failed turn).
// Returns the user text when a user message was found.
func (s *Session) PopFailedTurn() (userText string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.Transcript)
	if n == 0 {
		return "", false
	}
	userIdx := -1
	for i := n - 1; i >= 0; i-- {
		it := s.Transcript[i]
		if it.Type == ItemMessage && it.Role == RoleUser {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		return "", false
	}
	userText = TextParts(s.Transcript[userIdx].Content)
	s.Transcript = s.Transcript[:userIdx]
	return userText, true
}

// Lock locks the session.
func (s *Session) Lock() { s.mu.Lock() }

// Unlock unlocks the session.
func (s *Session) Unlock() { s.mu.Unlock() }
