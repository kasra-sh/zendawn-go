package config

import "testing"

func TestParseProvidersAndModels(t *testing.T) {
	yaml := `
id: multi
instructions: hi
model:
  provider: openrouter
  id: openai/gpt-4o-mini
providers:
  openrouter:
    baseUrl: https://openrouter.ai/api/v1
    apiKeyEnv: OPENROUTER_API_KEY
  openai:
    baseUrl: https://api.openai.com/v1
    apiKeyEnv: OPENAI_API_KEY
models:
  - provider: openrouter
    id: openai/gpt-4o-mini
  - provider: openai
    id: gpt-4o-mini
`
	doc, err := ParseYAML([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Providers) != 2 {
		t.Fatalf("providers: %+v", doc.Providers)
	}
	if doc.Providers["openrouter"].BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("base: %+v", doc.Providers["openrouter"])
	}
	if len(doc.Models) != 2 {
		t.Fatalf("models: %+v", doc.Models)
	}
}

func TestProvidersCatalogMismatch(t *testing.T) {
	yaml := `
id: bad
instructions: hi
model:
  provider: openai
  id: gpt-4o-mini
providers:
  openai:
    baseUrl: https://api.openai.com/v1
models:
  - provider: openrouter
    id: x
`
	_, err := ParseYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for catalog provider missing from providers map")
	}
}
