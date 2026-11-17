package paths

import (
	"os"
	"path/filepath"
)

const (
	EnvConfigDir = "ZENDAWN_CONFIG_DIR"
	DMZName      = ".zendawn"
	// ContextBudgetBytes caps AGENTS.md and the combined local-rules injection.
	ContextBudgetBytes = 64 * 1024
)

// WorkspaceDMZ returns {workdir}/.zendawn.
func WorkspaceDMZ(workdir string) string {
	return filepath.Join(workdir, DMZName)
}

// WorkspaceSkills returns {workdir}/.zendawn/skills.
func WorkspaceSkills(workdir string) string {
	return filepath.Join(WorkspaceDMZ(workdir), "skills")
}

// WorkspaceSpecs returns {workdir}/.zendawn/specs.
func WorkspaceSpecs(workdir string) string {
	return filepath.Join(WorkspaceDMZ(workdir), "specs")
}

// WorkspaceUploads returns {workdir}/.zendawn/uploads.
func WorkspaceUploads(workdir string) string {
	return filepath.Join(WorkspaceDMZ(workdir), "uploads")
}

// UploadDir returns {workdir}/.zendawn/uploads/{attachmentId}.
func UploadDir(workdir, attachmentID string) string {
	return filepath.Join(WorkspaceUploads(workdir), attachmentID)
}

// WorkspaceRules returns {workdir}/.zendawn/rules.
func WorkspaceRules(workdir string) string {
	return filepath.Join(WorkspaceDMZ(workdir), "rules")
}

// AgentsSkills returns {workdir}/.agents/skills.
func AgentsSkills(workdir string) string {
	return filepath.Join(workdir, ".agents", "skills")
}

// CursorSkills returns {workdir}/.cursor/skills.
func CursorSkills(workdir string) string {
	return filepath.Join(workdir, ".cursor", "skills")
}

// CursorRules returns {workdir}/.cursor/rules.
func CursorRules(workdir string) string {
	return filepath.Join(workdir, ".cursor", "rules")
}

// AgentsMDCandidates returns AGENTS.md paths in load order.
func AgentsMDCandidates(workdir string) []string {
	return []string{
		filepath.Join(workdir, "AGENTS.md"),
		filepath.Join(WorkspaceDMZ(workdir), "AGENTS.md"),
	}
}

// AutoSkillRoots returns workdir skill roots scanned for auto-discovery (before skillsDirs/global).
func AutoSkillRoots(workdir string) []string {
	return []string{
		WorkspaceSkills(workdir),
		AgentsSkills(workdir),
		CursorSkills(workdir),
	}
}

// RuleRoots returns directories that may contain local rules.
func RuleRoots(workdir string) []string {
	return []string{
		WorkspaceRules(workdir),
		CursorRules(workdir),
	}
}

// GlobalConfigDir resolves the user-level zendawn config root.
func GlobalConfigDir() string {
	if v := os.Getenv(EnvConfigDir); v != "" {
		return filepath.Clean(v)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "zendawn")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".config", "zendawn")
	}
	return filepath.Join(home, ".config", "zendawn")
}

// GlobalPlans returns {global}/plans.
func GlobalPlans() string {
	return filepath.Join(GlobalConfigDir(), "plans")
}

// GlobalSkills returns {global}/skills.
func GlobalSkills() string {
	return filepath.Join(GlobalConfigDir(), "skills")
}

// PlanFile returns {global}/plans/{sessionID}.plan.md.
func PlanFile(sessionID string) string {
	return filepath.Join(GlobalPlans(), sessionID+".plan.md")
}

// EnsureSessionDirs creates DMZ + global plans/skills trees.
func EnsureSessionDirs(workdir string) error {
	for _, d := range []string{
		WorkspaceSkills(workdir),
		WorkspaceSpecs(workdir),
		WorkspaceUploads(workdir),
		GlobalPlans(),
		GlobalSkills(),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// WritePlan writes plan markdown to the global plans dir (overwrite).
func WritePlan(sessionID, body string) error {
	if body == "" {
		return nil
	}
	if err := os.MkdirAll(GlobalPlans(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(PlanFile(sessionID), []byte(body), 0o644)
}
