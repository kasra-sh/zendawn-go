package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kasra-sh/zendawn-go/paths"
)

func TestResolveSearchOrder(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	wd := filepath.Join(tmp, "ws")
	extra := filepath.Join(tmp, "extra")
	_ = paths.EnsureSessionDirs(wd)
	_ = os.MkdirAll(filepath.Join(extra, "demo"), 0o755)
	_ = os.MkdirAll(filepath.Join(paths.GlobalSkills(), "demo"), 0o755)

	writeSkill := func(dir string, body string) {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: "+body+"\n---\n\n# "+body+"\n"), 0o644)
	}
	writeSkill(filepath.Join(paths.WorkspaceSkills(wd), "demo"), "dmz")
	writeSkill(filepath.Join(extra, "demo"), "extra")
	writeSkill(filepath.Join(paths.GlobalSkills(), "demo"), "global")

	p, err := Resolve(wd, []string{extra}, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if p.Description != "dmz" {
		t.Fatalf("want dmz, got %q", p.Description)
	}

	_ = os.Remove(filepath.Join(paths.WorkspaceSkills(wd), "demo", "SKILL.md"))
	p, err = Resolve(wd, []string{extra}, "demo")
	if err != nil || p.Description != "extra" {
		t.Fatalf("extra: %v %v", err, p)
	}
}
