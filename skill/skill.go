package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kasra-sh/zendawn-go/paths"
)

// Pack is a loaded SKILL.md.
type Pack struct {
	ID          string
	Name        string
	Description string
	Always      bool
	AutoInvoke  bool // model may proactive-load when helpful (default true)
	Body        string
	Path        string
}

// Frontmatter is the YAML header of SKILL.md.
type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Always      bool   `yaml:"always"`
	AutoInvoke  *bool  `yaml:"autoInvoke"` // nil → default true
}

// Roots returns skill search roots in order: auto workdir roots, skillsDirs, global.
func Roots(workdir string, skillsDirs []string) []string {
	out := append([]string{}, paths.AutoSkillRoots(workdir)...)
	out = append(out, skillsDirs...)
	out = append(out, paths.GlobalSkills())
	return out
}

// Discover scans auto skill roots (and optional skillsDirs) for SKILL.md packs.
// Invalid packs are skipped. First-seen id wins.
func Discover(workdir string, skillsDirs []string) map[string]*Pack {
	out := map[string]*Pack{}
	seenRoots := map[string]bool{}
	for _, root := range append(paths.AutoSkillRoots(workdir), skillsDirs...) {
		if root == "" || seenRoots[root] {
			continue
		}
		seenRoots[root] = true
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			id := e.Name()
			if _, exists := out[id]; exists {
				continue
			}
			p := filepath.Join(root, id, "SKILL.md")
			pack, err := LoadFile(id, p)
			if err != nil {
				continue
			}
			out[id] = pack
		}
	}
	return out
}

// LoadBound unions Discover with ResolveAll for config-listed ids.
// Config-listed missing ids return unknown_skill.
func LoadBound(workdir string, skillsDirs []string, configIDs []string) (map[string]*Pack, []string, error) {
	packs := Discover(workdir, skillsDirs)
	for _, id := range configIDs {
		if _, ok := packs[id]; ok {
			continue
		}
		p, err := Resolve(workdir, skillsDirs, id)
		if err != nil {
			return nil, nil, err
		}
		packs[id] = p
	}
	ids := make([]string, 0, len(packs))
	for id := range packs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return packs, ids, nil
}

// Resolve finds a skill id under roots (first match).
func Resolve(workdir string, skillsDirs []string, id string) (*Pack, error) {
	for _, root := range Roots(workdir, skillsDirs) {
		p := filepath.Join(root, id, "SKILL.md")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			pack, err := LoadFile(id, p)
			if err != nil {
				return nil, err
			}
			return pack, nil
		}
	}
	return nil, fmt.Errorf("unknown_skill: %s", id)
}

// ResolveAll loads every id; fails on first missing.
func ResolveAll(workdir string, skillsDirs []string, ids []string) (map[string]*Pack, error) {
	out := make(map[string]*Pack, len(ids))
	for _, id := range ids {
		p, err := Resolve(workdir, skillsDirs, id)
		if err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, nil
}

// LoadFile parses a SKILL.md at path.
func LoadFile(id, path string) (*Pack, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm, body, err := splitFrontmatter(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(fm.Description) == "" {
		return nil, fmt.Errorf("%s: description is required", path)
	}
	name := fm.Name
	if name == "" {
		name = id
	}
	autoInvoke := true
	if fm.AutoInvoke != nil {
		autoInvoke = *fm.AutoInvoke
	}
	return &Pack{
		ID:          id,
		Name:        name,
		Description: fm.Description,
		Always:      fm.Always,
		AutoInvoke:  autoInvoke,
		Body:        strings.TrimSpace(body),
		Path:        path,
	}, nil
}

func splitFrontmatter(s string) (Frontmatter, string, error) {
	s = strings.TrimPrefix(s, "\ufeff")
	if !strings.HasPrefix(s, "---") {
		return Frontmatter{}, "", fmt.Errorf("missing YAML frontmatter")
	}
	rest := s[3:]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Frontmatter{}, "", fmt.Errorf("unclosed frontmatter")
	}
	yamlBlock := rest[:end]
	body := rest[end+4:]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return Frontmatter{}, "", err
	}
	return fm, body, nil
}

// CatalogBlock builds progressive-disclosure catalog text.
// Instructs the model when to load skills (mention / similar name / autoInvoke).
func CatalogBlock(packs map[string]*Pack, ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[skills catalog]\n")
	b.WriteString("Use the skill tool to load a bound skill when: (1) the user asks to use a skill or names one (including a similar/misspelled id), or (2) a skill is marked autoInvoke and would clearly help even if the user did not name it. Do not proactive-load skills marked manual.\n")
	for _, id := range ids {
		p := packs[id]
		if p == nil {
			continue
		}
		b.WriteString("- ")
		b.WriteString(id)
		if p.Name != "" && p.Name != id {
			b.WriteString(" (")
			b.WriteString(p.Name)
			b.WriteString(")")
		}
		b.WriteString(": ")
		b.WriteString(p.Description)
		if p.AutoInvoke {
			b.WriteString(" [autoInvoke]")
		} else {
			b.WriteString(" [manual]")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// BodiesBlock appends always + active skill bodies.
func BodiesBlock(packs map[string]*Pack, ids []string, active map[string]bool) string {
	var b strings.Builder
	for _, id := range ids {
		p := packs[id]
		if p == nil {
			continue
		}
		if !p.Always && !active[id] {
			continue
		}
		b.WriteString("\n\n[skill: ")
		b.WriteString(p.Name)
		b.WriteString("]\n")
		b.WriteString(p.Body)
	}
	return b.String()
}
