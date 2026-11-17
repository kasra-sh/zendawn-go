package main

import (
	"bytes"
	"io"
)

// Kitty / xterm sequences for Shift+Enter (Bubble Tea v1 does not decode these).
var shiftEnterSeqs = [][]byte{
	[]byte("\x1b[13;2u"),    // kitty CSI-u: enter + shift
	[]byte("\x1b[27;2;13~"), // xterm modifyOtherKeys
	[]byte("\x1b[13;2~"),    // variant seen in some terminals
}

const maxShiftEnterSeq = 16 // longest sequence above is 9

// shiftEnterReader rewrites Shift+Enter CSI sequences to LF (ctrl+j) so the
// textarea InsertNewline binding can insert a line break.
type shiftEnterReader struct {
	r          io.Reader
	buf        []byte
	pendingErr error
}

func newShiftEnterReader(r io.Reader) *shiftEnterReader {
	return &shiftEnterReader{r: r}
}

func (s *shiftEnterReader) Read(p []byte) (int, error) {
	for {
		if len(s.buf) > 0 {
			s.buf = rewriteShiftEnter(s.buf)
			out, rest := splitHoldIncompleteESC(s.buf)
			s.buf = rest
			if len(out) > 0 {
				n := copy(p, out)
				if n < len(out) {
					s.buf = append(out[n:], s.buf...)
				}
				return n, nil
			}
			// Only an incomplete ESC tail remains — need more input.
		}

		if s.pendingErr != nil {
			err := s.pendingErr
			s.pendingErr = nil
			return 0, err
		}

		tmp := make([]byte, max(len(p), 256))
		n, err := s.r.Read(tmp)
		if n > 0 {
			s.buf = append(s.buf, tmp[:n]...)
		}
		if err != nil {
			if len(s.buf) == 0 {
				return 0, err
			}
			s.pendingErr = err
			// flush remaining (including incomplete ESC) on error/EOF
			s.buf = rewriteShiftEnter(s.buf)
			if len(s.buf) == 0 {
				e := s.pendingErr
				s.pendingErr = nil
				return 0, e
			}
			nn := copy(p, s.buf)
			s.buf = s.buf[nn:]
			return nn, nil
		}
	}
}

func rewriteShiftEnter(b []byte) []byte {
	for _, seq := range shiftEnterSeqs {
		b = bytes.ReplaceAll(b, seq, []byte{'\n'})
	}
	return b
}

// minShiftEnterHold is the shortest unique Shift+Enter prefix we will buffer.
// Bare "\x1b" / "\x1b[" must NOT be held — SGR mouse (e.g. "\x1b[<65;…M") shares
// those bytes and must reach Bubble Tea immediately.
const minShiftEnterHold = 4 // "\x1b[13" or "\x1b[27"

// splitHoldIncompleteESC returns (deliverable, hold) where hold is a trailing
// incomplete ESC prefix that might still become a Shift+Enter sequence.
func splitHoldIncompleteESC(b []byte) (out, hold []byte) {
	if len(b) == 0 {
		return nil, nil
	}
	esc := bytes.LastIndexByte(b, '\x1b')
	if esc < 0 {
		return b, nil
	}
	tail := b[esc:]
	if len(tail) >= maxShiftEnterSeq {
		return b, nil
	}
	if !isShiftEnterHoldPrefix(tail) {
		return b, nil
	}
	return b[:esc], tail
}

func isShiftEnterHoldPrefix(tail []byte) bool {
	if len(tail) < minShiftEnterHold {
		return false
	}
	for _, seq := range shiftEnterSeqs {
		if bytes.HasPrefix(seq, tail) {
			return true
		}
	}
	return false
}
