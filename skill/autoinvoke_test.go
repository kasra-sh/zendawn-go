package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoInvokeDefaultTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	_ = os.WriteFile(path, []byte("---\ndescription: Def\n---\n\nBody\n"), 0o644)
	p, err := LoadFile("def", path)
	if err != nil {
		t.Fatal(err)
	}
	if !p.AutoInvoke {
		t.Fatal("autoInvoke should default true")
	}
}

func TestAutoInvokeFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	_ = os.WriteFile(path, []byte("---\ndescription: Manual\nautoInvoke: false\n---\n\nBody\n"), 0o644)
	p, err := LoadFile("manual", path)
	if err != nil {
		t.Fatal(err)
	}
	if p.AutoInvoke {
		t.Fatal("want autoInvoke false")
	}
}

func TestCatalogBlockAutoInvokeHints(t *testing.T) {
	packs := map[string]*Pack{
		"demo":   {ID: "demo", Name: "demo", Description: "Demo skill", AutoInvoke: true},
		"manual": {ID: "manual", Name: "manual", Description: "Manual only", AutoInvoke: false},
	}
	cat := CatalogBlock(packs, []string{"demo", "manual"})
	if !strings.Contains(cat, "[autoInvoke]") {
		t.Fatalf("missing autoInvoke mark: %s", cat)
	}
	if !strings.Contains(cat, "[manual]") {
		t.Fatalf("missing manual mark: %s", cat)
	}
	if !strings.Contains(cat, "skill tool") {
		t.Fatalf("missing load instructions: %s", cat)
	}
}
