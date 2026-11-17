package compact

import (
	"strings"
	"testing"
	"time"

	"github.com/kasra-sh/zendawn-go/session"
)

func msg(role, text string) session.Item {
	return session.Item{
		Type:      session.ItemMessage,
		Role:      role,
		CreatedAt: time.Now().UTC(),
		Content:   []session.ContentPart{{Type: session.PartText, Text: text}},
	}
}

func TestShouldCompactLax(t *testing.T) {
	items := []session.Item{msg(session.RoleUser, "a"), msg(session.RoleAssistant, "b"), msg(session.RoleUser, "c")}
	th := DefaultThresholds()
	if ShouldCompact(ModeLax, items, State{}, th, 100, 0) {
		t.Fatal("unknown context limit should skip")
	}
	if ShouldCompact(ModeLax, items, State{}, th, 70, 100) {
		t.Fatal("under threshold should skip")
	}
	if !ShouldCompact(ModeLax, items, State{}, th, 80, 100) {
		t.Fatal("at threshold should compact")
	}
}

func TestShouldCompactStrictMinChars(t *testing.T) {
	short := []session.Item{msg(session.RoleUser, "hi"), msg(session.RoleAssistant, "yo"), msg(session.RoleUser, "go")}
	th := Thresholds{MinChars: 100}
	if ShouldCompact(ModeStrict, short, State{}, th, 0, 0) {
		t.Fatal("below minChars should skip")
	}
	longBody := strings.Repeat("x", 80)
	long := []session.Item{msg(session.RoleUser, longBody), msg(session.RoleAssistant, longBody), msg(session.RoleUser, "latest")}
	if !ShouldCompact(ModeStrict, long, State{}, th, 0, 0) {
		t.Fatal("above minChars should compact")
	}
}

func TestShouldCompactSkipsWhenAlreadyThrough(t *testing.T) {
	items := []session.Item{msg(session.RoleUser, "a"), msg(session.RoleAssistant, "b"), msg(session.RoleUser, "c")}
	st := State{Summary: "old", Through: 2} // covers through last user
	if ShouldCompact(ModeAggressive, items, st, Thresholds{MinChars: 1}, 0, 0) {
		t.Fatal("already covered prefix should skip")
	}
}

func TestProjectKeepsLatestUser(t *testing.T) {
	items := []session.Item{
		msg(session.RoleUser, "first"),
		msg(session.RoleAssistant, "ok"),
		msg(session.RoleUser, "latest ask"),
	}
	st := State{Summary: "prior work done", Through: 2}
	proj := Project(items, st)
	if len(proj) != 2 {
		t.Fatalf("len=%d want 2 (summary + latest user)", len(proj))
	}
	if proj[0].Role != session.RoleSystem || !strings.Contains(session.TextParts(proj[0].Content), "prior work done") {
		t.Fatalf("summary item: %+v", proj[0])
	}
	if session.TextParts(proj[1].Content) != "latest ask" {
		t.Fatalf("latest user lost: %+v", proj[1])
	}
}

func TestProjectNonePassthrough(t *testing.T) {
	items := []session.Item{msg(session.RoleUser, "a"), msg(session.RoleAssistant, "b")}
	proj := Project(items, State{})
	if len(proj) != 2 {
		t.Fatalf("len=%d", len(proj))
	}
}

func TestCoverThrough(t *testing.T) {
	items := []session.Item{msg(session.RoleUser, "a"), msg(session.RoleAssistant, "b"), msg(session.RoleUser, "c")}
	if got := CoverThrough(items); got != 2 {
		t.Fatalf("got %d", got)
	}
}
