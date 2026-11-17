package runtime

import (
	"context"
	"time"

	"github.com/kasra-sh/zendawn-go/compact"
	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/session"
)

const compactionAgentSuffix = "::compaction"

// maybeCompactProjection may run a summarizer and returns messages for the main Complete call.
// Durable transcript is never modified. On summarizer failure, falls back to prior summary or full transcript.
func (r *Runtime) maybeCompactProjection(
	ctx context.Context,
	s *session.Session,
	prov provider.Provider,
	modelID string,
	agentID string,
	policy config.ResolvedPolicy,
	msgs []session.Item,
) []session.Item {
	mode := compact.NormalizeMode(policy.CompactionMode)
	if mode == compact.ModeNone {
		return msgs
	}

	s.Lock()
	st := compact.State{Summary: s.CompactionSummary, Through: s.CompactionThrough}
	inputTokens := s.LastInputTokens
	contextLimit := s.LastContextLimit
	s.Unlock()

	th := compact.Thresholds{
		LaxRatio: policy.CompactionLaxRatio,
		MinChars: policy.CompactionMinChars,
	}
	if !compact.ShouldCompact(mode, msgs, st, th, inputTokens, contextLimit) {
		return compact.Project(msgs, st)
	}

	through := compact.CoverThrough(msgs)
	if through <= st.Through {
		return compact.Project(msgs, st)
	}

	toFold := msgs[st.Through:through]
	sumReq := provider.Request{
		Model:        modelID,
		Instructions: compact.SummarizerInstructions(mode),
		Messages: []session.Item{{
			Type:      session.ItemMessage,
			Role:      session.RoleUser,
			CreatedAt: time.Now().UTC(),
			Content: []session.ContentPart{{
				Type: session.PartText,
				Text: compact.BuildSummarizerUserPrompt(st.Summary, toFold),
			}},
		}},
		Tools:   nil,
		Params:  map[string]any{},
		Stream:  false,
		AgentID: agentID + compactionAgentSuffix,
	}

	resp, err := prov.Complete(ctx, sumReq, nil)
	if err != nil {
		r.emit(NewEvent(EventError, s.ID, map[string]any{
			"code":    CodeCompactionFailed,
			"message": err.Error(),
		}))
		return compact.Project(msgs, st)
	}

	summary := session.TextParts(resp.Message.Content)
	if summary == "" {
		r.emit(NewEvent(EventError, s.ID, map[string]any{
			"code":    CodeCompactionFailed,
			"message": "empty compaction summary",
		}))
		return compact.Project(msgs, st)
	}

	// Optionally fold summarizer usage into session totals without changing Last* for triggers.
	if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
		s.Lock()
		s.UsageInput += resp.Usage.InputTokens
		s.UsageOutput += resp.Usage.OutputTokens
		if resp.Usage.CostKnown {
			s.UsageCostUSD += resp.Usage.CostUSD
			s.UsageCostKnown = true
		}
		s.Unlock()
	}

	newState := compact.State{Summary: summary, Through: through}
	s.Lock()
	s.CompactionSummary = summary
	s.CompactionThrough = through
	s.Unlock()

	r.emit(NewEvent(EventCompactionUpdated, s.ID, map[string]any{
		"summary": summary,
		"through": through,
	}))

	return compact.Project(msgs, newState)
}
