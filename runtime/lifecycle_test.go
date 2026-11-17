package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/hook"
	"github.com/kasra-sh/zendawn-go/paths"
	"github.com/kasra-sh/zendawn-go/session"
)

func TestCreateEnsuresDirsAndPhaseHooks(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	wd := filepath.Join(tmp, "ws")
	_ = os.MkdirAll(wd, 0o755)

	skillDir := filepath.Join(wd, ".zendawn", "skills", "demo")
	_ = os.MkdirAll(skillDir, 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Demo\n---\n\nBody\n"), 0o644)

	var order []string
	doc := &config.Document{Single: &config.Agent{
		ID:           "a",
		Instructions: "hi",
		Model:        config.ModelConfig{Provider: "mock", ID: "m"},
		Skills:       []string{"demo"},
		Hooks: map[string][]string{
			hook.BeforeSession:   {"h"},
			hook.BeforeExecution: {"h"},
			hook.BeforePlan:      {"h"},
			hook.AfterPlan:       {"h"},
		},
	}}
	// load.go path: Document needs Agent() — use Single via helper
	rt := New(singleDoc(doc.Single))
	rt.WorkDir = wd
	rt.Hooks.Register("h", func(ctx context.Context, hc hook.Context) error {
		order = append(order, hc.Point)
		return nil
	})

	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.WorkspaceSkills(wd)); err != nil {
		t.Fatal(err)
	}
	if len(order) < 2 || order[0] != hook.BeforeSession || order[1] != hook.BeforeExecution {
		t.Fatalf("hook order on create: %v", order)
	}

	s.Lock()
	s.AppendItem(session.Item{
		Type: session.ItemMessage, Role: session.RoleAssistant,
		Content: []session.ContentPart{{Type: session.PartText, Text: "# Plan\n\n## Todos\n\n- [ ] id: x — Do x\n"}},
	})
	s.Unlock()

	if err := rt.SetMode(context.Background(), s.ID, session.ModePlan); err != nil {
		t.Fatal(err)
	}
	if err := rt.SetMode(context.Background(), s.ID, session.ModeAgent); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(paths.PlanFile(s.ID))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "" || !strings.Contains(string(b), "Plan") {
		t.Fatalf("plan file: %q", b)
	}
}

func singleDoc(a *config.Agent) *config.Document {
	return &config.Document{Single: a}
}
