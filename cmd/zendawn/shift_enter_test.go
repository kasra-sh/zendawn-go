package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRewriteShiftEnterSeqs(t *testing.T) {
	for _, seq := range shiftEnterSeqs {
		got := rewriteShiftEnter(append([]byte(nil), seq...))
		if !bytes.Equal(got, []byte{'\n'}) {
			t.Fatalf("seq %q → %q, want LF", seq, got)
		}
	}
	mixed := append([]byte("ab"), append(shiftEnterSeqs[0], 'c')...)
	got := rewriteShiftEnter(mixed)
	if !bytes.Equal(got, []byte("ab\nc")) {
		t.Fatalf("mixed → %q", got)
	}
}

func TestShiftEnterReader(t *testing.T) {
	raw := append([]byte("hi"), append(shiftEnterSeqs[0], []byte("there")...)...)
	r := newShiftEnterReader(bytes.NewReader(raw))
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte("hi\nthere")) {
		t.Fatalf("got %q", out)
	}
}

func TestSplitHoldIncompleteESC(t *testing.T) {
	out, hold := splitHoldIncompleteESC([]byte("x\x1b[13;2"))
	if string(out) != "x" || !bytes.Equal(hold, []byte("\x1b[13;2")) {
		t.Fatalf("out=%q hold=%q", out, hold)
	}
	out, hold = splitHoldIncompleteESC([]byte("plain"))
	if string(out) != "plain" || hold != nil {
		t.Fatalf("out=%q hold=%q", out, hold)
	}
	// Bare ESC[ must not be held (mouse CSI shares this prefix).
	out, hold = splitHoldIncompleteESC([]byte("y\x1b["))
	if !bytes.Equal(out, []byte("y\x1b[")) || hold != nil {
		t.Fatalf("bare ESC[: out=%q hold=%q", out, hold)
	}
	out, hold = splitHoldIncompleteESC([]byte("\x1b"))
	if !bytes.Equal(out, []byte("\x1b")) || hold != nil {
		t.Fatalf("bare ESC: out=%q hold=%q", out, hold)
	}
}

func TestMouseWheelPassthrough(t *testing.T) {
	wheel := []byte("\x1b[<65;58;16M")
	burst := bytes.Repeat(wheel, 5)
	r := newShiftEnterReader(bytes.NewReader(burst))
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, burst) {
		t.Fatalf("mouse CSI mutated:\ngot  %q\nwant %q", out, burst)
	}
	if strings.Contains(string(out), "\n") && !bytes.Contains(burst, []byte("\n")) {
		t.Fatal("unexpected LF injection into mouse stream")
	}
}

func TestIsShiftEnterHoldPrefix(t *testing.T) {
	if isShiftEnterHoldPrefix([]byte("\x1b")) || isShiftEnterHoldPrefix([]byte("\x1b[")) {
		t.Fatal("must not hold bare ESC/[")
	}
	if isShiftEnterHoldPrefix([]byte("\x1b[<")) {
		t.Fatal("must not hold SGR mouse prefix")
	}
	if !isShiftEnterHoldPrefix([]byte("\x1b[13")) {
		t.Fatal("should hold \\x1b[13")
	}
	if !isShiftEnterHoldPrefix([]byte("\x1b[27")) {
		t.Fatal("should hold \\x1b[27")
	}
}
