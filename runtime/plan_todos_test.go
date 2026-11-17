package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/paths"
	"github.com/kasra-sh/zendawn-go/session"
)

func TestLeavePlanAbandonsWithoutTodos(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	rt := New(singleDoc(&config.Agent{
		ID: "a", Instructions: "hi",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}))
	rt.WorkDir = tmp
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.SetMode(context.Background(), s.ID, session.ModePlan); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	s.PlanMarkdown = "# Plan\n\nNo todos here.\n"
	s.Unlock()
	if err := rt.SetMode(context.Background(), s.ID, session.ModeAgent); err != nil {
		t.Fatalf("todo-less leave should abandon, got %v", err)
	}
	s.Lock()
	if s.Mode != session.ModeAgent || s.PlanMarkdown != "" || len(s.PlanTodos) != 0 || s.CurrentPlanID != "" {
		t.Fatalf("mode=%s md=%q todos=%+v id=%s", s.Mode, s.PlanMarkdown, s.PlanTodos, s.CurrentPlanID)
	}
	s.Unlock()
}

func TestLeavePlanCommitsWithTodos(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	rt := New(singleDoc(&config.Agent{
		ID: "a", Instructions: "hi",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}))
	rt.WorkDir = tmp
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.SetMode(context.Background(), s.ID, session.ModePlan); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	s.PlanMarkdown = "# Plan\n\n## Todos\n\n- [ ] id: one — Do it\n"
	s.Unlock()
	if err := rt.SetMode(context.Background(), s.ID, session.ModeAgent); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if s.Mode != session.ModeAgent || len(s.PlanTodos) != 1 || s.CurrentPlanID == "" {
		t.Fatalf("mode=%s todos=%+v id=%s", s.Mode, s.PlanTodos, s.CurrentPlanID)
	}
	s.Unlock()
	b, err := os.ReadFile(paths.PlanFile(s.ID))
	if err != nil || !strings.Contains(string(b), "## Todos") {
		t.Fatalf("file: %v %s", err, b)
	}
}

func TestPlanAskEditVsNew(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	rt := New(singleDoc(&config.Agent{
		ID: "a", Instructions: "hi",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}))
	rt.WorkDir = tmp
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	s.Lock()
	s.PlanMarkdown = "# Old\n\n## Todos\n\n- [ ] id: a — A\n"
	s.PlanTodos = []session.PlanTodo{{ID: "a", Content: "A"}}
	s.Unlock()
	if err := rt.SetMode(context.Background(), s.ID, session.ModePlan); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	pending := s.PendingPlanAsk
	s.Unlock()
	if !pending {
		t.Fatal("expected PendingPlanAsk")
	}
	if err := rt.ResolvePlanAsk(s.ID, session.PlanAskNew); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if s.PendingPlanAsk || s.PlanMarkdown != "" || len(s.Plans) == 0 {
		t.Fatalf("after new: pending=%v md=%q plans=%d", s.PendingPlanAsk, s.PlanMarkdown, len(s.Plans))
	}
	s.Unlock()
}

func TestTodoComplete(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	rt := New(singleDoc(&config.Agent{
		ID: "a", Instructions: "hi",
		Model:       config.ModelConfig{Provider: "mock", ID: "m"},
		DefaultMode: session.ModeAgent,
	}))
	rt.WorkDir = tmp
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	s.Lock()
	s.PlanMarkdown = "# P\n"
	s.PlanTodos = []session.PlanTodo{{ID: "one", Content: "One"}, {ID: "two", Content: "Two"}}
	s.CurrentPlanID = "p1"
	s.Plans = []session.StoredPlan{{ID: "p1", Title: "P", Body: "# P\n", Todos: s.PlanTodos}}
	s.Unlock()

	res, err := rt.execTodo(context.WithValue(context.Background(), ctxKeySessionID, s.ID), "todo", map[string]any{
		"action": "complete", "id": "on",
	})
	if err != nil || res.IsError {
		t.Fatalf("%+v %v", res, err)
	}
	s.Lock()
	if !s.PlanTodos[0].Done || s.PlanTodos[1].Done {
		t.Fatalf("%+v", s.PlanTodos)
	}
	s.Unlock()
}
