package runtime

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/session"
)

type captureProvider struct {
	last provider.Request
}

func (c *captureProvider) ID() string { return "mock" }
func (c *captureProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Tools: true, Streaming: false, Images: true}
}
func (c *captureProvider) Complete(ctx context.Context, req provider.Request, onEvent provider.StreamHandler) (provider.Response, error) {
	c.last = req
	return provider.Response{
		Message: session.Item{
			Type: session.ItemMessage, Role: session.RoleAssistant,
			Content: []session.ContentPart{{Type: session.PartText, Text: "ok"}},
		},
		FinishReason: "stop",
	}, nil
}

func TestSubmitWithAttachments(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZENDAWN_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	doc := &config.Document{Single: &config.Agent{
		ID: "a", Instructions: "test",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}}
	rt := New(doc)
	rt.WorkDir = filepath.Join(tmp, "ws")
	_ = os.MkdirAll(rt.WorkDir, 0o755)
	cap := &captureProvider{}
	rt.RegisterProvider(cap)

	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := rt.Upload(s.ID, "pic.png", "image/png", bytes.NewReader([]byte("PNGDATA")))
	if err != nil {
		t.Fatal(err)
	}
	meta2, err := rt.Upload(s.ID, "notes.txt", "text/plain", bytes.NewReader([]byte("hi")))
	if err != nil {
		t.Fatal(err)
	}

	reason, err := rt.Submit(context.Background(), s.ID, session.UserInput{
		Text:          "describe",
		AttachmentIDs: []string{meta.ID, meta2.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reason != session.StopCompleted {
		t.Fatalf("reason %s", reason)
	}
	hist := s.History()
	if len(hist) < 1 {
		t.Fatal("empty hist")
	}
	user := hist[0]
	if len(user.Content) != 3 {
		t.Fatalf("parts %#v", user.Content)
	}
	if user.Content[1].Type != session.PartImage || user.Content[1].AttachmentID != meta.ID {
		t.Fatalf("image part %#v", user.Content[1])
	}
	if user.Content[2].Type != session.PartFile || user.Content[2].AttachmentID != meta2.ID {
		t.Fatalf("file part %#v", user.Content[2])
	}

	list, err := rt.ListAttachments(s.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %v %d", err, len(list))
	}
	m, rc, err := rt.Download(s.ID, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if m.Filename != "pic.png" || string(b) != "PNGDATA" {
		t.Fatalf("download %#v %q", m, b)
	}
}

func TestRegisterArtifact(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZENDAWN_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	doc := &config.Document{Single: &config.Agent{
		ID:    "a",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}}
	rt := New(doc)
	rt.WorkDir = filepath.Join(tmp, "ws")
	_ = os.MkdirAll(rt.WorkDir, 0o755)
	rt.RegisterProvider(&captureProvider{})
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rt.WorkDir, "built.bin")
	if err := os.WriteFile(path, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := rt.RegisterArtifact(s.ID, "built.bin")
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "artifact" {
		t.Fatalf("source %s", m.Source)
	}
}

func TestSubmitRejectsUnknownAttachment(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZENDAWN_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	doc := &config.Document{Single: &config.Agent{
		ID:    "a",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}}
	rt := New(doc)
	rt.WorkDir = filepath.Join(tmp, "ws")
	_ = os.MkdirAll(rt.WorkDir, 0o755)
	rt.RegisterProvider(&captureProvider{})
	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.Submit(context.Background(), s.ID, session.UserInput{
		Text: "x", AttachmentIDs: []string{"att_missing"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
