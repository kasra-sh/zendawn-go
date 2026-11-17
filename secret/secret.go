package secret

import (
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/zerr"
)

// Source / persist defaults.
const (
	SourceEnv      = "env"
	SourceEnvOrAsk = "env_or_ask"
	SourceAsk      = "ask"
	PersistSession = "session"
	PersistProcess = "process"
)

var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Decl is a normalized secret declaration.
type Decl struct {
	Name        string
	Description string
	Source      string
	Persist     string
}

// AskFunc is invoked when a declared secret must be collected from the host.
// Returns the secret value, or an error / empty to fail.
type AskFunc func(name, description, reason string) (string, error)

// Store is an in-memory secret vault with template expansion.
type Store struct {
	mu        sync.RWMutex
	decls     map[string]Decl
	session   map[string]string
	process   map[string]string
	Ask       AskFunc
	LookupEnv func(string) (string, bool) // defaults to os.LookupEnv
}

// New creates a store from config declarations.
func New(decls []config.SecretDecl) *Store {
	s := &Store{
		decls:     map[string]Decl{},
		session:   map[string]string{},
		process:   map[string]string{},
		LookupEnv: os.LookupEnv,
	}
	for _, d := range decls {
		src := d.Source
		if src == "" {
			src = SourceEnvOrAsk
		}
		persist := d.Persist
		if persist == "" {
			persist = PersistSession
		}
		s.decls[d.Name] = Decl{
			Name:        d.Name,
			Description: d.Description,
			Source:      src,
			Persist:     persist,
		}
	}
	return s
}

// Provide stores a value (used by host after secret.ask).
func (s *Store) Provide(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	persist := PersistSession
	if d, ok := s.decls[name]; ok {
		persist = d.Persist
	}
	if persist == PersistProcess {
		s.process[name] = value
	} else {
		s.session[name] = value
	}
}

// ClearSession removes session-scoped secrets.
func (s *Store) ClearSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = map[string]string{}
}

// Clear removes one secret from both maps.
func (s *Store) Clear(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.session, name)
	delete(s.process, name)
}

// Has reports whether a non-empty value is in the vault (not env).
func (s *Store) Has(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v := s.session[name]; v != "" {
		return true
	}
	return s.process[name] != ""
}

// Values returns a copy of all vault values (for redaction).
func (s *Store) Values() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.session)+len(s.process))
	for _, v := range s.session {
		if v != "" {
			out = append(out, v)
		}
	}
	for _, v := range s.process {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Redact replaces known vault values in text.
func (s *Store) Redact(text string) string {
	for _, v := range s.Values() {
		if v != "" && strings.Contains(text, v) {
			text = strings.ReplaceAll(text, v, "[REDACTED]")
		}
	}
	return text
}

// Get resolves a single secret name without expanding templates.
// reason is passed to Ask when prompting.
func (s *Store) Get(name, reason string) (string, error) {
	s.mu.RLock()
	if v := s.session[name]; v != "" {
		s.mu.RUnlock()
		return v, nil
	}
	if v := s.process[name]; v != "" {
		s.mu.RUnlock()
		return v, nil
	}
	decl, declared := s.decls[name]
	s.mu.RUnlock()

	lookup := s.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	source := SourceEnv // undeclared: env only
	if declared {
		source = decl.Source
	}

	switch source {
	case SourceAsk:
		return s.ask(name, decl.Description, reason)
	case SourceEnv:
		if v, ok := lookup(name); ok && v != "" {
			return v, nil
		}
		return "", zerr.New(zerr.CodeMissingSecret, name, false)
	default: // env_or_ask
		if v, ok := lookup(name); ok && v != "" {
			return v, nil
		}
		if !declared {
			return "", zerr.New(zerr.CodeMissingSecret, name, false)
		}
		return s.ask(name, decl.Description, reason)
	}
}

func (s *Store) ask(name, description, reason string) (string, error) {
	if s.Ask == nil {
		return "", zerr.New(zerr.CodeMissingSecret, name, false)
	}
	v, err := s.Ask(name, description, reason)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(v) == "" {
		return "", zerr.New(zerr.CodeMissingSecret, name, false)
	}
	s.Provide(name, v)
	return v, nil
}

// Expand replaces ${NAME} placeholders in s.
func (s *Store) Expand(text, reason string) (string, error) {
	var firstErr error
	out := placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := placeholderRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		val, err := s.Get(sub[1], reason)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		return val
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

// ExpandMap expands all values in a string map.
func (s *Store) ExpandMap(m map[string]string, reason string) (map[string]string, error) {
	if m == nil {
		return nil, nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		ev, err := s.Expand(v, reason)
		if err != nil {
			return nil, err
		}
		out[k] = ev
	}
	return out, nil
}

// ExpandSlice expands each string in a slice.
func (s *Store) ExpandSlice(ss []string, reason string) ([]string, error) {
	if ss == nil {
		return nil, nil
	}
	out := make([]string, len(ss))
	for i, v := range ss {
		ev, err := s.Expand(v, reason)
		if err != nil {
			return nil, err
		}
		out[i] = ev
	}
	return out, nil
}
