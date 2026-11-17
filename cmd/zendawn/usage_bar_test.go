package main

import (
	"strings"
	"testing"
)

func TestFormatTokenCount(t *testing.T) {
	if got := formatTokenCount(999); got != "999" {
		t.Fatalf("got %s", got)
	}
	if got := formatTokenCount(25400); got != "25.4K" {
		t.Fatalf("got %s", got)
	}
	if got := formatTokenCount(200000); got != "200K" {
		t.Fatalf("got %s", got)
	}
	if got := formatTokenCount(1048576); got != "1.0M" {
		t.Fatalf("got %s", got)
	}
}

func TestFormatUsageBar(t *testing.T) {
	s := formatUsageBar(25000, 200000)
	if !strings.Contains(s, "25K") || !strings.Contains(s, "200K") {
		t.Fatalf("bar: %s", s)
	}
	if !strings.HasPrefix(s, "[") || !strings.Contains(s, "]") {
		t.Fatalf("missing brackets: %s", s)
	}
	unknown := formatUsageBar(1000, 0)
	if !strings.Contains(unknown, "/?") {
		t.Fatalf("unknown limit: %s", unknown)
	}
}

func TestFormatSessionUsageLine(t *testing.T) {
	line := formatSessionUsageLine(1000, 8000, 5000, 1000, 0.042, true, "openrouter/x")
	if !strings.Contains(line, "$0.042") || !strings.Contains(line, "openrouter/x") {
		t.Fatalf("line: %s", line)
	}
	line2 := formatSessionUsageLine(10, 0, 10, 0, 0, false, "openai/y")
	if !strings.Contains(line2, "—") {
		t.Fatalf("unknown cost: %s", line2)
	}
}
