package attachment

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPutDownloadRoundTrip(t *testing.T) {
	wd := t.TempDir()
	s := NewStore(wd)
	payload := []byte("hello image")
	m, err := s.Put("shot.png", "image/png", SourceUpload, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || m.Filename != "shot.png" || m.MimeType != "image/png" {
		t.Fatalf("meta: %+v", m)
	}
	full := filepath.Join(wd, m.RelPath)
	if _, err := os.Stat(full); err != nil {
		t.Fatal(err)
	}
	got, rc, err := s.Open(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if got.ID != m.ID || !bytes.Equal(b, payload) {
		t.Fatalf("roundtrip mismatch")
	}
	if !IsImageMime(m.MimeType) {
		t.Fatal("expected image mime")
	}
}

func TestPutRejectsEmptyAndOversize(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Put("a.txt", "", SourceUpload, bytes.NewReader(nil)); err == nil {
		t.Fatal("expected empty error")
	}
	big := bytes.NewReader(make([]byte, MaxBytes+1))
	if _, err := s.Put("big.bin", "", SourceUpload, big); err == nil {
		t.Fatal("expected oversize error")
	}
}

func TestPutFileArtifactStaysInWorkdir(t *testing.T) {
	wd := t.TempDir()
	src := filepath.Join(wd, "out.txt")
	if err := os.WriteFile(src, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(wd)
	m, err := s.PutFile("out.txt", SourceArtifact)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != SourceArtifact {
		t.Fatalf("source %s", m.Source)
	}
	if !strings.HasPrefix(filepath.ToSlash(m.RelPath), ".zendawn/uploads/") {
		t.Fatalf("relPath %s", m.RelPath)
	}
	rel, err := filepath.Rel(wd, filepath.Join(wd, m.RelPath))
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("escapes workdir: %v %s", err, rel)
	}
}

func TestPutFileRejectsEscape(t *testing.T) {
	wd := t.TempDir()
	s := NewStore(wd)
	if _, err := s.PutFile("../outside", SourceArtifact); err == nil {
		t.Fatal("expected escape error")
	}
}
