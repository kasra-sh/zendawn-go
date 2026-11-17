package main

import (
	"testing"

	"github.com/kasra-sh/zendawn-go/session"
)

func TestPushPromptHistory(t *testing.T) {
	var h []string
	h = pushPromptHistory(h, "one")
	h = pushPromptHistory(h, "one") // duplicate skip
	h = pushPromptHistory(h, "two")
	h = pushPromptHistory(h, "")
	if len(h) != 2 || h[0] != "one" || h[1] != "two" {
		t.Fatalf("got %#v", h)
	}
}

func TestFailedStop(t *testing.T) {
	if !failedStop(session.StopError) || !failedStop(session.StopAborted) {
		t.Fatal("expected error/abort failed")
	}
	if !failedStop("error: boom") {
		t.Fatal("expected error: prefix")
	}
	if failedStop(session.StopCompleted) || failedStop("completed") {
		t.Fatal("completed should not be failed")
	}
}

func TestEdgeArrowAction(t *testing.T) {
	cases := []struct {
		key             string
		line, lineCount int
		want            string
	}{
		{"up", 0, 1, arrowScrollUp},
		{"up", 0, 3, arrowScrollUp},
		{"up", 1, 3, arrowPass},
		{"down", 0, 1, arrowScrollDown},
		{"down", 2, 3, arrowScrollDown},
		{"down", 1, 3, arrowPass},
		{"alt+up", 0, 1, arrowHistoryUp},
		{"alt+up", 1, 3, arrowHistoryUp},
		{"alt+down", 0, 1, arrowHistoryDown},
		{"alt+down", 1, 3, arrowHistoryDown},
		{"left", 0, 1, arrowPass},
	}
	for _, tc := range cases {
		got := edgeArrowAction(tc.key, tc.line, tc.lineCount)
		if got != tc.want {
			t.Fatalf("%s line=%d/%d: got %q want %q", tc.key, tc.line, tc.lineCount, got, tc.want)
		}
	}
}

func TestHistoryStep(t *testing.T) {
	hist := []string{"a", "b", "c"}
	idx, text, draft := historyStep(hist, -1, "", "draft", -1)
	if idx != 2 || text != "c" || draft != "draft" {
		t.Fatalf("enter browse: idx=%d text=%q draft=%q", idx, text, draft)
	}
	idx, text, draft = historyStep(hist, 2, "draft", "c", -1)
	if idx != 1 || text != "b" {
		t.Fatalf("up: idx=%d text=%q", idx, text)
	}
	idx, text, draft = historyStep(hist, 0, "draft", "a", -1)
	if idx != 0 || text != "a" {
		t.Fatalf("clamp top: idx=%d text=%q", idx, text)
	}
	idx, text, draft = historyStep(hist, 2, "draft", "c", 1)
	if idx != -1 || text != "draft" {
		t.Fatalf("exit browse: idx=%d text=%q draft=%q", idx, text, draft)
	}
}
