package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/paths"
)

func TestActivateSkillFuzzy(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	wd := filepath.Join(tmp, "ws")
	_ = os.MkdirAll(filepath.Join(wd, ".zendawn", "skills", "demo"), 0o755)
	_ = os.WriteFile(filepath.Join(wd, ".zendawn", "skills", "demo", "SKILL.md"),
		[]byte("---\ndescription: Demo\n---\n\nBody\n"), 0o644)

	rt := New(singleDoc(&config.Agent{
		ID: "a", Instructions: "hi",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}))
	rt.WorkDir = wd
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	ar, err := rt.ActivateSkill(s.ID, "dem0", false)
	if err != nil || ar.ID != "demo" || !ar.Guessed {
		t.Fatalf("%+v %v", ar, err)
	}
	s.Lock()
	active := s.ActiveSkills["demo"]
	s.Unlock()
	if !active {
		t.Fatal("not active")
	}
	_, err = rt.ActivateSkill(s.ID, "nope", false)
	if err == nil {
		t.Fatal("expected error")
	}
}
