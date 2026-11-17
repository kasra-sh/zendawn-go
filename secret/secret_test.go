package secret

import (
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/zerr"
)

func TestExpandFromEnv(t *testing.T) {
	s := New([]config.SecretDecl{{Name: "FOO", Source: SourceEnv}})
	s.LookupEnv = func(k string) (string, bool) {
		if k == "FOO" {
			return "bar", true
		}
		return "", false
	}
	got, err := s.Expand("Bearer ${FOO}", "test")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Bearer bar" {
		t.Fatalf("got %q", got)
	}
}

func TestAskDeclared(t *testing.T) {
	s := New([]config.SecretDecl{{Name: "KEY", Source: SourceEnvOrAsk, Description: "k"}})
	s.LookupEnv = func(string) (string, bool) { return "", false }
	asked := false
	s.Ask = func(name, desc, reason string) (string, error) {
		asked = true
		if name != "KEY" || desc != "k" {
			t.Fatalf("ask args %s %s", name, desc)
		}
		return "secret-value", nil
	}
	got, err := s.Get("KEY", "mcp:x")
	if err != nil {
		t.Fatal(err)
	}
	if !asked || got != "secret-value" {
		t.Fatalf("asked=%v got=%q", asked, got)
	}
	// second resolve from vault
	asked = false
	got, err = s.Get("KEY", "mcp:x")
	if err != nil || asked || got != "secret-value" {
		t.Fatalf("vault reuse asked=%v got=%q err=%v", asked, got, err)
	}
}

func TestUndeclaredNoAsk(t *testing.T) {
	s := New(nil)
	s.LookupEnv = func(string) (string, bool) { return "", false }
	s.Ask = func(string, string, string) (string, error) {
		t.Fatal("should not ask")
		return "", nil
	}
	_, err := s.Get("UNDECLARED", "r")
	if err == nil || err.(*zerr.Error).Code != zerr.CodeMissingSecret {
		t.Fatalf("want missing_secret, got %v", err)
	}
}

func TestRedact(t *testing.T) {
	s := New(nil)
	s.Provide("X", "super-secret")
	out := s.Redact("token=super-secret ok")
	if out != "token=[REDACTED] ok" {
		t.Fatalf("got %q", out)
	}
}
