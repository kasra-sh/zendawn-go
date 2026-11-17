package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/paths"
)

func TestLoadWorkspaceContext(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("# Agents\nUse tabs."), 0o644)
	_ = os.MkdirAll(filepath.Join(tmp, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".cursor", "rules", "a.mdc"), []byte("---\ndescription: x\n---\n\nRule A body"), 0o644)
	_ = os.MkdirAll(filepath.Join(tmp, ".zendawn", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".zendawn", "rules", "b.md"), []byte("Rule B body"), 0o644)

	wc := LoadWorkspaceContext(tmp)
	if !strings.Contains(wc.AgentsMD, "Use tabs") {
		t.Fatalf("agents: %q", wc.AgentsMD)
	}
	if !strings.Contains(wc.RulesText, "Rule A body") || !strings.Contains(wc.RulesText, "Rule B body") {
		t.Fatalf("rules: %q", wc.RulesText)
	}
}

func TestDiscoverAndLoadBound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(paths.EnvConfigDir, filepath.Join(tmp, "cfg"))
	_ = paths.EnsureSessionDirs(tmp)
	skillDir := filepath.Join(paths.WorkspaceSkills(tmp), "auto")
	_ = os.MkdirAll(skillDir, 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Auto skill\n---\n\nBody\n"), 0o644)

	packs, ids, err := LoadBound(tmp, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "auto" || packs["auto"] == nil {
		t.Fatalf("discover: %v %v", ids, packs)
	}
}
