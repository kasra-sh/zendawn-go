package runtime

import (
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/provider/openrouter"
)

func TestMergeOpenRouterCatalog(t *testing.T) {
	yaml := []config.ModelCatalogEntry{
		{Provider: "openai", ID: "b:free", Class: "fast"},
		{Provider: "openai", ID: "yaml-only", Class: "balanced"},
		{Provider: "other", ID: "ignored-elsewhere"},
	}
	api := []openrouter.Model{
		{ID: "a:free", Name: "A", ContextLength: 1000},
		{ID: "b:free", Name: "B", ContextLength: 2000},
		{ID: "c:free", Name: "C", ContextLength: 3000},
	}
	merged, specs := MergeOpenRouterCatalog(yaml, api, "openai")
	if len(merged) != 5 {
		t.Fatalf("len=%d %+v", len(merged), merged)
	}
	wantIDs := []string{"a:free", "b:free", "c:free", "yaml-only", "ignored-elsewhere"}
	for i, id := range wantIDs {
		if merged[i].ID != id {
			t.Fatalf("pos %d: %+v want %s", i, merged[i], id)
		}
	}
	if merged[1].Class != "fast" {
		t.Fatalf("expected YAML class on overlap: %+v", merged[1])
	}
	if merged[3].Provider != "openai" {
		t.Fatalf("yaml-only: %+v", merged[3])
	}
	if merged[4].Provider != "other" {
		t.Fatalf("other provider: %+v", merged[4])
	}
	if specs["openai/a:free"].ContextLength != 1000 {
		t.Fatalf("specs: %+v", specs)
	}
	if specs["openai/b:free"].Name != "B" {
		t.Fatalf("specs b: %+v", specs["openai/b:free"])
	}
}

func TestApplyModelsCatalog(t *testing.T) {
	doc := &config.Document{
		Single: &config.Agent{
			ID:           "a",
			Instructions: "x",
			Model:        config.ModelConfig{Provider: "openai", ID: "old"},
		},
		Models: []config.ModelCatalogEntry{
			{Provider: "openai", ID: "old"},
		},
	}
	rt := New(doc)
	rt.RegisterProvider(provider.NewOpenAI("openai", "https://openrouter.ai/api/v1", "K"))
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	merged := []config.ModelCatalogEntry{
		{Provider: "openai", ID: "first:free"},
		{Provider: "openai", ID: "second:free"},
	}
	specs := map[string]ModelSpec{
		"openai/first:free":  {ContextLength: 128000, Name: "First"},
		"openai/second:free": {ContextLength: 64000, Name: "Second"},
	}
	p, id, err := rt.ApplyModelsCatalog(s.ID, merged, specs)
	if err != nil {
		t.Fatal(err)
	}
	if p != "openai" || id != "first:free" {
		t.Fatalf("got %s/%s", p, id)
	}
	s.Lock()
	defer s.Unlock()
	if s.Provider != "openai" || s.ModelID != "first:free" {
		t.Fatalf("session %+v", s)
	}
	if s.LastContextLimit != 128000 {
		t.Fatalf("LastContextLimit=%d", s.LastContextLimit)
	}
	if s.ContextLimits["openai/second:free"] != 64000 {
		t.Fatalf("limits %+v", s.ContextLimits)
	}
	if len(rt.Doc.Models) != 2 || rt.Doc.Models[0].ID != "first:free" {
		t.Fatalf("doc models %+v", rt.Doc.Models)
	}
}
