package permission

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern, value string
		want           bool
	}{
		{"*", "anything", true},
		{"**", "anything", true},
		{"", "", true},
		{"", "x", false},
		{"?", "é", true},
		{"?", "ab", false},
		{"a/**/b", "a/b", false}, // ** ≡ *; keeps both slashes → needs a/*/b
		{"a/**/b", "a/x/b", true},
		{"a/**", "a", false}, // a/** → a/*; requires '/' after a
		{"a/**", "a/", true},
		{"a/**", "a/b/c", true},
		{"src/*", "src/main.go", true},
		{"src/*", "src/pkg/main.go", true}, // * spans /
		{"read", "read", true},
		{"re?d", "read", true},
		{"[", "[", true}, // literal; no filepath.Match char-class path
	}
	for _, tt := range tests {
		got := Match(tt.pattern, tt.value)
		if got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}

func TestResolve(t *testing.T) {
	t.Run("no match asks", func(t *testing.T) {
		got := Resolve(nil, nil, Assertion{Action: "read", Resource: "x"})
		if got != EffectAsk {
			t.Fatalf("got %q want ask", got)
		}
	})

	t.Run("last match wins", func(t *testing.T) {
		rules := []Rule{
			{Action: "read", Resource: "*", Effect: EffectDeny},
			{Action: "read", Resource: "src/*", Effect: EffectAllow},
		}
		got := Resolve(rules, nil, Assertion{Action: "read", Resource: "src/a.go"})
		if got != EffectAllow {
			t.Fatalf("got %q want allow", got)
		}
	})

	t.Run("overlay overrides agent deny", func(t *testing.T) {
		agent := []Rule{{Action: "shell", Resource: "*", Effect: EffectDeny}}
		overlay := []Rule{{Action: "shell", Resource: "*", Effect: EffectAllow}}
		got := Resolve(agent, overlay, Assertion{Action: "shell", Resource: "ls"})
		if got != EffectAllow {
			t.Fatalf("got %q want allow", got)
		}
	})

	t.Run("invalid effect denies", func(t *testing.T) {
		rules := []Rule{{Action: "read", Resource: "*", Effect: "alow"}}
		got := Resolve(rules, nil, Assertion{Action: "read", Resource: "x"})
		if got != EffectDeny {
			t.Fatalf("got %q want deny", got)
		}
	})

	t.Run("cleaned traversal does not match prefix", func(t *testing.T) {
		rules := []Rule{{Action: "read", Resource: "/home/user/*", Effect: EffectAllow}}
		got := Resolve(rules, nil, Assertion{
			Action:   "read",
			Resource: "/home/user/../../etc/passwd",
		})
		if got != EffectAsk {
			t.Fatalf("got %q want ask (no match after clean)", got)
		}
	})
}

func TestResolveMCPCatchAllCoercesAsk(t *testing.T) {
	denyAll := []Rule{{Action: "*", Resource: "*", Effect: EffectDeny}}
	a := Assertion{Action: "tavily__search", Resource: "*", ToolName: "tavily__search"}

	t.Run("deny star asks for MCP", func(t *testing.T) {
		got := Resolve(denyAll, nil, a)
		if got != EffectAsk {
			t.Fatalf("got %q want ask", got)
		}
	})

	t.Run("allow star still asks for MCP", func(t *testing.T) {
		rules := []Rule{{Action: "*", Resource: "*", Effect: EffectAllow}}
		got := Resolve(rules, nil, a)
		if got != EffectAsk {
			t.Fatalf("got %q want ask", got)
		}
	})

	t.Run("explicit allow wins", func(t *testing.T) {
		rules := []Rule{
			{Action: "*", Resource: "*", Effect: EffectDeny},
			{Action: "tavily__*", Resource: "*", Effect: EffectAllow},
		}
		got := Resolve(rules, nil, a)
		if got != EffectAllow {
			t.Fatalf("got %q want allow", got)
		}
	})

	t.Run("explicit deny wins", func(t *testing.T) {
		rules := []Rule{
			{Action: "*", Resource: "*", Effect: EffectDeny},
			{Action: "tavily__*", Resource: "*", Effect: EffectDeny},
		}
		got := Resolve(rules, nil, a)
		if got != EffectDeny {
			t.Fatalf("got %q want deny", got)
		}
	})

	t.Run("non-MCP deny star stays deny", func(t *testing.T) {
		got := Resolve(denyAll, nil, Assertion{Action: "shell", Resource: "ls", ToolName: "shell"})
		if got != EffectDeny {
			t.Fatalf("got %q want deny", got)
		}
	})
}
