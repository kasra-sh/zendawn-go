package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/runtime"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/tool"
)

// Vector is a conformance test vector.
type Vector struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Specs    []string `json:"specs"`
	Fixtures Fixtures `json:"fixtures"`
	Actions  []Action `json:"actions"`
	Expect   Expect   `json:"expect"`
}

type Fixtures struct {
	Agent                json.RawMessage                   `json:"agent"`
	Document             json.RawMessage                   `json:"document"`
	SessionAgentID       string                            `json:"sessionAgentId"`
	Tools                []MockTool                        `json:"tools"`
	ProviderScript       []provider.ScriptEntry            `json:"providerScript"`
	ChildProviderScripts map[string][]provider.ScriptEntry `json:"childProviderScripts"`
	AskDecisions         []string                          `json:"askDecisions"`
	AskAutoTimeout       bool                              `json:"askAutoTimeout"`
}

type MockTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Permission  struct {
		Action       string `json:"action"`
		ResourceFrom string `json:"resourceFrom"`
	} `json:"permission"`
	Script []struct {
		WhenArgs map[string]any `json:"whenArgs"`
		Result   struct {
			IsError bool `json:"isError"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	} `json:"script"`
}

type Action struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

type Expect struct {
	DrainReason     string   `json:"drainReason"`
	TranscriptTypes []string `json:"transcriptTypes"`
	Events          []string `json:"events"`
	EventsExact     bool     `json:"eventsExact"`
	ActiveAgentID   string   `json:"activeAgentId"`
	ToolResults     []struct {
		ToolCallID   string `json:"toolCallId"`
		IsError      *bool  `json:"isError"`
		TextContains string `json:"textContains"`
	} `json:"toolResults"`
	ProviderCalls []struct {
		AgentID              string `json:"agentId"`
		InstructionsContains string `json:"instructionsContains"`
	} `json:"providerCalls"`
	Transcript []struct {
		Type string `json:"type"`
		Role string `json:"role"`
	} `json:"transcript"`
}

// Dir returns the specs/conformance directory.
func Dir() (string, error) {
	candidates := []string{
		filepath.Join("..", "specs", "conformance"),
		filepath.Join("..", "..", "specs", "conformance"),
	}
	wd, _ := os.Getwd()
	for _, c := range candidates {
		p := filepath.Clean(filepath.Join(wd, c))
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p, nil
		}
	}
	// from module root
	p := filepath.Join(wd, "..", "specs", "conformance")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p, nil
	}
	return "", fmt.Errorf("conformance dir not found from %s", wd)
}

// LoadAll loads all vector JSON files.
func LoadAll() ([]Vector, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Vector
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var v Vector
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, v)
	}
	return out, nil
}

// RunVector executes one vector.
func RunVector(t *testing.T, v Vector) {
	t.Helper()

	doc, agentID, err := loadDoc(v)
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	rt := runtime.New(doc)
	mock := provider.NewMock(normalizeScript(v.Fixtures.ProviderScript))
	if len(v.Fixtures.ChildProviderScripts) > 0 {
		children := map[string][]provider.ScriptEntry{}
		for k, script := range v.Fixtures.ChildProviderScripts {
			children[k] = normalizeScript(script)
		}
		mock.SetChildScripts(children)
	}
	rt.RegisterProvider(mock)

	for _, mt := range v.Fixtures.Tools {
		registerMockTool(rt, mt)
	}

	var askMu sync.Mutex
	askIdx := 0
	decisions := append([]string{}, v.Fixtures.AskDecisions...)
	rt.Ask = func(sessionID, action, resource, toolCallID string) session.AskDecision {
		if v.Fixtures.AskAutoTimeout {
			time.Sleep(time.Hour) // wait for ask timeout
			return session.AskReject
		}
		askMu.Lock()
		defer askMu.Unlock()
		if askIdx >= len(decisions) {
			return session.AskReject
		}
		d := session.AskDecision(decisions[askIdx])
		askIdx++
		return d
	}

	sess, err := rt.Create(agentID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx := context.Background()
	var lastReason string
	for _, a := range v.Actions {
		switch a.Op {
		case "submit":
			lastReason, err = rt.Submit(ctx, sess.ID, session.UserInput{Text: a.Text})
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
		case "steer":
			lastReason, err = rt.Steer(ctx, sess.ID, session.UserInput{Text: a.Text})
			if err != nil {
				t.Fatalf("steer: %v", err)
			}
		default:
			t.Fatalf("unknown op %s", a.Op)
		}
	}

	if v.Expect.DrainReason != "" && lastReason != v.Expect.DrainReason {
		t.Fatalf("drainReason: got %q want %q", lastReason, v.Expect.DrainReason)
	}

	hist := sess.History()
	if len(v.Expect.TranscriptTypes) > 0 {
		got := make([]string, len(hist))
		for i, it := range hist {
			got[i] = it.Type
		}
		if !equalStrings(got, v.Expect.TranscriptTypes) {
			t.Fatalf("transcriptTypes: got %v want %v", got, v.Expect.TranscriptTypes)
		}
	}
	if len(v.Expect.Transcript) > 0 {
		if len(hist) != len(v.Expect.Transcript) {
			t.Fatalf("transcript len: got %d want %d", len(hist), len(v.Expect.Transcript))
		}
		for i, exp := range v.Expect.Transcript {
			if hist[i].Type != exp.Type {
				t.Fatalf("transcript[%d].type: got %q want %q", i, hist[i].Type, exp.Type)
			}
			if exp.Role != "" && hist[i].Role != exp.Role {
				t.Fatalf("transcript[%d].role: got %q want %q", i, hist[i].Role, exp.Role)
			}
		}
	}

	events := rt.Events()
	if len(v.Expect.Events) > 0 {
		types := make([]string, len(events))
		for i, e := range events {
			types[i] = e.Type
		}
		if v.Expect.EventsExact {
			if !equalStrings(types, v.Expect.Events) {
				t.Fatalf("events exact: got %v want %v", types, v.Expect.Events)
			}
		} else if !isSubsequence(types, v.Expect.Events) {
			t.Fatalf("events subsequence missing: got %v want subsequence %v", types, v.Expect.Events)
		}
	}

	if v.Expect.ActiveAgentID != "" && sess.ActiveAgent != v.Expect.ActiveAgentID {
		t.Fatalf("activeAgentId: got %q want %q", sess.ActiveAgent, v.Expect.ActiveAgentID)
	}

	for _, tr := range v.Expect.ToolResults {
		found := false
		for _, it := range hist {
			if it.Type != session.ItemToolResult || it.ToolCallID != tr.ToolCallID {
				continue
			}
			found = true
			if tr.IsError != nil && it.IsError != *tr.IsError {
				t.Fatalf("toolResult %s isError: got %v want %v", tr.ToolCallID, it.IsError, *tr.IsError)
			}
			if tr.TextContains != "" && !strings.Contains(session.TextParts(it.Content), tr.TextContains) {
				t.Fatalf("toolResult %s text: %q missing %q", tr.ToolCallID, session.TextParts(it.Content), tr.TextContains)
			}
		}
		if !found {
			t.Fatalf("toolResult %s not found", tr.ToolCallID)
		}
	}

	if len(v.Expect.ProviderCalls) > 0 {
		if len(mock.Calls) < len(v.Expect.ProviderCalls) {
			t.Fatalf("providerCalls: got %d want >= %d", len(mock.Calls), len(v.Expect.ProviderCalls))
		}
		for i, exp := range v.Expect.ProviderCalls {
			call := mock.Calls[i]
			if exp.AgentID != "" && call.AgentID != exp.AgentID {
				t.Fatalf("providerCalls[%d].agentId: got %q want %q", i, call.AgentID, exp.AgentID)
			}
			if exp.InstructionsContains != "" && !strings.Contains(call.Instructions, exp.InstructionsContains) {
				t.Fatalf("providerCalls[%d].instructions missing %q in %q", i, exp.InstructionsContains, call.Instructions)
			}
		}
	}
}

func loadDoc(v Vector) (*config.Document, string, error) {
	if len(v.Fixtures.Document) > 0 && string(v.Fixtures.Document) != "null" {
		doc, err := config.ParseJSON(v.Fixtures.Document)
		if err != nil {
			return nil, "", err
		}
		id := v.Fixtures.SessionAgentID
		if id == "" {
			for k := range doc.Agents {
				id = k
				break
			}
		}
		return doc, id, nil
	}
	doc, err := config.ParseJSON(v.Fixtures.Agent)
	if err != nil {
		return nil, "", err
	}
	return doc, doc.Single.ID, nil
}

func normalizeScript(in []provider.ScriptEntry) []provider.ScriptEntry {
	out := make([]provider.ScriptEntry, len(in))
	for i, e := range in {
		out[i] = e
		if e.Message != nil {
			m := *e.Message
			m.Type = session.ItemMessage
			if m.Role == "" {
				m.Role = session.RoleAssistant
			}
			out[i].Message = &m
		}
	}
	return out
}

func registerMockTool(rt *runtime.Runtime, mt MockTool) {
	def := tool.Definition{
		Name:        mt.Name,
		Description: mt.Description,
		InputSchema: mt.InputSchema,
		Permission: tool.PermSpec{
			Action:       mt.Permission.Action,
			ResourceFrom: mt.Permission.ResourceFrom,
		},
	}
	scripts := mt.Script
	rt.Tools.Register(def, func(ctx context.Context, name string, args map[string]any) (tool.Result, error) {
		for _, s := range scripts {
			if argsMatch(s.WhenArgs, args) {
				parts := make([]tool.ContentPart, 0, len(s.Result.Content))
				for _, c := range s.Result.Content {
					parts = append(parts, tool.ContentPart{Type: c.Type, Text: c.Text})
				}
				return tool.Result{IsError: s.Result.IsError, Content: parts}, nil
			}
		}
		return tool.TextResult("mock_tool_no_script", true, "mock_tool_no_script"), nil
	})
}

func argsMatch(when, args map[string]any) bool {
	if len(when) == 0 && len(args) == 0 {
		return true
	}
	for k, v := range when {
		av, ok := args[k]
		if !ok {
			return false
		}
		if fmt.Sprint(av) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isSubsequence(haystack, needle []string) bool {
	i := 0
	for _, h := range haystack {
		if i < len(needle) && h == needle[i] {
			i++
		}
	}
	return i == len(needle)
}
