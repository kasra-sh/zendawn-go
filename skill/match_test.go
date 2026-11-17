package skill

import (
	"strings"
	"testing"
)

func TestResolveName(t *testing.T) {
	bound := []string{"demo", "docker", "docs"}

	id, guessed, err := ResolveName("demo", bound)
	if err != nil || id != "demo" || guessed {
		t.Fatalf("exact: %q %v %v", id, guessed, err)
	}

	id, guessed, err = ResolveName("DEMO", bound)
	if err != nil || id != "demo" || !guessed {
		t.Fatalf("ci: %q %v %v", id, guessed, err)
	}

	id, guessed, err = ResolveName("dem", bound)
	if err != nil || id != "demo" || !guessed {
		t.Fatalf("prefix: %q %v %v", id, guessed, err)
	}

	_, _, err = ResolveName("do", bound)
	if err == nil {
		t.Fatal("expected ambiguous prefix")
	}

	id, guessed, err = ResolveName("dem0", bound) // o→0
	if err != nil || id != "demo" || !guessed {
		t.Fatalf("fuzzy: %q %v %v", id, guessed, err)
	}

	_, _, err = ResolveName("zzzzz", bound)
	if err == nil {
		t.Fatal("expected miss")
	}
	if !strings.Contains(err.Error(), "skill not found") {
		t.Fatalf("want skill not found, got %v", err)
	}
}
