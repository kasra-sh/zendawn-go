package config

import (
	"testing"
)

func TestLoadTavilyExampleShape(t *testing.T) {
	doc, err := ParseYAML([]byte(`
id: tavily-assistant
instructions: test
model:
  provider: openai
  id: gpt-4.1
tools: "*"
secrets:
  - name: TAVILY_API_KEY
    source: env_or_ask
mcp:
  servers:
    tavily:
      transport: http
      url: https://mcp.tavily.com/mcp/
      headers:
        Authorization: "Bearer ${TAVILY_API_KEY}"
      optional: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Secrets) != 1 || doc.Secrets[0].Name != "TAVILY_API_KEY" {
		t.Fatalf("secrets %+v", doc.Secrets)
	}
	if doc.MCP == nil || doc.MCP.Servers["tavily"].Transport != "http" {
		t.Fatalf("mcp %+v", doc.MCP)
	}
}

func TestValidateMCPStdioRequiresCommand(t *testing.T) {
	err := validateMCP(&MCPConfig{Servers: map[string]MCPServer{
		"x": {Transport: "stdio"},
	}})
	if err == nil {
		t.Fatal("expected error")
	}
}
