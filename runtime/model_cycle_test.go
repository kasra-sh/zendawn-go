package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/session"
)

func TestCycleModelWraps(t *testing.T) {
	tmp := t.TempDir()
	wd := filepath.Join(tmp, "ws")
	_ = os.MkdirAll(wd, 0o755)

	doc := &config.Document{
		Single: &config.Agent{
			ID: "a", Instructions: "x",
			Model: config.ModelConfig{Provider: "openai", ID: "m1"},
		},
		Providers: map[string]config.ProviderConfig{
			"openai":     {BaseURL: "http://x"},
			"openrouter": {BaseURL: "http://y"},
		},
		Models: []config.ModelCatalogEntry{
			{Provider: "openai", ID: "m1"},
			{Provider: "openrouter", ID: "m2"},
			{Provider: "openai", ID: "m3"},
		},
	}
	rt := New(doc)
	rt.WorkDir = wd
	rt.RegisterProvider(provider.NewOpenAI("openai", "http://x", "K"))
	rt.RegisterProvider(provider.NewOpenAI("openrouter", "http://y", "K"))

	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	p, id, err := rt.CycleModel(s.ID)
	if err != nil || p != "openrouter" || id != "m2" {
		t.Fatalf("1st cycle: %s/%s %v", p, id, err)
	}
	p, id, err = rt.CycleModel(s.ID)
	if err != nil || p != "openai" || id != "m3" {
		t.Fatalf("2nd cycle: %s/%s %v", p, id, err)
	}
	p, id, err = rt.CycleModel(s.ID)
	if err != nil || p != "openai" || id != "m1" {
		t.Fatalf("wrap: %s/%s %v", p, id, err)
	}
}

func TestApplyUsageAccumulates(t *testing.T) {
	rt := New(&config.Document{Single: &config.Agent{
		ID: "a", Instructions: "x", Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}})
	s := session.New("ses1", "a")
	s.Provider = "mock"
	s.ModelID = "m"
	rt.mu.Lock()
	rt.sessions[s.ID] = s
	rt.mu.Unlock()

	rt.applyUsage(s, provider.Usage{InputTokens: 100, OutputTokens: 20, CostUSD: 0.01, CostKnown: true, ContextLimit: 200000})
	rt.applyUsage(s, provider.Usage{InputTokens: 50, OutputTokens: 5, CostUSD: 0.002, CostKnown: true})

	s.Lock()
	defer s.Unlock()
	if s.UsageInput != 150 || s.UsageOutput != 25 {
		t.Fatalf("totals in/out: %d %d", s.UsageInput, s.UsageOutput)
	}
	if !s.UsageCostKnown || s.UsageCostUSD < 0.011 || s.UsageCostUSD > 0.013 {
		t.Fatalf("cost: %v %v", s.UsageCostKnown, s.UsageCostUSD)
	}
	if s.LastInputTokens != 50 || s.LastContextLimit != 200000 {
		t.Fatalf("last: %d %d", s.LastInputTokens, s.LastContextLimit)
	}
}
