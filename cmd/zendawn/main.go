package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/runtime"
	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/tool/local"
)

func main() {
	configPath := flag.String("config", "", "path to agent YAML/JSON")
	agentID := flag.String("agent", "", "agent id (default: single-agent id or first multi-agent)")
	baseURL := flag.String("base-url", "https://api.openai.com/v1", "OpenAI-compatible API base URL (default provider fallback)")
	apiKey := flag.String("api-key", "", "API key override for default provider (else model.apiKey, else env)")
	providerID := flag.String("provider", "", "provider registry id (default: agent model.provider)")
	workDir := flag.String("workdir", "", "working directory for local tools (default: cwd)")
	mode := flag.String("mode", "", "initial mode: ask|plan|agent (overrides agent defaultMode)")
	plain := flag.Bool("plain", false, "use classic line REPL instead of TUI")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "usage: zendawn -config path/to/agent.yaml [-base-url URL] [-workdir DIR] [-plain]")
		os.Exit(2)
	}

	doc, err := config.LoadFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	aid := *agentID
	if aid == "" {
		aid, err = defaultAgentID(doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}
	agent, err := doc.Agent(aid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	provName := *providerID
	if provName == "" {
		provName = agent.Model.Provider
	}
	key, keyEnv, err := ensureAPIKey(doc, agent, provName, *apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "api key: %v\n", err)
		os.Exit(1)
	}

	wd := *workDir
	if wd == "" {
		wd, _ = os.Getwd()
	}
	wd, _ = filepath.Abs(wd)
	cfgDir, _ := filepath.Abs(filepath.Dir(*configPath))

	rt := runtime.New(doc)
	rt.WorkDir = wd
	rt.ConfigDir = cfgDir
	rt.DefaultMode = *mode

	registerProviders(rt, doc, provName, *baseURL, keyEnv, key)

	if err := local.Register(rt.Tools, local.Options{WorkDir: wd}); err != nil {
		fmt.Fprintf(os.Stderr, "local tools: %v\n", err)
		os.Exit(1)
	}

	sess, err := rt.Create(aid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session: %v\n", err)
		os.Exit(1)
	}

	if *plain {
		runPlain(rt, sess, agent, aid, *baseURL, wd)
		return
	}

	orBase := resolveProviderBaseURL(rt, provName, *baseURL)
	if err := runTUI(rt, sess, aid, agent.Model.Provider+"/"+agent.Model.ID, wd, provName, orBase, key); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}

// resolveProviderBaseURL returns the registered OpenAI-compatible base URL for providerID.
func resolveProviderBaseURL(rt *runtime.Runtime, providerID, fallback string) string {
	if p, ok := rt.Providers[providerID]; ok {
		if o, ok := p.(*provider.OpenAI); ok && o.BaseURL != "" {
			return o.BaseURL
		}
	}
	return strings.TrimRight(fallback, "/")
}

// registerProviders registers document providers plus a fallback for the agent's default.
func registerProviders(rt *runtime.Runtime, doc *config.Document, defaultProv, baseURL, keyEnv, key string) {
	if len(doc.Providers) > 0 {
		for id, pc := range doc.Providers {
			bu := pc.BaseURL
			if bu == "" {
				bu = "https://api.openai.com/v1"
			}
			env := pc.APIKeyEnv
			if env == "" {
				env = "OPENAI_API_KEY"
			}
			p := provider.NewOpenAI(id, bu, env)
			if pc.APIKey != "" {
				p = p.WithAPIKey(pc.APIKey)
			}
			// CLI overrides apply only to the agent's default provider.
			if id == defaultProv {
				if baseURL != "" && baseURL != "https://api.openai.com/v1" {
					p.BaseURL = strings.TrimRight(baseURL, "/")
				}
				if key != "" {
					p = p.WithAPIKey(key)
				}
			}
			rt.RegisterProvider(p)
		}
		// Ensure default provider exists even if omitted from map.
		if _, ok := rt.Providers[defaultProv]; !ok {
			p := provider.NewOpenAI(defaultProv, baseURL, keyEnv).WithAPIKey(key)
			rt.RegisterProvider(p)
		}
		return
	}
	p := provider.NewOpenAI(defaultProv, baseURL, keyEnv).WithAPIKey(key)
	rt.RegisterProvider(p)
}

func runPlain(rt *runtime.Runtime, sess *session.Session, agent *config.Agent, aid, baseURL, wd string) {
	in := bufio.NewReader(os.Stdin)
	rt.Ask = func(sessionID, action, resource, toolCallID string) session.AskDecision {
		if action == "shell" {
			fmt.Printf("\n[permission ask] Allow shell? command=%s\n", resource)
		} else {
			fmt.Printf("\n[permission ask] action=%s resource=%s toolCallId=%s\n", action, resource, toolCallID)
		}
		fmt.Print("once / always / reject [reject]: ")
		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		switch line {
		case "once", "o":
			return session.AskOnce
		case "always", "a":
			return session.AskAlways
		default:
			return session.AskReject
		}
	}
	rt.SecretAsk = func(name, description, reason string) (string, error) {
		label := description
		if label == "" {
			label = name
		}
		fmt.Printf("\n[secret ask] %s (%s) reason=%s\n", label, name, reason)
		fmt.Print("value: ")
		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return "", fmt.Errorf("missing_secret: %s", name)
		}
		return line, nil
	}
	rt.InitSecrets()
	rep, err := rt.ConnectMCP(context.Background(), sess.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp: %v\n", err)
	}
	for _, line := range rep.Lines() {
		fmt.Println(line)
	}
	defer rt.CloseMCP()

	fmt.Printf("zendawn-cli (plain)  agent=%s  model=%s  base=%s  workdir=%s  session=%s\n",
		aid, agent.Model.ID, baseURL, wd, sess.ID)
	fmt.Printf("agent tools: %s\n", formatAgentTools(agent))
	fmt.Println("Type a message, or /quit /abort")

	ctx := context.Background()
	for {
		fmt.Print("> ")
		line, err := in.ReadString('\n')
		if err != nil {
			fmt.Println()
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case line == "/quit" || line == "/exit":
			return
		case line == "/abort":
			rt.Abort(sess.ID)
			fmt.Println("(aborted)")
			continue
		}

		reason, err := rt.Submit(ctx, sess.ID, session.UserInput{Text: line})
		if err != nil {
			fmt.Fprintf(os.Stderr, "submit: %v\n", err)
			continue
		}
		printTurn(sess)
		fmt.Printf("[stop: %s]\n", reason)
	}
}

func defaultAgentID(doc *config.Document) (string, error) {
	if doc.Single != nil {
		return doc.Single.ID, nil
	}
	for id := range doc.Agents {
		return id, nil
	}
	return "", fmt.Errorf("no agents in document")
}

func formatAgentTools(agent *config.Agent) string {
	if agent.ToolsIsAll() {
		return "* (all registered non-orchestration tools); shell/edit may require approval"
	}
	names := agent.ToolNames()
	if len(names) == 0 {
		return "(none) — model cannot call read/shell; set tools in the agent YAML"
	}
	return strings.Join(names, ", ") + " (shell/edit require approval when configured as ask)"
}

func printTurn(sess *session.Session) {
	hist := sess.History()
	lastUser := -1
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Type == session.ItemMessage && hist[i].Role == session.RoleUser {
			lastUser = i
			break
		}
	}
	for i := lastUser + 1; i < len(hist); i++ {
		it := hist[i]
		switch it.Type {
		case session.ItemToolResult:
			text := session.TextParts(it.Content)
			if len(text) > 200 {
				text = text[:200] + "…"
			}
			errMark := ""
			if it.IsError {
				errMark = " ERROR"
			}
			fmt.Printf("[tool %s%s] %s\n", it.Name, errMark, text)
		case session.ItemHandoff:
			fmt.Printf("[handoff] %s → %s (%s)\n", it.From, it.To, it.Reason)
		case session.ItemMessage:
			if it.Role == session.RoleAssistant {
				text := session.TextParts(it.Content)
				if text != "" {
					fmt.Println(text)
				}
			}
		}
	}
}
