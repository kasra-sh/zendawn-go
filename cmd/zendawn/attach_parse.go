package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// pendingFile is a local path queued for upload on submit.
type pendingFile struct {
	Path     string
	Filename string
}

// extractFilePathsFromPaste finds existing regular-file paths in pasted text
// (terminal drag-drop / bracketed paste). Returns remaining text and paths.
func extractFilePathsFromPaste(pasted, workdir string) (rest string, files []pendingFile) {
	tokens := tokenizePaste(pasted)
	var kept []string
	seen := map[string]bool{}
	for _, tok := range tokens {
		p, ok := normalizeDroppedPath(tok, workdir)
		if !ok {
			kept = append(kept, tok)
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		files = append(files, pendingFile{Path: p, Filename: filepath.Base(p)})
	}
	rest = strings.TrimSpace(strings.Join(kept, " "))
	return rest, files
}

func tokenizePaste(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	var b strings.Builder
	inQuote := rune(0)
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range s {
		switch {
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			} else {
				b.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = r
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	// Also split on newlines if join collapsed oddly.
	if len(out) == 1 && strings.Contains(out[0], "\n") {
		var lines []string
		for _, line := range strings.Split(out[0], "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				lines = append(lines, line)
			}
		}
		return lines
	}
	return out
}

func normalizeDroppedPath(tok, workdir string) (string, bool) {
	tok = strings.TrimSpace(tok)
	tok = strings.Trim(tok, `"'`)
	if tok == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(tok), "file://") {
		u, err := url.Parse(tok)
		if err != nil {
			return "", false
		}
		tok = u.Path
		if decoded, err := url.PathUnescape(tok); err == nil {
			tok = decoded
		}
	}
	if strings.HasPrefix(tok, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			tok = filepath.Join(home, tok[2:])
		}
	}
	if !filepath.IsAbs(tok) {
		tok = filepath.Join(workdir, tok)
	}
	tok = filepath.Clean(tok)
	st, err := os.Stat(tok)
	if err != nil || !st.Mode().IsRegular() {
		return "", false
	}
	abs, err := filepath.Abs(tok)
	if err != nil {
		return tok, true
	}
	return abs, true
}

// tryAtPathChip extracts a trailing @path token from the composer value.
func tryAtPathChip(value, workdir string) (newValue string, file pendingFile, ok bool) {
	v := strings.TrimRightFunc(value, unicode.IsSpace)
	idx := strings.LastIndex(v, "@")
	if idx < 0 {
		return value, pendingFile{}, false
	}
	// @ must start a token (start or after whitespace).
	if idx > 0 && !unicode.IsSpace(rune(v[idx-1])) {
		return value, pendingFile{}, false
	}
	pathTok := strings.TrimSpace(v[idx+1:])
	if pathTok == "" || strings.ContainsAny(pathTok, " \t\n") {
		return value, pendingFile{}, false
	}
	p, ok := normalizeDroppedPath(pathTok, workdir)
	if !ok {
		return value, pendingFile{}, false
	}
	newValue = strings.TrimSpace(v[:idx])
	return newValue, pendingFile{Path: p, Filename: filepath.Base(p)}, true
}

// parseAttachSlash handles `/attach <path>`.
func parseAttachSlash(line, workdir string) (file pendingFile, ok bool, errMsg string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/attach") {
		return pendingFile{}, false, ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/attach"))
	if rest == "" {
		return pendingFile{}, false, "usage: /attach <path>"
	}
	p, ok := normalizeDroppedPath(rest, workdir)
	if !ok {
		return pendingFile{}, false, "file not found: " + rest
	}
	return pendingFile{Path: p, Filename: filepath.Base(p)}, true, ""
}
