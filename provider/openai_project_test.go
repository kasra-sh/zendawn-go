package provider

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/kasra-sh/zendawn-go/session"
)

func TestProjectMessagesTextOnly(t *testing.T) {
	items := []session.Item{{
		Type: session.ItemMessage, Role: session.RoleUser,
		Content: []session.ContentPart{{Type: session.PartText, Text: "hi"}},
	}}
	out, err := projectMessages("sys", items, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len %d", len(out))
	}
	if out[1]["content"] != "hi" {
		t.Fatalf("content %#v", out[1]["content"])
	}
}

func TestProjectMessagesImageDataURL(t *testing.T) {
	raw := []byte{0x89, 0x50, 0x4e, 0x47}
	items := []session.Item{{
		Type: session.ItemMessage, Role: session.RoleUser,
		Content: []session.ContentPart{
			{Type: session.PartText, Text: "look"},
			{Type: session.PartImage, AttachmentID: "att_1", MimeType: "image/png"},
		},
	}}
	resolve := func(id string) (string, string, string, []byte, error) {
		return "x.png", "image/png", ".zendawn/uploads/att_1/x.png", raw, nil
	}
	out, err := projectMessages("", items, resolve)
	if err != nil {
		t.Fatal(err)
	}
	content, ok := out[0]["content"].([]map[string]any)
	if !ok || len(content) != 2 {
		t.Fatalf("content %#v", out[0]["content"])
	}
	img := content[1]
	if img["type"] != "image_url" {
		t.Fatalf("type %#v", img)
	}
	url := img["image_url"].(map[string]any)["url"].(string)
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	if url != want {
		t.Fatalf("url %q", url)
	}
}

func TestProjectMessagesFileStub(t *testing.T) {
	items := []session.Item{{
		Type: session.ItemMessage, Role: session.RoleUser,
		Content: []session.ContentPart{
			{Type: session.PartFile, AttachmentID: "att_2", Filename: "doc.pdf", MimeType: "application/pdf"},
		},
	}}
	resolve := func(id string) (string, string, string, []byte, error) {
		return "doc.pdf", "application/pdf", ".zendawn/uploads/att_2/doc.pdf", []byte("%PDF"), nil
	}
	out, err := projectMessages("", items, resolve)
	if err != nil {
		t.Fatal(err)
	}
	content := out[0]["content"].([]map[string]any)
	text := content[0]["text"].(string)
	if !strings.Contains(text, ".zendawn/uploads/att_2/doc.pdf") || !strings.Contains(text, "application/pdf") {
		t.Fatalf("stub %q", text)
	}
}
