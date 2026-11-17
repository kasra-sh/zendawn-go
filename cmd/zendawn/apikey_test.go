package main

import (
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
)

func TestResolveAPIKeyOrder(t *testing.T) {
	env := map[string]string{"OPENROUTER_API_KEY": "from-env"}
	getenv := func(k string) string { return env[k] }

	if got := resolveAPIKey("flag", "model", "prov", "OPENROUTER_API_KEY", getenv); got != "flag" {
		t.Fatalf("flag wins: %q", got)
	}
	if got := resolveAPIKey("", "model", "prov", "OPENROUTER_API_KEY", getenv); got != "model" {
		t.Fatalf("model wins: %q", got)
	}
	if got := resolveAPIKey("", "", "prov", "OPENROUTER_API_KEY", getenv); got != "prov" {
		t.Fatalf("provider wins: %q", got)
	}
	if got := resolveAPIKey("", "", "", "OPENROUTER_API_KEY", getenv); got != "from-env" {
		t.Fatalf("env wins: %q", got)
	}
	if got := resolveAPIKey("", "", "", "MISSING", getenv); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestResolveAPIKeyEnv(t *testing.T) {
	doc := &config.Document{
		Providers: map[string]config.ProviderConfig{
			"openai": {APIKeyEnv: "OPENROUTER_API_KEY"},
		},
	}
	agent := &config.Agent{Model: config.ModelConfig{APIKeyEnv: "MODEL_KEY"}}
	if got := resolveAPIKeyEnv(doc, "openai", agent); got != "OPENROUTER_API_KEY" {
		t.Fatalf("provider env: %q", got)
	}
	if got := resolveAPIKeyEnv(nil, "openai", agent); got != "MODEL_KEY" {
		t.Fatalf("model env: %q", got)
	}
	if got := resolveAPIKeyEnv(nil, "", &config.Agent{}); got != "OPENAI_API_KEY" {
		t.Fatalf("default: %q", got)
	}
}
