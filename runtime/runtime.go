package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kasra-sh/zendawn-go/attachment"
	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/hook"
	"github.com/kasra-sh/zendawn-go/mcp"
	"github.com/kasra-sh/zendawn-go/paths"
	"github.com/kasra-sh/zendawn-go/permission"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/secret"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/skill"
	"github.com/kasra-sh/zendawn-go/tool"
)

// AskResolver supplies ask decisions (conformance / host UI).
type AskResolver func(sessionID string, action, resource, toolCallID string) session.AskDecision

// Runtime is the zendawn harness.
type Runtime struct {
	Doc       *config.Document
	Tools     *tool.Registry
	Hooks     *hook.Registry
	Providers map[string]provider.Provider
	Ask       AskResolver
	SecretAsk SecretAskResolver
	OnEvent   func(Event) // optional live event sink (TUI)
	Secrets   *secret.Store
	MCP       *mcp.Manager

	WorkDir     string // session working directory (DMZ root parent)
	ConfigDir   string // directory of the agent config file (for relative skillsDirs)
	DefaultMode string // optional host override of agent defaultMode (ask|plan|agent)

	mu              sync.Mutex
	sessions        map[string]*session.Session
	sessionSkills   map[string]map[string]*skill.Pack
	attachments     map[string]*attachment.Store
	modelSpecs      map[string]ModelSpec // provider/id → OpenRouter (or other) metadata
	idSeq           atomic.Uint64
	events          []Event // global chronological for conformance
	evMu            sync.Mutex
	secretWait      chan string
	secretSessionID string
}

// New creates a runtime.
func New(doc *config.Document) *Runtime {
	r := &Runtime{
		Doc:           doc,
		Tools:         tool.NewRegistry(),
		Hooks:         hook.NewRegistry(),
		Providers:     map[string]provider.Provider{},
		sessions:      map[string]*session.Session{},
		sessionSkills: map[string]map[string]*skill.Pack{},
		attachments:   map[string]*attachment.Store{},
	}
	r.registerBuiltins()
	r.InitSecrets()
	return r
}

// ConnectMCP connects configured MCP servers and registers tools.
// Call after SecretAsk is wired if secrets may need prompts.
func (r *Runtime) ConnectMCP(ctx context.Context, sessionID string) (mcp.ConnectReport, error) {
	r.InitSecrets()
	r.secretSessionID = sessionID
	if r.MCP == nil {
		r.MCP = mcp.NewManager(r.Secrets)
	}
	if r.Doc == nil || r.Doc.MCP == nil {
		return mcp.ConnectReport{}, nil
	}
	return r.MCP.ConnectAll(ctx, r.Doc.MCP, r.Tools)
}

// CloseMCP shuts down MCP clients and clears session secrets.
func (r *Runtime) CloseMCP() {
	if r.MCP != nil {
		_ = r.MCP.Close()
	}
	if r.Secrets != nil {
		r.Secrets.ClearSession()
	}
}

// RegisterProvider adds a model provider.
func (r *Runtime) RegisterProvider(p provider.Provider) {
	r.Providers[p.ID()] = p
}

// Events returns a copy of emitted events.
func (r *Runtime) Events() []Event {
	r.evMu.Lock()
	defer r.evMu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

func (r *Runtime) emit(ev Event) {
	r.evMu.Lock()
	r.events = append(r.events, ev)
	r.evMu.Unlock()
	if r.OnEvent != nil {
		r.OnEvent(ev)
	}
}

func (r *Runtime) nextID(prefix string) string {
	n := r.idSeq.Add(1)
	return fmt.Sprintf("%s_%d", prefix, n)
}

// Create opens a session for an agent.
func (r *Runtime) Create(agentID string) (*session.Session, error) {
	a, err := r.Doc.Agent(agentID)
	if err != nil {
		return nil, err
	}
	if r.WorkDir == "" {
		r.WorkDir, _ = filepathAbs(".")
	}
	if err := paths.EnsureSessionDirs(r.WorkDir); err != nil {
		return nil, err
	}
	packs, boundIDs, err := r.loadAgentSkills(a)
	if err != nil {
		return nil, NewError(CodeUnknownSkill, err.Error(), false)
	}
	wc := skill.LoadWorkspaceContext(r.WorkDir)
	id := r.nextID("ses")
	s := session.New(id, a.ID)
	s.Provider = a.Model.Provider
	s.ModelID = a.Model.ID
	s.WorkDir = r.WorkDir
	s.BoundSkills = boundIDs
	s.AgentsMD = wc.AgentsMD
	s.RulesText = wc.RulesText
	initialMode := NormalizeMode(a.DefaultMode)
	if r.DefaultMode != "" {
		initialMode = NormalizeMode(r.DefaultMode)
	}
	applyModeState(s, initialMode)
	r.mu.Lock()
	r.sessions[id] = s
	r.sessionSkills[id] = packs
	r.mu.Unlock()

	policy := config.ResolvePolicy(r.Doc, a)
	ctx := context.Background()
	if err := r.invokeHooks(ctx, a, hook.BeforeSession, s, 0, nil, policy.IgnoreHookErrors); err != nil {
		return nil, NewError(CodeHookError, err.Error(), false)
	}
	if err := r.enterModeHooks(ctx, a, s, initialMode, policy.IgnoreHookErrors); err != nil {
		return nil, NewError(CodeHookError, err.Error(), false)
	}
	return s, nil
}

func filepathAbs(p string) (string, error) {
	return filepath.Abs(p) //nolint:wrapcheck // thin helper
}

// GetSession returns a session by id.
func (r *Runtime) GetSession(id string) (*session.Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	return s, ok
}

// Submit appends user input and starts or queues a drain.
func (r *Runtime) Submit(ctx context.Context, sessionID string, in session.UserInput) (string, error) {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return "", NewError(CodeInternal, "session not found", false)
	}
	parts, err := r.userMessageParts(sessionID, in)
	if err != nil {
		return "", err
	}
	s.Lock()
	if s.PendingPlanAsk {
		s.Unlock()
		return "", NewError(CodeInvalidRequest, "resolve plan ask first (edit|new)", false)
	}
	if s.Busy {
		s.Queued = append(s.Queued, in)
		s.Unlock()
		return s.LastStopReason, nil
	}
	s.Busy = true
	s.AppendItem(session.Item{
		ID: r.nextID("msg"), Type: session.ItemMessage, Role: session.RoleUser,
		Content: parts,
	})
	s.Unlock()

	reason := r.drain(ctx, s)
	r.flushQueue(ctx, s)
	return reason, nil
}

// Steer delivers input at the next step boundary.
func (r *Runtime) Steer(ctx context.Context, sessionID string, in session.UserInput) (string, error) {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return "", NewError(CodeInternal, "session not found", false)
	}
	s.Lock()
	if !s.Busy {
		s.Unlock()
		return r.Submit(ctx, sessionID, in)
	}
	// Validate attachments early so steer fails closed like submit.
	if _, err := r.userMessageParts(sessionID, in); err != nil {
		s.Unlock()
		return "", err
	}
	s.Steer = append(s.Steer, in)
	s.Unlock()
	return session.StopAwaitingInput, nil
}

// Abort interrupts the session drain.
func (r *Runtime) Abort(sessionID string) {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return
	}
	s.Lock()
	s.Aborting = true
	cancel := s.DrainCancel
	parent := s.ID
	s.Unlock()
	if cancel != nil {
		cancel()
	}

	var childCancels []context.CancelFunc
	r.mu.Lock()
	for _, c := range r.sessions {
		if c.ParentID == parent {
			c.Lock()
			c.Aborting = true
			if c.DrainCancel != nil {
				childCancels = append(childCancels, c.DrainCancel)
			}
			c.Unlock()
		}
	}
	r.mu.Unlock()
	for _, c := range childCancels {
		c()
	}
	select {
	case s.AskCh <- session.AskReject:
	default:
	}
}

func (r *Runtime) flushQueue(ctx context.Context, s *session.Session) {
	for {
		s.Lock()
		if len(s.Queued) == 0 || s.Busy {
			s.Unlock()
			return
		}
		in := s.Queued[0]
		s.Queued = s.Queued[1:]
		s.Unlock()
		parts, err := r.userMessageParts(s.ID, in)
		if err != nil {
			continue
		}
		s.Lock()
		s.Busy = true
		s.AppendItem(session.Item{
			ID: r.nextID("msg"), Type: session.ItemMessage, Role: session.RoleUser,
			Content: parts,
		})
		s.Unlock()
		r.drain(ctx, s)
	}
}

func (r *Runtime) drain(ctx context.Context, s *session.Session) string {
	ctx, cancel := context.WithCancel(ctx)
	s.Lock()
	s.Aborting = false
	s.DrainCancel = cancel
	s.Unlock()
	defer func() {
		cancel()
		s.Lock()
		s.DrainCancel = nil
		s.Aborting = false
		s.Unlock()
	}()

	agent, err := r.Doc.Agent(s.ActiveAgent)
	if err != nil {
		s.Lock()
		s.Busy = false
		s.LastStopReason = session.StopError
		s.Unlock()
		return session.StopError
	}
	policy := r.resolveSessionPolicy(s, agent)

	r.emit(NewEvent(EventDrainStarted, s.ID, map[string]any{
		"agentId": agent.ID, "model": s.ModelID, "provider": s.Provider,
	}))

	_ = r.invokeHooks(ctx, agent, hook.BeforeDrain, s, 0, nil, policy.IgnoreHookErrors)

	steps := 0
	reason := session.StopCompleted

	for {
		s.Lock()
		aborting := s.Aborting
		s.Unlock()
		if aborting || ctx.Err() != nil {
			reason = session.StopAborted
			break
		}

		r.deliverSteer(s)

		agent, err = r.Doc.Agent(s.ActiveAgent)
		if err != nil {
			reason = session.StopError
			break
		}
		policy = r.resolveSessionPolicy(s, agent)

		stop, handoff := r.step(ctx, s, agent, policy, steps)
		steps++
		if stop != "" {
			reason = stop
			break
		}
		if steps >= policy.MaxSteps {
			reason = session.StopMaxSteps
			break
		}
		_ = handoff
	}

	s.Lock()
	s.Busy = false
	s.LastStopReason = reason
	s.Unlock()

	r.emit(NewEvent(EventDrainCompleted, s.ID, map[string]any{"reason": reason}))
	_ = r.invokeHooks(ctx, agent, hook.AfterDrain, s, steps, map[string]any{"reason": reason}, policy.IgnoreHookErrors)
	return reason
}

func abortedDrain(ctx context.Context, s *session.Session) bool {
	if ctx.Err() != nil {
		return true
	}
	s.Lock()
	a := s.Aborting
	s.Unlock()
	return a
}

func (r *Runtime) deliverSteer(s *session.Session) {
	s.Lock()
	pending := append([]session.UserInput{}, s.Steer...)
	s.Steer = nil
	s.Unlock()
	for _, in := range pending {
		parts, err := r.userMessageParts(s.ID, in)
		if err != nil {
			continue
		}
		s.Lock()
		s.AppendItem(session.Item{
			ID: r.nextID("msg"), Type: session.ItemMessage, Role: session.RoleUser,
			Content: parts,
		})
		s.Unlock()
	}
}

func (r *Runtime) step(ctx context.Context, s *session.Session, agent *config.Agent, policy config.ResolvedPolicy, stepIndex int) (stopReason string, handoffOccurred bool) {
	r.emit(NewEvent(EventStepStarted, s.ID, map[string]any{"stepIndex": stepIndex, "agentId": agent.ID}))

	s.Lock()
	mode := s.Mode
	if mode == "" {
		mode = session.ModeAgent
	}
	s.Unlock()

	instructions := r.mergeInstructions(s, agent, mode)
	params := map[string]any{}
	for k, v := range policy.Params {
		params[k] = v
	}
	hookData := map[string]any{"instructions": instructions, "params": params}
	if err := r.invokeHooks(ctx, agent, hook.BeforeStep, s, stepIndex, hookData, policy.IgnoreHookErrors); err != nil {
		r.failDrain(s, CodeHookError, err.Error())
		r.emit(NewEvent(EventStepCompleted, s.ID, map[string]any{"stepIndex": stepIndex, "hadToolCalls": false}))
		return session.StopError, false
	}
	if v, ok := hookData["instructions"].(string); ok {
		instructions = v
	}
	if v, ok := hookData["params"].(map[string]any); ok {
		params = v
	}

	toolNames, toolsAll := FilterToolsByMode(mode, agent.ToolNames(), agent.ToolsIsAll(), r.Tools)
	includeTask := len(agent.Subagents) > 0 && mode == session.ModeAgent
	includeHandoff := len(agent.Handoffs) > 0 && mode == session.ModeAgent
	s.Lock()
	includeSkill := len(s.BoundSkills) > 0
	includeTodo := mode == session.ModeAgent || mode == session.ModePlan
	s.Unlock()
	snap, err := tool.BuildSnapshot(r.Tools, toolNames, toolsAll, includeTask, includeHandoff, includeSkill, includeTodo)
	if err != nil {
		r.failDrain(s, CodeUnknownTool, err.Error())
		r.emit(NewEvent(EventStepCompleted, s.ID, map[string]any{"stepIndex": stepIndex, "hadToolCalls": false}))
		return session.StopError, false
	}

	s.Lock()
	modelProvider := s.Provider
	modelID := s.ModelID
	if modelProvider == "" {
		modelProvider = agent.Model.Provider
	}
	if modelID == "" {
		modelID = agent.Model.ID
	}
	msgs := append([]session.Item{}, s.Transcript...)
	s.Unlock()

	prov, ok := r.Providers[modelProvider]
	if !ok {
		r.failDrain(s, CodeInternal, "provider not registered: "+modelProvider)
		return session.StopError, false
	}
	if transcriptHasImages(msgs) && !prov.Capabilities().Images {
		r.failDrain(s, CodeProviderCapabilityMismatch, "provider does not support images")
		r.emit(NewEvent(EventError, s.ID, map[string]any{"code": CodeProviderCapabilityMismatch, "message": "provider does not support images"}))
		r.emit(NewEvent(EventStepCompleted, s.ID, map[string]any{"stepIndex": stepIndex, "hadToolCalls": false}))
		return session.StopError, false
	}

	projected := r.maybeCompactProjection(ctx, s, prov, modelID, agent.ID, policy, msgs)
	if abortedDrain(ctx, s) {
		return session.StopAborted, false
	}

	sessionID := s.ID
	req := provider.Request{
		Model:        modelID,
		Instructions: instructions,
		Messages:     projected,
		Tools:        snap.Schemas(),
		Params:       params,
		Stream:       policy.Stream,
		AgentID:      agent.ID,
		ResolveAttachment: func(id string) (filename, mimeType, relPath string, data []byte, err error) {
			return r.resolveAttachment(sessionID, id)
		},
	}

	var resp provider.Response
	attempts := 0
	for {
		attempts++
		resp, err = prov.Complete(ctx, req, func(eventType string, data map[string]any) {
			r.emit(NewEvent(eventType, s.ID, data))
		})
		if err == nil {
			break
		}
		if abortedDrain(ctx, s) {
			return session.StopAborted, false
		}
		ze, ok := err.(*Error)
		if !ok || !ze.Retryable || attempts >= policy.Retries.MaxAttempts {
			if !ok {
				ze = NewError(CodeProviderExhausted, err.Error(), false)
			} else if attempts >= policy.Retries.MaxAttempts {
				ze = NewError(CodeProviderExhausted, ze.Message, false)
			}
			r.failDrain(s, ze.Code, ze.Message)
			r.emit(NewEvent(EventError, s.ID, map[string]any{"code": ze.Code, "message": ze.Message}))
			return session.StopError, false
		}
		backoff := time.Duration(float64(policy.Retries.BackoffMs)*pow(policy.Retries.BackoffMultiplier, attempts-1)) * time.Millisecond
		select {
		case <-ctx.Done():
			return session.StopAborted, false
		case <-time.After(backoff):
			if abortedDrain(ctx, s) {
				return session.StopAborted, false
			}
		}
	}

	r.applyUsage(s, resp.Usage)

	msg := resp.Message
	if msg.ID == "" {
		msg.ID = r.nextID("msg")
	}
	msg.Type = session.ItemMessage
	msg.Role = session.RoleAssistant
	s.Lock()
	s.AppendItem(msg)
	s.Unlock()

	calls := session.ToolCalls(msg.Content)
	if len(calls) == 0 {
		r.emit(NewEvent(EventStepCompleted, s.ID, map[string]any{"stepIndex": stepIndex, "hadToolCalls": false}))
		_ = r.invokeHooks(ctx, agent, hook.AfterStep, s, stepIndex, nil, policy.IgnoreHookErrors)
		return session.StopCompleted, false
	}

	// Parallelism decision
	sequential := policy.ToolParallelism != "parallel"
	if !sequential {
		for _, c := range calls {
			if c.Name == "handoff" {
				sequential = true
				break
			}
			args, _ := tool.ArgsMap(c.Arguments)
			entry, _ := snap.Lookup(c.Name)
			action := entry.Def.Permission.Action
			if action == "" {
				action = c.Name
			}
			res := tool.ResourceFromArgs(entry.Def.Permission, args)
			eff := permission.Resolve(toPermRules(agent.Permissions), s.EffectiveOverlays(), permission.Assertion{
				Action: action, Resource: res, ToolName: c.Name, ToolCallID: c.ToolCallID,
			})
			if eff == permission.EffectAsk {
				sequential = true
				break
			}
		}
	}

	// Reorder: non-handoff first, then handoff (when mixed)
	ordered := orderToolCalls(calls)

	results := make([]session.Item, len(ordered))
	if sequential {
		for i, c := range ordered {
			it, ho, stop := r.runToolCall(ctx, s, agent, policy, snap, c)
			results[i] = it
			if ho {
				handoffOccurred = true
				agent, _ = r.Doc.Agent(s.ActiveAgent)
				policy = config.ResolvePolicy(r.Doc, agent)
			}
			if stop != "" {
				// ask wait handled inside; only abort/error stop step early
				if stop == session.StopAborted || stop == session.StopError {
					s.Lock()
					for _, ri := range results {
						if ri.ID != "" {
							s.AppendItem(ri)
						}
					}
					s.Unlock()
					return stop, handoffOccurred
				}
			}
		}
	} else {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for i, c := range ordered {
			wg.Add(1)
			go func(i int, c session.ContentPart) {
				defer wg.Done()
				it, _, _ := r.runToolCall(ctx, s, agent, policy, snap, c)
				mu.Lock()
				results[i] = it
				mu.Unlock()
			}(i, c)
		}
		wg.Wait()
	}

	s.Lock()
	for _, ri := range results {
		if ri.Type == "" {
			continue
		}
		if ri.Name == "handoff" && !ri.IsError && ri.To != "" {
			s.AppendItem(session.Item{
				ID: r.nextID("ho"), Type: session.ItemHandoff,
				From: ri.From, To: ri.To, Reason: ri.Reason,
			})
		}
		// clear handoff meta from tool_result fields
		ri.From, ri.To, ri.Reason = "", "", ""
		s.AppendItem(ri)
	}
	s.Unlock()

	r.emit(NewEvent(EventStepCompleted, s.ID, map[string]any{"stepIndex": stepIndex, "hadToolCalls": true}))
	_ = r.invokeHooks(ctx, agent, hook.AfterStep, s, stepIndex, nil, policy.IgnoreHookErrors)
	return "", handoffOccurred
}

func orderToolCalls(calls []session.ContentPart) []session.ContentPart {
	var normal, handoffs []session.ContentPart
	for _, c := range calls {
		if c.Name == "handoff" {
			handoffs = append(handoffs, c)
		} else {
			normal = append(normal, c)
		}
	}
	return append(normal, handoffs...)
}

func (r *Runtime) runToolCall(ctx context.Context, s *session.Session, agent *config.Agent, policy config.ResolvedPolicy, snap tool.Snapshot, c session.ContentPart) (session.Item, bool, string) {
	entry, ok := snap.Lookup(c.Name)
	if !ok {
		return r.toolResultItem(c, tool.TextResult("unknown tool", true, CodeUnknownTool)), false, ""
	}

	args, err := tool.ArgsMap(c.Arguments)
	if err != nil {
		return r.toolResultItem(c, tool.TextResult("invalid args", true, CodeToolInvalidArgs)), false, ""
	}
	if err := tool.ValidateArgs(entry.Def.InputSchema, args); err != nil {
		return r.toolResultItem(c, tool.TextResult(err.Error(), true, CodeToolInvalidArgs)), false, ""
	}

	action := entry.Def.Permission.Action
	if action == "" {
		action = c.Name
	}
	resource := tool.ResourceFromArgs(entry.Def.Permission, args)

	overlays := s.EffectiveOverlays()
	eff := permission.Resolve(toPermRules(agent.Permissions), overlays, permission.Assertion{
		Action: action, Resource: resource, ToolName: c.Name, ToolCallID: c.ToolCallID,
	})

	switch eff {
	case permission.EffectAllow:
		// proceed to execute
	case permission.EffectAsk:
		r.emit(NewEvent(EventPermissionAsk, s.ID, map[string]any{
			"action": action, "resource": resource, "toolCallId": c.ToolCallID,
		}))
		decision, timedOut := r.waitAsk(ctx, s, policy, action, resource, c.ToolCallID)
		r.emit(NewEvent(EventPermissionResolved, s.ID, map[string]any{
			"decision": string(decision), "toolCallId": c.ToolCallID, "timedOut": timedOut,
		}))
		switch decision {
		case session.AskReject:
			s.Lock()
			s.Overlays = append(s.Overlays, permission.Rule{Action: action, Resource: resource, Effect: permission.EffectDeny})
			s.Unlock()
			return r.rejectedAskResult(c, action, resource, args), false, ""
		case session.AskAlways:
			s.Lock()
			s.Overlays = append(s.Overlays, permission.Rule{Action: action, Resource: resource, Effect: permission.EffectAllow})
			s.Unlock()
		case session.AskOnce:
			// allow once
		}
	case permission.EffectDeny:
		return r.deniedResult(c, action, resource), false, ""
	default:
		// Unknown / invalid effect: fail closed (same as deny).
		return r.deniedResult(c, action, resource), false, ""
	}

	toolData := map[string]any{"name": c.Name, "args": args}
	if err := r.invokeHooks(ctx, agent, hook.BeforeTool, s, 0, toolData, policy.IgnoreHookErrors); err != nil {
		return r.toolResultItem(c, tool.TextResult(err.Error(), true, CodeHookError)), false, ""
	}
	if v, ok := toolData["args"].(map[string]any); ok {
		args = v
	}

	r.emit(NewEvent(EventToolStarted, s.ID, map[string]any{"toolCallId": c.ToolCallID, "name": c.Name}))

	timeout := policy.ToolTimeoutMs
	if entry.Def.TimeoutMs > 0 {
		timeout = entry.Def.TimeoutMs
	}
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	tctx = context.WithValue(tctx, ctxKeySessionID, s.ID)

	var result tool.Result
	var handoff bool
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				result = tool.TextResult(fmt.Sprint(rec), true, CodeToolPanic)
			}
		}()
		result, err = entry.Run(tctx, c.Name, args)
		if err != nil {
			result = tool.TextResult(err.Error(), true, CodeToolPanic)
		}
	}()
	if tctx.Err() == context.DeadlineExceeded && !result.IsError {
		result = tool.TextResult("timeout", true, CodeToolTimeout)
	}

	if c.Name == "handoff" && !result.IsError {
		handoff = true
	}

	_ = r.invokeHooks(ctx, agent, hook.AfterTool, s, 0, map[string]any{"name": c.Name, "result": result}, policy.IgnoreHookErrors)
	r.emit(NewEvent(EventToolCompleted, s.ID, map[string]any{"toolCallId": c.ToolCallID, "isError": result.IsError}))

	item := r.toolResultItem(c, result)
	if handoff {
		from, to, reason := handoffMeta(result)
		item.From = from
		item.To = to
		item.Reason = reason
	}
	return item, handoff, ""
}

func handoffMeta(res tool.Result) (from, to, reason string) {
	for _, p := range res.Content {
		if p.Type == "json" {
			if m, ok := p.Value.(map[string]any); ok {
				from, _ = m["from"].(string)
				to, _ = m["to"].(string)
				reason, _ = m["reason"].(string)
			}
		}
	}
	return
}

func (r *Runtime) waitAsk(ctx context.Context, s *session.Session, policy config.ResolvedPolicy, action, resource, toolCallID string) (session.AskDecision, bool) {
	if r.Ask != nil {
		// Auto resolver (conformance): may block until timeout internally
		if policy.AskTimeoutMs > 0 {
			done := make(chan session.AskDecision, 1)
			go func() {
				done <- r.Ask(s.ID, action, resource, toolCallID)
			}()
			select {
			case d := <-done:
				return d, false
			case <-time.After(time.Duration(policy.AskTimeoutMs) * time.Millisecond):
				return session.AskReject, true
			case <-ctx.Done():
				return session.AskReject, false
			}
		}
		return r.Ask(s.ID, action, resource, toolCallID), false
	}

	if policy.AskTimeoutMs > 0 {
		select {
		case d := <-s.AskCh:
			return d, false
		case <-time.After(time.Duration(policy.AskTimeoutMs) * time.Millisecond):
			return session.AskReject, true
		case <-ctx.Done():
			return session.AskReject, false
		}
	}
	select {
	case d := <-s.AskCh:
		return d, false
	case <-ctx.Done():
		return session.AskReject, false
	}
}

func (r *Runtime) deniedResult(c session.ContentPart, action, resource string) session.Item {
	res := tool.JSONResult(
		"permission_denied: "+action+" "+resource,
		map[string]any{"error": CodePermissionDenied, "action": action, "resource": resource},
		true,
		CodePermissionDenied,
	)
	return r.toolResultItem(c, res)
}

const largeWriteContentRunes = 2000

// rejectedWriteGuidance builds the user-reject tool message for edit/write actions.
func rejectedWriteGuidance(action, resource string, args map[string]any) (text string, contentLarge bool, known bool) {
	content, hasContent := writeContentFromArgs(args)
	known = hasContent
	if hasContent {
		contentLarge = len([]rune(content)) > largeWriteContentRunes
	} else if action == "edit" {
		// Write-like edit without visible content — treat as large to avoid dumps.
		contentLarge = true
		known = false
	}

	var b strings.Builder
	b.WriteString("permission_denied: user rejected writing ")
	b.WriteString(resource)
	b.WriteString(". Do not retry write/create or propose creating a file. ")
	b.WriteString("Fulfill the request another way if possible, or respond directly in chat.")
	if contentLarge {
		b.WriteString(" Content is large — ask the user whether they want the result pasted in chat before sending it.")
	} else if known {
		b.WriteString(" Content is small enough to include directly in your reply if that fulfills the request.")
	}
	return b.String(), contentLarge, known
}

// rejectedToolGuidance builds the user-reject message for non-edit tool calls (shell, read, …).
func rejectedToolGuidance(action, resource string) string {
	var b strings.Builder
	b.WriteString("permission_denied: user rejected this ")
	b.WriteString(action)
	b.WriteString(" tool call (")
	b.WriteString(resource)
	b.WriteString("). Do not retry the same call. Continue without it, or answer directly in chat.")
	return b.String()
}

func writeContentFromArgs(args map[string]any) (string, bool) {
	if args == nil {
		return "", false
	}
	for _, key := range []string{"content", "text", "body"} {
		if v, ok := args[key]; ok {
			switch t := v.(type) {
			case string:
				return t, true
			}
		}
	}
	return "", false
}

func (r *Runtime) rejectedAskResult(c session.ContentPart, action, resource string, args map[string]any) session.Item {
	payload := map[string]any{
		"error":    CodePermissionDenied,
		"action":   action,
		"resource": resource,
		"rejected": true,
	}
	var text string
	if action == "edit" {
		var contentLarge, known bool
		text, contentLarge, known = rejectedWriteGuidance(action, resource, args)
		if known || contentLarge {
			payload["contentLarge"] = contentLarge
		}
	} else {
		text = rejectedToolGuidance(action, resource)
	}
	res := tool.JSONResult(text, payload, true, CodePermissionDenied)
	return r.toolResultItem(c, res)
}

func (r *Runtime) toolResultItem(c session.ContentPart, res tool.Result) session.Item {
	parts := make([]session.ContentPart, len(res.Content))
	for i, p := range res.Content {
		parts[i] = session.ContentPart{Type: p.Type, Text: p.Text, Value: p.Value}
	}
	return session.Item{
		ID:         r.nextID("tr"),
		Type:       session.ItemToolResult,
		ToolCallID: c.ToolCallID,
		Name:       c.Name,
		IsError:    res.IsError,
		Content:    parts,
		CreatedAt:  time.Now().UTC(),
	}
}

func (r *Runtime) failDrain(s *session.Session, code, message string) {
	s.Lock()
	s.AppendItem(session.Item{
		ID: r.nextID("err"), Type: session.ItemError, Code: code, Message: message,
	})
	s.Unlock()
}

func (r *Runtime) invokeHooks(ctx context.Context, agent *config.Agent, point string, s *session.Session, step int, data map[string]any, ignore bool) error {
	if agent.Hooks == nil {
		return nil
	}
	names := agent.Hooks[point]
	if len(names) == 0 {
		return nil
	}
	return r.Hooks.Invoke(ctx, names, hook.Context{
		SessionID: s.ID, AgentID: agent.ID, StepIndex: step, Point: point, Data: data,
	}, ignore)
}

func toPermRules(in []config.PermissionRule) []permission.Rule {
	out := make([]permission.Rule, len(in))
	for i, r := range in {
		out[i] = permission.Rule{Action: r.Action, Resource: r.Resource, Effect: r.Effect}
	}
	return out
}

func pow(base float64, exp int) float64 {
	r := 1.0
	for i := 0; i < exp; i++ {
		r *= base
	}
	return r
}
