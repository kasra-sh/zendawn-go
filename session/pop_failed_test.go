package session

import "testing"

func TestPopFailedTurn(t *testing.T) {
	s := New("s1", "agent")
	s.AppendItem(Item{
		ID: "u1", Type: ItemMessage, Role: RoleUser,
		Content: []ContentPart{{Type: PartText, Text: "hello"}},
	})
	s.AppendItem(Item{
		ID: "a1", Type: ItemMessage, Role: RoleAssistant,
		Content: []ContentPart{{Type: PartText, Text: "partial"}},
	})
	text, ok := s.PopFailedTurn()
	if !ok || text != "hello" {
		t.Fatalf("got %q ok=%v", text, ok)
	}
	if len(s.Transcript) != 0 {
		t.Fatalf("transcript len %d", len(s.Transcript))
	}
	if _, ok := s.PopFailedTurn(); ok {
		t.Fatal("expected empty pop to fail")
	}
}
