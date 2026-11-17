package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/paths"
	"github.com/kasra-sh/zendawn-go/session"
)

func TestDefaultModeConfigurable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	rt := New(singleDoc(&config.Agent{
		ID:           "a",
		Instructions: "hi",
		Model:        config.ModelConfig{Provider: "mock", ID: "m"},
		DefaultMode:  session.ModePlan,
	}))
	rt.WorkDir = tmp
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != session.ModePlan {
		t.Fatalf("mode=%q", s.Mode)
	}
	rt2 := New(singleDoc(&config.Agent{
		ID: "a", Instructions: "hi",
		Model: config.ModelConfig{Provider: "mock", ID: "m"}, DefaultMode: session.ModeAsk,
	}))
	rt2.WorkDir = tmp
	rt2.DefaultMode = session.ModeAgent // host override
	s2, err := rt2.Create("a")
	if err != nil || s2.Mode != session.ModeAgent {
		t.Fatalf("override: %v mode=%q", err, s2.Mode)
	}
}

func TestAutoDetectAgentsRulesSkills(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	wd := filepath.Join(tmp, "ws")
	_ = os.MkdirAll(wd, 0o755)
	_ = os.WriteFile(filepath.Join(wd, "AGENTS.md"), []byte("AGENTS-BODY"), 0o644)
	_ = os.MkdirAll(filepath.Join(wd, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(wd, ".cursor", "rules", "style.mdc"), []byte("---\ndesc: s\n---\n\nRULE-BODY"), 0o644)
	skillDir := filepath.Join(wd, ".zendawn", "skills", "foo")
	_ = os.MkdirAll(skillDir, 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Foo skill\nalways: true\n---\n\nFOO-BODY\n"), 0o644)

	rt := New(singleDoc(&config.Agent{
		ID:           "a",
		Instructions: "BASE",
		Model:        config.ModelConfig{Provider: "mock", ID: "m"},
	}))
	rt.WorkDir = wd
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	if s.AgentsMD == "" || !strings.Contains(s.AgentsMD, "AGENTS-BODY") {
		t.Fatalf("AgentsMD: %q", s.AgentsMD)
	}
	if !strings.Contains(s.RulesText, "RULE-BODY") {
		t.Fatalf("RulesText: %q", s.RulesText)
	}
	if len(s.BoundSkills) != 1 || s.BoundSkills[0] != "foo" {
		t.Fatalf("Bound: %v", s.BoundSkills)
	}
	merged := rt.mergeInstructions(s, &config.Agent{ID: "a", Instructions: "BASE"}, session.ModeAgent)
	for _, want := range []string{"BASE", "AGENTS-BODY", "RULE-BODY", "Foo skill", "FOO-BODY"} {
		if !strings.Contains(merged, want) {
			t.Fatalf("merged missing %q:\n%s", want, merged)
		}
	}
}
