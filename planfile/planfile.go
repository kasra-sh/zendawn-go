package planfile

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Todo is one plan checklist item.
type Todo struct {
	ID      string `json:"id" yaml:"id"`
	Content string `json:"content" yaml:"content"`
	Done    bool   `json:"done" yaml:"done"`
}

type fmTodo struct {
	ID      string `yaml:"id"`
	Content string `yaml:"content"`
	Status  string `yaml:"status"` // pending|completed
	Done    *bool  `yaml:"done"`
}

type frontmatter struct {
	Name  string   `yaml:"name"`
	Todos []fmTodo `yaml:"todos"`
}

var (
	checklistLine = regexp.MustCompile(`(?i)^\s*-\s*\[([ xX])\]\s*(.+)$`)
	idPrefix      = regexp.MustCompile(`(?i)^id:\s*([A-Za-z0-9][A-Za-z0-9_-]*)\s*[—\-–:]\s*(.+)$`)
	slugRe        = regexp.MustCompile(`[^a-z0-9]+`)
)

// Parse extracts todos from YAML frontmatter and/or a trailing checklist.
func Parse(body string) (markdown string, todos []Todo, err error) {
	body = strings.TrimSpace(body)
	fmTodos, rest, _ := splitFrontmatterTodos(body)
	markdown = rest

	var out []Todo
	for _, t := range fmTodos {
		id := strings.TrimSpace(t.ID)
		content := strings.TrimSpace(t.Content)
		if id == "" && content == "" {
			continue
		}
		if id == "" {
			id = slugify(content)
		}
		done := false
		if t.Done != nil {
			done = *t.Done
		} else if strings.EqualFold(t.Status, "completed") || strings.EqualFold(t.Status, "done") {
			done = true
		}
		out = append(out, Todo{ID: id, Content: content, Done: done})
	}

	sectionTodos, bodyWithout := extractChecklistSection(markdown)
	if len(sectionTodos) > 0 {
		out = mergeTodos(out, sectionTodos)
		markdown = bodyWithout
	} else {
		// Also scan whole doc for checklist lines if no ## Todos section
		scanned := scanChecklistLines(markdown)
		if len(scanned) > 0 && len(out) == 0 {
			out = scanned
			markdown = stripTrailingChecklist(markdown)
		}
	}

	markdown = strings.TrimSpace(markdown)
	return markdown, out, nil
}

// HasTodos reports whether any todos exist.
func HasTodos(todos []Todo) bool {
	return len(todos) > 0
}

// Normalize returns markdown ending with a ## Todos checklist; strips fm todos.
func Normalize(body string, todos []Todo) string {
	md, parsed, _ := Parse(body)
	if len(todos) == 0 {
		todos = parsed
	}
	md = strings.TrimSpace(md)
	var b strings.Builder
	if md != "" {
		b.WriteString(md)
		b.WriteString("\n\n")
	}
	b.WriteString("## Todos\n\n")
	if len(todos) == 0 {
		b.WriteString("<!-- no todos -->\n")
		return b.String()
	}
	for _, t := range todos {
		id := t.ID
		if id == "" {
			id = slugify(t.Content)
		}
		mark := " "
		if t.Done {
			mark = "x"
		}
		content := t.Content
		if content == "" {
			content = id
		}
		b.WriteString(fmt.Sprintf("- [%s] id: %s — %s\n", mark, id, content))
	}
	return b.String()
}

func splitFrontmatterTodos(s string) (todos []fmTodo, rest string, ok bool) {
	s = strings.TrimPrefix(s, "\ufeff")
	if !strings.HasPrefix(s, "---") {
		return nil, s, false
	}
	rest0 := s[3:]
	if strings.HasPrefix(rest0, "\n") {
		rest0 = rest0[1:]
	} else if strings.HasPrefix(rest0, "\r\n") {
		rest0 = rest0[2:]
	} else {
		return nil, s, false
	}
	end := strings.Index(rest0, "\n---")
	if end < 0 {
		return nil, s, false
	}
	yamlBlock := rest0[:end]
	body := rest0[end+4:]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return nil, s, false
	}
	return fm.Todos, body, true
}

func extractChecklistSection(md string) ([]Todo, string) {
	lines := strings.Split(md, "\n")
	start := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.EqualFold(trim, "## Todos") || strings.EqualFold(trim, "## Todo") || strings.EqualFold(trim, "## To-dos") {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, md
	}
	var todos []Todo
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			end = i
			break
		}
		if t, ok := parseChecklistItem(line); ok {
			todos = append(todos, t)
		}
	}
	kept := append([]string{}, lines[:start]...)
	if end < len(lines) {
		kept = append(kept, lines[end:]...)
	}
	return todos, strings.TrimSpace(strings.Join(kept, "\n"))
}

func scanChecklistLines(md string) []Todo {
	var out []Todo
	for _, line := range strings.Split(md, "\n") {
		if t, ok := parseChecklistItem(line); ok {
			out = append(out, t)
		}
	}
	return out
}

func stripTrailingChecklist(md string) string {
	lines := strings.Split(md, "\n")
	i := len(lines) - 1
	for i >= 0 && strings.TrimSpace(lines[i]) == "" {
		i--
	}
	for i >= 0 {
		if _, ok := parseChecklistItem(lines[i]); ok || strings.TrimSpace(lines[i]) == "" {
			i--
			continue
		}
		break
	}
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[:i+1], "\n"))
}

func parseChecklistItem(line string) (Todo, bool) {
	m := checklistLine.FindStringSubmatch(line)
	if m == nil {
		return Todo{}, false
	}
	done := m[1] != " "
	rest := strings.TrimSpace(m[2])
	id, content := "", rest
	if im := idPrefix.FindStringSubmatch(rest); im != nil {
		id = im[1]
		content = strings.TrimSpace(im[2])
	} else {
		id = slugify(rest)
	}
	if id == "" {
		id = slugify(content)
	}
	return Todo{ID: id, Content: content, Done: done}, true
}

func mergeTodos(a, b []Todo) []Todo {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := map[string]int{}
	out := append([]Todo{}, a...)
	for i, t := range out {
		seen[t.ID] = i
	}
	for _, t := range b {
		if i, ok := seen[t.ID]; ok {
			out[i] = t
			continue
		}
		seen[t.ID] = len(out)
		out = append(out, t)
	}
	return out
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "todo"
	}
	if len(s) > 40 {
		s = s[:40]
		s = strings.Trim(s, "-")
	}
	return s
}
