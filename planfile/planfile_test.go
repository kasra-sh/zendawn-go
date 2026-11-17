package planfile

import (
	"strings"
	"testing"
)

func TestParseFrontmatterAndNormalize(t *testing.T) {
	raw := `---
name: Demo
todos:
  - id: one
    content: Do one
    status: pending
  - id: two
    content: Do two
    status: completed
---

# Demo plan

Body text.
`
	md, todos, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "Body text") || strings.Contains(md, "todos:") {
		t.Fatalf("md=%q", md)
	}
	if len(todos) != 2 || todos[0].ID != "one" || !todos[1].Done || todos[0].Done {
		t.Fatalf("%+v", todos)
	}
	out := Normalize(md, todos)
	if !strings.Contains(out, "## Todos") || !strings.Contains(out, "- [x] id: two") {
		t.Fatalf("norm=%s", out)
	}
	if !HasTodos(todos) {
		t.Fatal("HasTodos")
	}
}

func TestParseChecklistSection(t *testing.T) {
	raw := `# Plan

Stuff

## Todos

- [ ] id: a — Alpha
- [x] id: b — Beta
`
	md, todos, err := Parse(raw)
	if err != nil || len(todos) != 2 {
		t.Fatalf("%v %+v", err, todos)
	}
	if strings.Contains(md, "## Todos") {
		t.Fatalf("section not stripped: %q", md)
	}
	if !todos[1].Done || todos[0].Content != "Alpha" {
		t.Fatalf("%+v", todos)
	}
}

func TestNormalizeEmptyTodos(t *testing.T) {
	out := Normalize("# Hi", nil)
	if !strings.Contains(out, "## Todos") {
		t.Fatal(out)
	}
}
