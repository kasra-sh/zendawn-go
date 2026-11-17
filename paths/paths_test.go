package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSessionDirsAndPlan(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvConfigDir, filepath.Join(tmp, "cfg"))
	wd := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSessionDirs(wd); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{WorkspaceSkills(wd), WorkspaceSpecs(wd), WorkspaceUploads(wd), GlobalPlans(), GlobalSkills()} {
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			t.Fatalf("missing dir %s: %v", p, err)
		}
	}
	if err := WritePlan("ses_1", "# Plan\n"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(PlanFile("ses_1"))
	if err != nil || string(b) != "# Plan\n" {
		t.Fatalf("plan write: %v %q", err, b)
	}
}

func TestGlobalConfigDirPrecedence(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvConfigDir, filepath.Join(tmp, "override"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg"))
	if got := GlobalConfigDir(); got != filepath.Join(tmp, "override") {
		t.Fatalf("got %s", got)
	}
}
