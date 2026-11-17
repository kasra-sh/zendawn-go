package local

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kasra-sh/zendawn-go/tool"
)

const maxOutput = 64 * 1024

// Options configures local coding tools.
type Options struct {
	WorkDir string
}

// Register adds read, write, glob, grep, and shell to the registry.
func Register(reg *tool.Registry, opts Options) error {
	wd := opts.WorkDir
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	wd, err := filepath.Abs(wd)
	if err != nil {
		return err
	}

	reg.Register(tool.Definition{
		Name:        "read",
		Description: "Read a text file under the working directory. Args: path (relative or absolute within workdir).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required":             []any{"path"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "read", ResourceFrom: "path"},
	}, func(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
		path, _ := args["path"].(string)
		full, err := resolvePath(wd, path)
		if err != nil {
			return tool.TextResult(err.Error(), true, "path_escape"), nil
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return tool.TextResult(err.Error(), true, "read_error"), nil
		}
		return tool.TextResult(truncate(string(b)), false, ""), nil
	})

	reg.Register(tool.Definition{
		Name:        "write",
		Description: "Create or overwrite a text file under the working directory. Args: path, content.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
			},
			"required":             []any{"path", "content"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "edit", ResourceFrom: "path"},
	}, func(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		full, err := resolvePath(wd, path)
		if err != nil {
			return tool.TextResult(err.Error(), true, "path_escape"), nil
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return tool.TextResult(err.Error(), true, "write_error"), nil
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return tool.TextResult(err.Error(), true, "write_error"), nil
		}
		return tool.TextResult("wrote "+path, false, ""), nil
	})

	reg.Register(tool.Definition{
		Name:        "glob",
		Description: "List files matching a glob pattern under the working directory. Args: pattern (e.g. **/*.go).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
			},
			"required":             []any{"pattern"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "glob", ResourceFrom: "pattern"},
	}, func(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
		pattern, _ := args["pattern"].(string)
		matches, err := walkGlob(wd, pattern)
		if err != nil {
			return tool.TextResult(err.Error(), true, "glob_error"), nil
		}
		if len(matches) == 0 {
			return tool.TextResult("(no matches)", false, ""), nil
		}
		return tool.TextResult(truncate(strings.Join(matches, "\n")), false, ""), nil
	})

	reg.Register(tool.Definition{
		Name:        "grep",
		Description: "Search file contents under the working directory. Args: pattern (regex or literal), optional path (subdir or file).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
				"path":    map[string]any{"type": "string"},
			},
			"required":             []any{"pattern"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "grep", ResourceFrom: "pattern"},
	}, func(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
		pattern, _ := args["pattern"].(string)
		sub, _ := args["path"].(string)
		root := wd
		if sub != "" {
			var err error
			root, err = resolvePath(wd, sub)
			if err != nil {
				return tool.TextResult(err.Error(), true, "path_escape"), nil
			}
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			re = regexp.MustCompile(regexp.QuoteMeta(pattern))
		}
		var lines []string
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if len(lines) >= 100 {
				return filepath.SkipAll
			}
			b, err := os.ReadFile(path)
			if err != nil || bytes.IndexByte(b, 0) >= 0 {
				return nil
			}
			rel, _ := filepath.Rel(wd, path)
			for i, line := range strings.Split(string(b), "\n") {
				if re.MatchString(line) {
					lines = append(lines, fmt.Sprintf("%s:%d:%s", rel, i+1, line))
					if len(lines) >= 100 {
						return filepath.SkipAll
					}
				}
			}
			return nil
		})
		if len(lines) == 0 {
			return tool.TextResult("(no matches)", false, ""), nil
		}
		return tool.TextResult(truncate(strings.Join(lines, "\n")), false, ""), nil
	})

	reg.Register(tool.Definition{
		Name:        "shell",
		Description: "Run a shell command in the working directory (requires user approval). Args: command.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
			"required":             []any{"command"},
			"additionalProperties": false,
		},
		Permission: tool.PermSpec{Action: "shell", ResourceFrom: "command"},
	}, func(ctx context.Context, _ string, args map[string]any) (tool.Result, error) {
		command, _ := args["command"].(string)
		if strings.TrimSpace(command) == "" {
			return tool.TextResult("empty command", true, "shell_error"), nil
		}
		cmd := exec.CommandContext(ctx, "bash", "-lc", command)
		cmd.Dir = wd
		var buf bytes.Buffer
		cmd.Stdout = &limitedWriter{w: &buf, n: maxOutput}
		cmd.Stderr = &limitedWriter{w: &buf, n: maxOutput}
		err := cmd.Run()
		out := buf.String()
		if out == "" && err != nil {
			out = err.Error()
		}
		if err != nil {
			return tool.TextResult(truncate(out), true, "shell_error"), nil
		}
		if out == "" {
			out = "(exit 0, no output)"
		}
		return tool.TextResult(truncate(out), false, ""), nil
	})

	return nil
}

func resolvePath(workdir, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	var full string
	if filepath.IsAbs(p) {
		full = filepath.Clean(p)
	} else {
		full = filepath.Clean(filepath.Join(workdir, p))
	}
	rel, err := filepath.Rel(workdir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes workdir: %s", p)
	}
	return full, nil
}

func walkGlob(workdir, pattern string) ([]string, error) {
	pattern = strings.TrimPrefix(pattern, "./")
	var out []string
	// Support ** by walking
	if strings.Contains(pattern, "**") {
		re := globToRegexp(pattern)
		err := filepath.WalkDir(workdir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(workdir, path)
			if rel == "." {
				return nil
			}
			if re.MatchString(filepath.ToSlash(rel)) {
				out = append(out, filepath.ToSlash(rel))
			}
			if len(out) >= 500 {
				return filepath.SkipAll
			}
			return nil
		})
		return out, err
	}
	matches, err := filepath.Glob(filepath.Join(workdir, pattern))
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		rel, _ := filepath.Rel(workdir, m)
		out = append(out, filepath.ToSlash(rel))
	}
	return out, nil
}

func globToRegexp(pattern string) *regexp.Regexp {
	p := filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); {
		if i+1 < len(p) && p[i] == '*' && p[i+1] == '*' {
			b.WriteString(".*")
			i += 2
			if i < len(p) && p[i] == '/' {
				i++
			}
			continue
		}
		switch p[i] {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(p[i])))
		}
		i++
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func truncate(s string) string {
	if len(s) <= maxOutput {
		return s
	}
	return s[:maxOutput] + "\n…(truncated)"
}

type limitedWriter struct {
	w io.Writer
	n int
	c int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.c >= l.n {
		return len(p), nil
	}
	remain := l.n - l.c
	if len(p) > remain {
		_, _ = l.w.Write(p[:remain])
		l.c = l.n
		return len(p), nil
	}
	n, err := l.w.Write(p)
	l.c += n
	return n, err
}
