package hook

import (
	"context"
	"sync"
)

// Point names.
const (
	BeforeSession    = "before_session"
	AfterSession     = "after_session"
	BeforePlan       = "before_plan"
	AfterPlan        = "after_plan"
	BeforeExecution  = "before_execution"
	AfterExecution   = "after_execution"
	BeforeDrain      = "before_drain"
	AfterDrain       = "after_drain"
	BeforeStep       = "before_step"
	AfterStep        = "after_step"
	BeforeTool       = "before_tool"
	AfterTool        = "after_tool"
	OnAsk            = "on_ask"
	OnError          = "on_error"
	OnEvent          = "on_event"
)

// Context is passed to hooks.
type Context struct {
	SessionID string
	AgentID   string
	StepIndex int
	Point     string
	Data      map[string]any
}

// Func is a hook implementation.
type Func func(ctx context.Context, hc Context) error

// Registry stores named hooks.
type Registry struct {
	mu    sync.RWMutex
	hooks map[string]Func
}

// NewRegistry creates an empty hook registry.
func NewRegistry() *Registry {
	return &Registry{hooks: map[string]Func{}}
}

// Register adds a named hook.
func (r *Registry) Register(name string, fn Func) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[name] = fn
}

// Get returns a hook by name.
func (r *Registry) Get(name string) (Func, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.hooks[name]
	return fn, ok
}

// Invoke runs named hooks in order.
func (r *Registry) Invoke(ctx context.Context, names []string, hc Context, ignoreErrors bool) error {
	for _, name := range names {
		fn, ok := r.Get(name)
		if !ok {
			return errUnknown(name)
		}
		if err := fn(ctx, hc); err != nil {
			if ignoreErrors {
				continue
			}
			return err
		}
	}
	return nil
}

type unknownHookError struct{ name string }

func (e unknownHookError) Error() string { return "unknown_hook: " + e.name }

func errUnknown(name string) error { return unknownHookError{name: name} }
