package skill

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kasra-sh/zendawn-go/paths"
)

// WorkspaceContext is cached AGENTS.md + local rules text.
type WorkspaceContext struct {
	AgentsMD  string
	RulesText string
}

// LoadWorkspaceContext loads AGENTS.md and local rules once (budgeted).
func LoadWorkspaceContext(workdir string) WorkspaceContext {
	var wc WorkspaceContext
	for _, p := range paths.AgentsMDCandidates(workdir) {
		if b, ok := readBudgeted(p, paths.ContextBudgetBytes); ok {
			wc.AgentsMD = b
			break
		}
	}
	wc.RulesText = loadRules(workdir, paths.ContextBudgetBytes)
	return wc
}

func readBudgeted(path string, budget int) (string, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return "", false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	s := string(raw)
	if len(s) > budget {
		s = s[:budget] + "\n\n[truncated: exceeded 64KiB budget]\n"
	}
	return strings.TrimRight(s, "\n"), true
}

func loadRules(workdir string, budget int) string {
	var files []string
	for _, root := range paths.RuleRoots(workdir) {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".md" && ext != ".mdc" {
				return nil
			}
			files = append(files, path)
			return nil
		})
	}
	sort.Strings(files)
	var b strings.Builder
	remaining := budget
	for _, path := range files {
		if remaining <= 0 {
			break
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		body := stripOptionalFrontmatter(string(raw))
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		rel, err := filepath.Rel(workdir, path)
		if err != nil {
			rel = path
		}
		header := "\n\n[rule: " + rel + "]\n"
		chunk := header + body
		if len(chunk) > remaining {
			chunk = chunk[:remaining] + "\n\n[truncated: rules budget]\n"
			b.WriteString(chunk)
			break
		}
		b.WriteString(chunk)
		remaining -= len(chunk)
	}
	return strings.TrimPrefix(b.String(), "\n")
}

func stripOptionalFrontmatter(s string) string {
	s = strings.TrimPrefix(s, "\ufeff")
	if !strings.HasPrefix(s, "---") {
		return s
	}
	rest := s[3:]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else {
		return s
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return s
	}
	body := rest[end+4:]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	return body
}
