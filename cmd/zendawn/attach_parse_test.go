package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFilePathsFromPaste(t *testing.T) {
	wd := t.TempDir()
	f := filepath.Join(wd, "a.png")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rest, files := extractFilePathsFromPaste(`"`+f+`" please look`, wd)
	if len(files) != 1 || files[0].Filename != "a.png" {
		t.Fatalf("files %#v", files)
	}
	if rest != "please look" {
		t.Fatalf("rest %q", rest)
	}
}

func TestExtractFileURI(t *testing.T) {
	wd := t.TempDir()
	f := filepath.Join(wd, "b.txt")
	if err := os.WriteFile(f, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + f
	rest, files := extractFilePathsFromPaste(uri, wd)
	if rest != "" || len(files) != 1 {
		t.Fatalf("rest=%q files=%#v", rest, files)
	}
}

func TestTryAtPathChip(t *testing.T) {
	wd := t.TempDir()
	f := filepath.Join(wd, "c.md")
	if err := os.WriteFile(f, []byte("#"), 0o644); err != nil {
		t.Fatal(err)
	}
	newV, file, ok := tryAtPathChip("see @"+f, wd)
	if !ok || file.Filename != "c.md" || newV != "see" {
		t.Fatalf("got %q %#v %v", newV, file, ok)
	}
}

func TestParseAttachSlash(t *testing.T) {
	wd := t.TempDir()
	f := filepath.Join(wd, "d.bin")
	if err := os.WriteFile(f, []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, ok, errMsg := parseAttachSlash("/attach "+f, wd)
	if !ok || errMsg != "" || file.Filename != "d.bin" {
		t.Fatalf("%v %q %#v", ok, errMsg, file)
	}
	_, ok, errMsg = parseAttachSlash("/attach", wd)
	if ok || errMsg == "" {
		t.Fatal("expected usage error")
	}
}
