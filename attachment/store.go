package attachment

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/kasra-sh/zendawn-go/paths"
)

const (
	// MaxBytes is the per-file upload/artifact size limit (20 MiB).
	MaxBytes int64 = 20 << 20

	SourceUpload   = "upload"
	SourceArtifact = "artifact"

	metaFileName = "meta.json"
)

// Meta describes one session attachment on disk.
type Meta struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Source   string `json:"source"`  // upload | artifact
	RelPath  string `json:"relPath"` // workdir-relative path to the blob
}

// Store indexes attachments for one session under workdir/.zendawn/uploads.
type Store struct {
	mu      sync.RWMutex
	workdir string
	byID    map[string]Meta
	nextID  uint64
}

// NewStore creates an empty in-memory index bound to workdir.
func NewStore(workdir string) *Store {
	return &Store{
		workdir: workdir,
		byID:    map[string]Meta{},
	}
}

// List returns a copy of all attachment metadata.
func (s *Store) List() []Meta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Meta, 0, len(s.byID))
	for _, m := range s.byID {
		out = append(out, m)
	}
	return out
}

// Get returns metadata for an id.
func (s *Store) Get(id string) (Meta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.byID[id]
	return m, ok
}

// Open returns metadata and a reader for the blob. Caller closes the reader.
func (s *Store) Open(id string) (Meta, io.ReadCloser, error) {
	m, ok := s.Get(id)
	if !ok {
		return Meta{}, nil, fmt.Errorf("unknown attachment: %s", id)
	}
	full := filepath.Join(s.workdir, m.RelPath)
	f, err := os.Open(full)
	if err != nil {
		return Meta{}, nil, err
	}
	return m, f, nil
}

// ReadBytes loads the full blob (for vision projection). Enforces MaxBytes.
func (s *Store) ReadBytes(id string) (Meta, []byte, error) {
	m, rc, err := s.Open(id)
	if err != nil {
		return Meta{}, nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, MaxBytes+1))
	if err != nil {
		return Meta{}, nil, err
	}
	if int64(len(data)) > MaxBytes {
		return Meta{}, nil, fmt.Errorf("attachment exceeds %d bytes", MaxBytes)
	}
	return m, data, nil
}

// Put writes a new attachment from r. Empty or oversized inputs fail.
func (s *Store) Put(filename, mimeType, source string, r io.Reader) (Meta, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return Meta{}, err
	}
	return s.putBytes(filename, mimeType, source, data)
}

// PutFile copies a workdir file into the store as an artifact (or upload).
func (s *Store) PutFile(srcPath, source string) (Meta, error) {
	if source == "" {
		source = SourceArtifact
	}
	full := srcPath
	if !filepath.IsAbs(full) {
		full = filepath.Join(s.workdir, srcPath)
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(s.workdir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return Meta{}, fmt.Errorf("path escapes workdir: %s", srcPath)
	}
	f, err := os.Open(full)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Meta{}, err
	}
	if !st.Mode().IsRegular() {
		return Meta{}, fmt.Errorf("not a regular file: %s", srcPath)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return Meta{}, err
	}
	return s.putBytes(filepath.Base(full), "", source, data)
}

func (s *Store) putBytes(filename, mimeType, source string, data []byte) (Meta, error) {
	if source == "" {
		source = SourceUpload
	}
	filename = safeFilename(filename)
	if filename == "" {
		filename = "file"
	}
	if mimeType == "" {
		mimeType = sniffMime(filename)
	}
	if len(data) == 0 {
		return Meta{}, fmt.Errorf("empty attachment")
	}
	if int64(len(data)) > MaxBytes {
		return Meta{}, fmt.Errorf("attachment exceeds %d bytes", MaxBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := fmt.Sprintf("att_%d", s.nextID)

	dir := paths.UploadDir(s.workdir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Meta{}, err
	}
	blobPath := filepath.Join(dir, filename)
	if err := os.WriteFile(blobPath, data, 0o644); err != nil {
		return Meta{}, err
	}

	rel, err := filepath.Rel(s.workdir, blobPath)
	if err != nil {
		return Meta{}, err
	}
	rel = filepath.ToSlash(rel)

	m := Meta{
		ID:       id,
		Filename: filename,
		MimeType: mimeType,
		Size:     int64(len(data)),
		Source:   source,
		RelPath:  rel,
	}
	metaPath := filepath.Join(dir, metaFileName)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Meta{}, err
	}
	if err := os.WriteFile(metaPath, b, 0o644); err != nil {
		return Meta{}, err
	}
	s.byID[id] = m
	return m, nil
}

func safeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == ".." {
		return "file"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == 0:
			continue
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == metaFileName {
		return "file"
	}
	return out
}

func sniffMime(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		return "application/octet-stream"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// IsImageMime reports whether mime is an image/* type.
func IsImageMime(mimeType string) bool {
	mimeType = strings.TrimSpace(strings.ToLower(mimeType))
	return strings.HasPrefix(mimeType, "image/")
}
