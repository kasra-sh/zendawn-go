package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/kasra-sh/zendawn-go/attachment"
	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider/openrouter"
	"github.com/kasra-sh/zendawn-go/runtime"
	"github.com/kasra-sh/zendawn-go/session"
)

const (
	padY         = 1
	padX         = 2
	composerMaxH = 8
	composerMinH = 1
)

type (
	evMsg      runtime.Event
	submitDone struct {
		reason string
		files  []pendingFile // restored on failure
	}
	askReqMsg    struct{ action, resource, toolCallID string }
	secretReqMsg struct{ name, description, reason string }
	mcpDoneMsg   struct {
		lines []string
		err   error
	}
	modelsDoneMsg struct {
		modelLabel string
		count      int
		err        error
	}
	mdReadyMsg struct {
		width int
		r     *glamour.TermRenderer
	}
)

type tuiModel struct {
	rt      *runtime.Runtime
	sess    *session.Session
	agent   string
	model   string
	workdir string

	vp     viewport.Model
	input  textarea.Model
	width  int
	height int

	lines         []string
	status        string
	streaming     strings.Builder
	draining      bool
	streamingText bool // true while receiving provider.text_delta

	spin         spinner.Model
	pendingFiles []pendingFile

	slashSugs []string

	askOpen     bool
	askAction   string
	askResource string
	askCh       chan session.AskDecision
	askMu       sync.Mutex
	planAskOpen bool

	secretOpen bool
	secretName string
	secretDesc string
	secretBuf  string
	secretCh   chan string

	mcpReady bool // false until ConnectMCP finishes (or no MCP configured)

	modelsReady       bool // false until OpenRouter catalog refresh finishes (or skipped)
	modelsFetchNeeded bool
	orProviderID      string
	orBaseURL         string
	orAPIKey          string

	md               *glamour.TermRenderer
	mdWidth          int
	lastAssistantRaw string
	lastAssistantIdx int
	stickBottom      bool
	prog             *tea.Program

	promptHistory   []string
	historyIdx      int // -1 = not browsing
	historyDraft    string
	lastSubmitted   string
	lastUserLineIdx int
}

func needMCPConnect(rt *runtime.Runtime) bool {
	return rt != nil && rt.Doc != nil && rt.Doc.MCP != nil && len(rt.Doc.MCP.Servers) > 0
}

func newTUI(rt *runtime.Runtime, sess *session.Session, agentID, modelID, workdir, orProviderID, orBaseURL, orAPIKey string) *tuiModel {
	ta := textarea.New()
	ta.Placeholder = "message… (shift+enter/ctrl+j newline · enter send)"
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(composerMinH)
	ta.SetWidth(80)
	// Match composer dialogPanel fill so textarea SGR does not punch through panelBG.
	panelText := lipgloss.NewStyle().Background(panelBG).Foreground(lipgloss.Color("252"))
	panelMuted := lipgloss.NewStyle().Background(panelBG).Foreground(lipgloss.Color("240"))
	panelBase := lipgloss.NewStyle().Background(panelBG)
	ta.FocusedStyle.Base = panelBase
	ta.FocusedStyle.Text = panelText
	ta.FocusedStyle.Placeholder = panelMuted
	ta.FocusedStyle.CursorLine = panelBase
	ta.BlurredStyle.Base = panelBase
	ta.BlurredStyle.Text = panelMuted
	ta.BlurredStyle.Placeholder = panelMuted
	ta.BlurredStyle.CursorLine = panelBase
	// Enter submits (handled in Update); newlines via shift+enter / ctrl+j.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j"))
	ta.Focus()

	vp := viewport.New(80, 20)
	vp.SetContent("")

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))

	mcpReady := !needMCPConnect(rt)
	fetchModels := orProviderID != "" && openrouter.IsOpenRouterBase(orBaseURL)
	status := "idle"
	switch {
	case !mcpReady && fetchModels:
		status = "connecting mcp · refreshing models…"
	case !mcpReady:
		status = "connecting mcp…"
	case fetchModels:
		status = "refreshing models…"
	}
	m := &tuiModel{
		rt:                rt,
		sess:              sess,
		agent:             agentID,
		model:             modelID,
		workdir:           workdir,
		vp:                vp,
		input:             ta,
		spin:              sp,
		status:            status,
		mcpReady:          mcpReady,
		modelsReady:       !fetchModels,
		modelsFetchNeeded: fetchModels,
		orProviderID:      orProviderID,
		orBaseURL:         orBaseURL,
		orAPIKey:          orAPIKey,
		lines:             []string{lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Ready. /=skill · drop/paste files · /attach · Tab mode · ^p model.")},
		askCh:             make(chan session.AskDecision, 1),
		secretCh:          make(chan string, 1),
		stickBottom:       true,
		lastAssistantIdx:  -1,
		historyIdx:        -1,
		lastUserLineIdx:   -1,
	}
	m.refreshSlashSuggestions()
	m.refreshViewport()
	return m
}

func (m *tuiModel) refreshSlashSuggestions() {
	m.sess.Lock()
	bound := append([]string{}, m.sess.BoundSkills...)
	m.sess.Unlock()
	sugs := make([]string, 0, len(bound)+1)
	sugs = append(sugs, "/skills")
	for _, id := range bound {
		sugs = append(sugs, "/"+id)
	}
	m.slashSugs = sugs
}

func (m *tuiModel) sidebarVisible() bool {
	return sidebarVisibleForWidth(m.width)
}

func (m *tuiModel) mainWidth() int {
	return mainColumnWidth(m.width)
}

func (m *tuiModel) contentWidth() int {
	w := m.mainWidth() - padX*2 - 2
	if w < 20 {
		return 20
	}
	return w
}

func (m *tuiModel) mdWrapWidth() int {
	w := m.contentWidth() - 2
	if w < 20 {
		return 20
	}
	return w
}

func (m *tuiModel) syncComposerSize() {
	v := m.input.Value()
	lines := strings.Count(v, "\n") + 1
	if lines < composerMinH {
		lines = composerMinH
	}
	if lines > composerMaxH {
		lines = composerMaxH
	}
	m.input.SetHeight(lines)
	m.input.SetWidth(panelInnerWidth(m.contentWidth()))
}

func (m *tuiModel) footerStack(mode, modelLabel string) string {
	if mode == "" {
		mode = session.ModeAgent
	}
	maxW := m.contentWidth()
	if maxW < 20 {
		maxW = 20
	}
	clamp := lipgloss.NewStyle().MaxWidth(maxW) // no inherited panel background
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	if strings.Contains(strings.ToLower(m.status), "error") || strings.HasPrefix(m.status, "provider") {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	}
	statusText := m.status
	if m.streamingText {
		statusText = m.spin.View() + " Generating response…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	} else if m.startupPending() {
		statusText = m.spin.View() + " " + m.status
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	} else if hint := m.slashHint(); hint != "" {
		statusText = m.status + " · " + hint
	}
	status := clamp.Render(statusStyle.Render(statusText))
	composerInner := m.input.View()
	if chip := m.chipsView(); chip != "" {
		composerInner = chip + "\n" + composerInner
	}
	composer := dialogPanel(m.contentWidth(), modeRailAccent(mode), composerInner)
	under := clamp.Render(composerMeta(mode, modelLabel))
	help := clamp.Render(lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(
		"/=skill · drop/paste·/attach file · enter send · s-enter/^j nl · tab mode · ^p model · ^x detach · ^a stop · ^c quit",
	))
	parts := make([]string, 0, 5)
	if !m.sidebarVisible() {
		parts = append(parts, clamp.Render(m.usageFooter()))
	}
	parts = append(parts, status, composer, under, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *tuiModel) chipsView() string {
	if len(m.pendingFiles) == 0 {
		return ""
	}
	chipStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("16")).
		Background(lipgloss.Color("81")).
		Padding(0, 1).
		Bold(true)
	var b strings.Builder
	for i, f := range m.pendingFiles {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(chipStyle.Render(f.Filename))
	}
	return b.String()
}

func (m *tuiModel) slashHint() string {
	v := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(v, "/") || strings.Contains(strings.TrimPrefix(v, "/"), " ") || strings.Contains(v, "\n") {
		return ""
	}
	matches := m.matchingSlash(v)
	if len(matches) == 0 {
		return ""
	}
	shown := matches
	if len(shown) > 4 {
		shown = shown[:4]
	}
	return "suggest " + strings.Join(shown, " ")
}

func (m *tuiModel) matchingSlash(prefix string) []string {
	var out []string
	for _, s := range m.slashSugs {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func (m *tuiModel) acceptSlashSuggestion() bool {
	v := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(v, "/") || strings.Contains(v, "\n") {
		return false
	}
	matches := m.matchingSlash(v)
	if len(matches) == 0 {
		return false
	}
	m.input.SetValue(matches[0])
	m.input.CursorEnd()
	m.syncComposerSize()
	return true
}

func (m *tuiModel) layoutViewport() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.sess.Lock()
	mode := m.sess.Mode
	modelLabel := runtime.ActiveModelLabel(m.sess.Provider, m.sess.ModelID)
	m.sess.Unlock()
	if mode == "" {
		mode = session.ModeAgent
	}
	if modelLabel == "/" || modelLabel == "" {
		modelLabel = m.model
	}
	m.syncComposerSize()

	headerH := lipgloss.Height(headerBar(max(40, m.mainWidth()), mode, m.agent, m.workdir))
	footerH := lipgloss.Height(m.footerStack(mode, modelLabel))
	if m.askOpen || m.planAskOpen {
		footerH += 8
	}
	innerW := max(20, m.contentWidth())
	// View = header + body + blank + footer + bottom pad. Never exceed terminal
	// height or alt-screen clips from the top (header disappears).
	const bodyFooterGap = 1
	innerH := m.height - headerH - bodyFooterGap - footerH - padY
	if innerH < 1 {
		innerH = 1
	}
	m.vp.Width = innerW
	m.vp.Height = innerH
}

// buildMDCmd constructs a Glamour renderer off the Update thread so startup
// and resize do not freeze key handling.
func (m *tuiModel) buildMDCmd(width int) tea.Cmd {
	if width < 20 {
		width = 20
	}
	if m.md != nil && m.mdWidth == width {
		return nil
	}
	w := width
	return func() tea.Msg {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(w),
		)
		if err != nil {
			return mdReadyMsg{width: w}
		}
		return mdReadyMsg{width: w, r: r}
	}
}

func (m *tuiModel) connectMCPCmd() tea.Cmd {
	return func() tea.Msg {
		rep, err := m.rt.ConnectMCP(context.Background(), m.sess.ID)
		return mcpDoneMsg{lines: rep.Lines(), err: err}
	}
}

func (m *tuiModel) fetchModelsCmd() tea.Cmd {
	provID := m.orProviderID
	baseURL := m.orBaseURL
	apiKey := m.orAPIKey
	sessionID := m.sess.ID
	rt := m.rt
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := openrouter.ListFreeZDRCodingChatModels(ctx, baseURL, apiKey)
		if err != nil {
			return modelsDoneMsg{err: err}
		}
		if len(models) == 0 {
			return modelsDoneMsg{err: fmt.Errorf("empty free ZDR catalog")}
		}
		yamlModels := append([]config.ModelCatalogEntry(nil), rt.Doc.Models...)
		merged, specs := runtime.MergeOpenRouterCatalog(yamlModels, models, provID)
		p, id, err := rt.ApplyModelsCatalog(sessionID, merged, specs)
		if err != nil {
			return modelsDoneMsg{err: err}
		}
		return modelsDoneMsg{
			modelLabel: runtime.ActiveModelLabel(p, id),
			count:      len(merged),
		}
	}
}

func (m *tuiModel) startupPending() bool {
	return !m.mcpReady || !m.modelsReady
}

func (m *tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.spin.Tick}
	if !m.mcpReady {
		cmds = append(cmds, m.connectMCPCmd())
	}
	if m.modelsFetchNeeded {
		cmds = append(cmds, m.fetchModelsCmd())
	}
	return tea.Batch(cmds...)
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutViewport()
		m.refreshViewport()
		return m, m.buildMDCmd(m.mdWrapWidth())

	case mdReadyMsg:
		if msg.width != m.mdWrapWidth() && m.width > 0 {
			return m, m.buildMDCmd(m.mdWrapWidth())
		}
		if msg.r != nil {
			m.md = msg.r
			m.mdWidth = msg.width
			if m.lastAssistantRaw != "" {
				m.finalizeAssistantMD(m.lastAssistantRaw)
			}
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if m.streamingText || m.startupPending() {
			return m, cmd
		}
		return m, nil

	case mcpDoneMsg:
		m.mcpReady = true
		for _, line := range msg.lines {
			m.appendLine("dim", line)
		}
		if msg.err != nil {
			m.appendLine("err", msg.err.Error())
			m.status = "mcp failed"
		} else if !m.modelsReady {
			m.status = "refreshing models…"
		} else if len(msg.lines) == 0 {
			m.status = "idle"
		} else {
			m.status = "idle · mcp ready"
		}
		m.layoutViewport()
		return m, nil

	case modelsDoneMsg:
		m.modelsReady = true
		if msg.err != nil {
			m.appendLine("dim", "models: using config catalog ("+msg.err.Error()+")")
			if !m.mcpReady {
				m.status = "connecting mcp…"
			} else {
				m.status = "idle · models fallback"
			}
		} else {
			m.model = msg.modelLabel
			m.appendLine("dim", fmt.Sprintf("models: %d free ZDR (coding) · active %s", msg.count, msg.modelLabel))
			if !m.mcpReady {
				m.status = "connecting mcp…"
			} else {
				m.status = "idle · models ready"
			}
		}
		m.layoutViewport()
		return m, nil

	case tea.KeyMsg:
		if msg.Paste {
			if m.secretOpen {
				m.secretBuf = appendSecretPaste(m.secretBuf, string(msg.Runes))
				m.layoutViewport()
				return m, nil
			}
			rest, files := extractFilePathsFromPaste(string(msg.Runes), m.workdir)
			added := false
			for _, f := range files {
				if m.addPendingFile(f) {
					added = true
				}
			}
			if rest != "" {
				cur := m.input.Value()
				if cur != "" && !strings.HasSuffix(cur, " ") && !strings.HasSuffix(cur, "\n") {
					cur += " "
				}
				m.input.SetValue(cur + rest)
				m.input.CursorEnd()
				m.syncComposerSize()
			}
			if added {
				m.status = fmt.Sprintf("attached %d file(s)", len(m.pendingFiles))
			} else if rest == "" && len(files) == 0 {
				// Non-path paste: fall through to textarea.
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				m.syncComposerSize()
				m.layoutViewport()
				return m, cmd
			}
			m.layoutViewport()
			return m, nil
		}
		if m.secretOpen {
			switch msg.String() {
			case "enter":
				m.resolveSecret(m.secretBuf)
			case "esc":
				m.resolveSecret("")
			case "backspace", "ctrl+h":
				if len(m.secretBuf) > 0 {
					m.secretBuf = m.secretBuf[:len(m.secretBuf)-1]
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.secretBuf += string(msg.Runes)
				}
			}
			m.layoutViewport()
			return m, nil
		}
		if m.askOpen {
			switch msg.String() {
			case "o", "O":
				m.resolveAsk(session.AskOnce)
			case "a", "A":
				m.resolveAsk(session.AskAlways)
			case "r", "R", "esc":
				m.resolveAsk(session.AskReject)
			}
			return m, nil
		}
		if m.planAskOpen {
			switch msg.String() {
			case "e", "E":
				_ = m.rt.ResolvePlanAsk(m.sess.ID, session.PlanAskEdit)
				m.planAskOpen = false
				m.status = "plan: edit current"
				m.appendLine("dim", "plan ask → edit")
			case "n", "N":
				_ = m.rt.ResolvePlanAsk(m.sess.ID, session.PlanAskNew)
				m.planAskOpen = false
				m.status = "plan: new"
				m.appendLine("dim", "plan ask → new")
			}
			return m, nil
		}
		if msg.Type == tea.KeyTab || msg.String() == "tab" || msg.String() == "ctrl+t" {
			if m.draining {
				m.status = "busy — wait for this turn or ctrl+a to stop"
				return m, nil
			}
			m.sess.Lock()
			busy := m.sess.Busy
			cur := m.sess.Mode
			m.sess.Unlock()
			if busy {
				m.status = "busy — wait for this turn or ctrl+a to stop"
				return m, nil
			}
			next := nextMode(cur)
			if err := m.rt.SetMode(context.Background(), m.sess.ID, next); err != nil {
				m.status = "mode error: " + err.Error()
				m.appendLine("err", err.Error())
				return m, nil
			}
			m.sess.Lock()
			pending := m.sess.PendingPlanAsk
			m.sess.Unlock()
			if pending {
				m.planAskOpen = true
				m.status = "plan ask"
			} else {
				m.status = "mode " + modeLabel(next)
			}
			return m, nil
		}
		switch msg.String() {
		case "pgup", "ctrl+u":
			m.vp.HalfViewUp()
			m.stickBottom = m.vp.AtBottom()
			return m, nil
		case "pgdown", "ctrl+d":
			m.vp.HalfViewDown()
			m.stickBottom = m.vp.AtBottom()
			return m, nil
		case "shift+enter", "ctrl+j":
			// Newline (not submit). ctrl+j also arrives after Shift+Enter CSI rewrite.
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.syncComposerSize()
			m.layoutViewport()
			return m, cmd
		case "up", "down", "alt+up", "alt+down":
			switch edgeArrowAction(msg.String(), m.input.Line(), m.input.LineCount()) {
			case arrowScrollUp:
				m.vp.LineUp(1)
				m.stickBottom = m.vp.AtBottom()
				return m, nil
			case arrowScrollDown:
				m.vp.LineDown(1)
				m.stickBottom = m.vp.AtBottom()
				return m, nil
			case arrowHistoryUp:
				m.recallHistory(-1)
				return m, nil
			case arrowHistoryDown:
				m.recallHistory(1)
				return m, nil
			}
			// arrowPass: fall through to textarea Update below
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+x":
			if len(m.pendingFiles) > 0 {
				m.pendingFiles = m.pendingFiles[:len(m.pendingFiles)-1]
				m.status = "detached last file"
				m.layoutViewport()
				return m, nil
			}
		case "backspace", "ctrl+h":
			if m.input.Value() == "" && len(m.pendingFiles) > 0 {
				m.pendingFiles = m.pendingFiles[:len(m.pendingFiles)-1]
				m.layoutViewport()
				return m, nil
			}
		case "ctrl+a":
			m.rt.Abort(m.sess.ID)
			m.appendLine("dim", "(stop requested)")
			if m.draining {
				m.status = "stopping…"
			}
			return m, nil
		case "ctrl+p":
			if m.draining {
				m.status = "busy — wait for this turn or ctrl+a to stop"
				return m, nil
			}
			prov, id, err := m.rt.CycleModel(m.sess.ID)
			if err != nil {
				m.status = "model error: " + err.Error()
				m.appendLine("err", err.Error())
				return m, nil
			}
			m.model = runtime.ActiveModelLabel(prov, id)
			m.status = "model " + m.model
			m.appendLine("dim", "model → "+m.model)
			return m, nil
		case "right", "ctrl+e":
			if m.acceptSlashSuggestion() {
				m.layoutViewport()
				return m, nil
			}
			if m.acceptAtPathChip() {
				m.layoutViewport()
				return m, nil
			}
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if m.draining {
				return m, nil
			}
			if m.startupPending() {
				if !m.mcpReady {
					m.status = "wait — mcp connecting…"
				} else {
					m.status = "wait — refreshing models…"
				}
				return m, nil
			}
			if text == "" && len(m.pendingFiles) == 0 {
				return m, nil
			}
			if strings.HasPrefix(text, "/attach") && !strings.Contains(text, "\n") {
				f, ok, errMsg := parseAttachSlash(text, m.workdir)
				m.input.Reset()
				m.syncComposerSize()
				if errMsg != "" {
					m.status = errMsg
					m.appendLine("err", errMsg)
				} else if ok {
					m.addPendingFile(f)
					m.status = "attached " + f.Filename
				}
				m.layoutViewport()
				return m, nil
			}
			if strings.HasPrefix(text, "/") && !strings.Contains(text, "\n") {
				m.input.Reset()
				m.syncComposerSize()
				m.handleSlashSkill(parseSlashSkill(text))
				m.layoutViewport()
				return m, nil
			}
			if m.acceptAtPathChip() {
				m.layoutViewport()
				return m, nil
			}
			files := append([]pendingFile{}, m.pendingFiles...)
			m.lastSubmitted = text
			m.historyIdx = -1
			m.input.Reset()
			m.pendingFiles = nil
			m.syncComposerSize()
			m.stickBottom = true
			m.appendUserTurn(text, files)
			m.draining = true
			m.streamingText = false
			m.status = "running"
			m.streaming.Reset()
			m.layoutViewport()
			return m, m.submitCmd(text, files)
		}

	case evMsg:
		cmd := m.handleEvent(runtime.Event(msg))
		m.layoutViewport()
		return m, cmd

	case submitDone:
		m.draining = false
		m.streamingText = false
		if failedStop(msg.reason) {
			m.pendingFiles = append([]pendingFile{}, msg.files...)
			m.restoreFailedPrompt(msg.reason)
			m.layoutViewport()
			return m, nil
		}
		m.promptHistory = pushPromptHistory(m.promptHistory, m.lastSubmitted)
		m.status = "idle · stop " + msg.reason
		if m.streaming.Len() > 0 {
			m.finalizeAssistantMD(m.streaming.String())
			m.streaming.Reset()
		}
		m.appendLine("stop", "[stop: "+msg.reason+"]")
		m.layoutViewport()
		return m, nil

	case askReqMsg:
		m.askOpen = true
		m.askAction = msg.action
		m.askResource = msg.resource
		m.status = "permission ask"
		m.layoutViewport()
		return m, nil

	case secretReqMsg:
		m.secretOpen = true
		m.secretName = msg.name
		m.secretDesc = msg.description
		m.secretBuf = ""
		m.status = "secret ask"
		m.layoutViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.syncComposerSize()
	m.layoutViewport()
	return m, cmd
}

func (m *tuiModel) handleSlashSkill(cmd slashSkillCmd) {
	if cmd.usage || (!cmd.list && cmd.name == "") {
		m.appendLine("err", slashSkillUsage)
		m.status = "skill usage"
		return
	}
	if cmd.list {
		m.sess.Lock()
		bound := append([]string{}, m.sess.BoundSkills...)
		active := map[string]bool{}
		for k, v := range m.sess.ActiveSkills {
			active[k] = v
		}
		m.sess.Unlock()
		if len(bound) == 0 {
			m.appendLine("dim", "skills: (none bound)")
			m.status = "no skills"
			return
		}
		var b strings.Builder
		b.WriteString("skills:")
		for _, id := range bound {
			b.WriteString(" ")
			b.WriteString(id)
			if active[id] {
				b.WriteString("*")
			}
		}
		m.appendLine("dim", b.String())
		m.status = "skills listed"
		return
	}
	ar, err := m.rt.ActivateSkill(m.sess.ID, cmd.name, cmd.unload)
	if err != nil {
		msg := err.Error()
		if !strings.Contains(strings.ToLower(msg), "skill not found") {
			msg = "skill not found: " + msg
		}
		m.appendLine("err", msg)
		m.status = "skill not found"
		return
	}
	m.appendLine("dim", runtime.FormatActivateLine(ar))
	m.status = runtime.FormatActivateLine(ar)
}

func (m *tuiModel) submitCmd(text string, files []pendingFile) tea.Cmd {
	rt, sessID := m.rt, m.sess.ID
	return func() tea.Msg {
		ids := make([]string, 0, len(files))
		for _, f := range files {
			fh, err := os.Open(f.Path)
			if err != nil {
				return submitDone{reason: "error: " + err.Error(), files: files}
			}
			meta, err := rt.Upload(sessID, f.Filename, "", fh)
			_ = fh.Close()
			if err != nil {
				return submitDone{reason: "error: " + err.Error(), files: files}
			}
			ids = append(ids, meta.ID)
		}
		reason, err := rt.Submit(context.Background(), sessID, session.UserInput{
			Text:          text,
			AttachmentIDs: ids,
		})
		if err != nil {
			return submitDone{reason: "error: " + err.Error(), files: files}
		}
		return submitDone{reason: reason}
	}
}

func (m *tuiModel) addPendingFile(f pendingFile) bool {
	for _, e := range m.pendingFiles {
		if e.Path == f.Path {
			return false
		}
	}
	st, err := os.Stat(f.Path)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	if st.Size() > attachment.MaxBytes {
		m.status = fmt.Sprintf("file too large (max %d MiB): %s", attachment.MaxBytes>>20, f.Filename)
		return false
	}
	m.pendingFiles = append(m.pendingFiles, f)
	return true
}

func (m *tuiModel) acceptAtPathChip() bool {
	v, f, ok := tryAtPathChip(m.input.Value(), m.workdir)
	if !ok {
		return false
	}
	if !m.addPendingFile(f) {
		return true
	}
	m.input.SetValue(v)
	m.input.CursorEnd()
	m.syncComposerSize()
	m.status = "attached " + f.Filename
	return true
}

func (m *tuiModel) appendUserTurn(text string, files []pendingFile) {
	var b strings.Builder
	if text != "" {
		b.WriteString(text)
	}
	if len(files) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		chipStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("16")).
			Background(lipgloss.Color("81")).
			Padding(0, 1).
			Bold(true)
		for i, f := range files {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(chipStyle.Render(f.Filename))
		}
	}
	m.appendLine("user", b.String())
}

func (m *tuiModel) resolveAsk(d session.AskDecision) {
	m.askOpen = false
	m.status = "idle"
	select {
	case m.askCh <- d:
	default:
		select {
		case <-m.askCh:
		default:
		}
		m.askCh <- d
	}
	m.appendLine("perm", fmt.Sprintf("permission %s → %s", m.askAction, d))
	m.layoutViewport()
}

// appendSecretPaste appends pasted text into the secret buffer, stripping
// trailing CR/LF so a pasted key followed by newline does not look like submit.
func appendSecretPaste(buf, pasted string) string {
	return buf + strings.TrimRight(pasted, "\r\n")
}

func (m *tuiModel) resolveSecret(value string) {
	m.secretOpen = false
	m.status = "idle"
	select {
	case m.secretCh <- value:
	default:
		select {
		case <-m.secretCh:
		default:
		}
		m.secretCh <- value
	}
	if value == "" {
		m.appendLine("perm", "secret "+m.secretName+" → cancelled")
	} else {
		m.appendLine("perm", "secret "+m.secretName+" → stored")
	}
	m.secretBuf = ""
	m.layoutViewport()
}

func (m *tuiModel) handleEvent(ev runtime.Event) tea.Cmd {
	switch ev.Type {
	case runtime.EventProviderTextDelta:
		was := m.streamingText
		m.streamingText = true
		t, _ := ev.Data["text"].(string)
		m.streaming.WriteString(t)
		m.replaceStreamLine()
		if !was {
			return m.spin.Tick
		}
	case runtime.EventProviderMsgCompleted:
		m.streamingText = false
		if m.streaming.Len() > 0 {
			raw := m.streaming.String()
			m.streaming.Reset()
			m.finalizeAssistantMD(raw)
		}
	case runtime.EventPlanAsk:
		m.planAskOpen = true
		m.status = "plan ask"
	case runtime.EventPlanResolved:
		m.planAskOpen = false
	case runtime.EventToolStarted:
		m.streamingText = false
		name, _ := ev.Data["name"].(string)
		m.appendLine("tool", "▸ "+name+"…")
	case runtime.EventToolCompleted:
		name, _ := ev.Data["name"].(string)
		isErr, _ := ev.Data["isError"].(bool)
		if isErr {
			m.appendLine("tool", "▸ "+name+" failed")
		} else {
			m.appendLine("tool", "▸ "+name+" ok")
		}
	case runtime.EventDrainStarted:
		m.status = "running"
	case runtime.EventDrainCompleted:
		m.streamingText = false
		reason, _ := ev.Data["reason"].(string)
		m.status = "done · " + reason
	case runtime.EventPermissionAsk:
		action, _ := ev.Data["action"].(string)
		res, _ := ev.Data["resource"].(string)
		m.askAction = action
		m.askResource = res
	case runtime.EventSecretAsk:
		name, _ := ev.Data["name"].(string)
		desc, _ := ev.Data["description"].(string)
		m.secretName = name
		m.secretDesc = desc
	case runtime.EventError:
		m.streamingText = false
		code, _ := ev.Data["code"].(string)
		msg, _ := ev.Data["message"].(string)
		m.appendLine("err", code+": "+msg)
	case runtime.EventUsage:
		// Footer redraws from session state in View.
	case runtime.EventModelChanged:
		prov, _ := ev.Data["provider"].(string)
		id, _ := ev.Data["model"].(string)
		m.model = runtime.ActiveModelLabel(prov, id)
	}
	return nil
}

const streamPrefix = "assistant │ "

func (m *tuiModel) replaceStreamLine() {
	line := streamPrefix + m.streaming.String()
	if len(m.lines) > 0 && strings.HasPrefix(m.lines[len(m.lines)-1], streamPrefix) {
		m.lines[len(m.lines)-1] = line
	} else {
		m.lines = append(m.lines, line)
	}
	m.refreshViewport()
}

func (m *tuiModel) finalizeAssistantMD(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	prevRaw := m.lastAssistantRaw
	rendered := raw
	if m.md != nil {
		if out, err := m.md.Render(raw); err == nil {
			rendered = strings.TrimRight(out, "\n")
		}
	}
	block := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("63")).
		PaddingLeft(1).
		Render(rendered)

	switch {
	case len(m.lines) > 0 && strings.HasPrefix(m.lines[len(m.lines)-1], streamPrefix):
		m.lines[len(m.lines)-1] = block
		m.lastAssistantIdx = len(m.lines) - 1
	case prevRaw == raw && m.lastAssistantIdx >= 0 && m.lastAssistantIdx < len(m.lines):
		// Same message (e.g. Glamour became ready) — upgrade in place.
		m.lines[m.lastAssistantIdx] = block
	default:
		m.lines = append(m.lines, block)
		m.lastAssistantIdx = len(m.lines) - 1
	}
	m.lastAssistantRaw = raw
	m.refreshViewport()
}

func (m *tuiModel) appendLine(kind, text string) {
	var styled string
	switch kind {
	case "user":
		m.sess.Lock()
		mode := m.sess.Mode
		m.sess.Unlock()
		styled = dialogPanel(m.contentWidth(), modeRailAccent(mode), text)
		m.lines = append(m.lines, styled)
		m.lastUserLineIdx = len(m.lines) - 1
		m.refreshViewport()
		return
	case "tool":
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("tool      │ " + text)
	case "perm":
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("permit    │ " + text)
	case "stop":
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("system    │ " + text)
	case "err":
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("error     │ " + text)
	case "dim":
		styled = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("system    │ " + text)
	default:
		styled = text
	}
	m.lines = append(m.lines, styled)
	m.refreshViewport()
}

func (m *tuiModel) refreshViewport() {
	m.vp.SetContent(strings.Join(m.lines, "\n"))
	if m.stickBottom {
		m.vp.GotoBottom()
	}
}

func (m *tuiModel) recallHistory(delta int) {
	idx, text, draft := historyStep(m.promptHistory, m.historyIdx, m.historyDraft, m.input.Value(), delta)
	m.historyIdx = idx
	m.historyDraft = draft
	m.input.SetValue(text)
	m.input.CursorEnd()
	m.syncComposerSize()
	m.layoutViewport()
}

func (m *tuiModel) popLastUserLine() {
	if m.lastUserLineIdx >= 0 && m.lastUserLineIdx < len(m.lines) {
		i := m.lastUserLineIdx
		m.lines = append(m.lines[:i], m.lines[i+1:]...)
		m.lastUserLineIdx = -1
		return
	}
	for i := len(m.lines) - 1; i >= 0; i-- {
		if strings.Contains(m.lines[i], "▌") && !strings.HasPrefix(m.lines[i], streamPrefix) {
			m.lines = append(m.lines[:i], m.lines[i+1:]...)
			return
		}
	}
}

func (m *tuiModel) clearStreamLine() {
	m.streaming.Reset()
	if len(m.lines) > 0 && strings.HasPrefix(m.lines[len(m.lines)-1], streamPrefix) {
		m.lines = m.lines[:len(m.lines)-1]
	}
}

func (m *tuiModel) restoreFailedPrompt(reason string) {
	m.historyIdx = -1
	m.clearStreamLine()
	m.popLastUserLine()
	if text, ok := m.sess.PopFailedTurn(); ok && strings.TrimSpace(text) != "" {
		m.lastSubmitted = text
	}
	m.input.SetValue(m.lastSubmitted)
	m.input.CursorEnd()
	m.syncComposerSize()
	m.status = "failed · edit and resend"
	if reason != "" {
		m.appendLine("err", reason)
	}
	m.refreshViewport()
}

func (m *tuiModel) usageFooter() string {
	m.sess.Lock()
	used := m.sess.LastInputTokens
	limit := m.sess.LastContextLimit
	in, out := m.sess.UsageInput, m.sess.UsageOutput
	cost, known := m.sess.UsageCostUSD, m.sess.UsageCostKnown
	label := runtime.ActiveModelLabel(m.sess.Provider, m.sess.ModelID)
	m.sess.Unlock()
	if label == "/" || label == "" {
		label = m.model
	}
	line := formatSessionUsageLine(used, limit, in, out, cost, known, label)
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(line)
}

func (m *tuiModel) sidebarView(height int) string {
	m.sess.Lock()
	sid := m.sess.ID
	mode := m.sess.Mode
	used := m.sess.LastInputTokens
	limit := m.sess.LastContextLimit
	in, out := m.sess.UsageInput, m.sess.UsageOutput
	cost, known := m.sess.UsageCostUSD, m.sess.UsageCostKnown
	label := runtime.ActiveModelLabel(m.sess.Provider, m.sess.ModelID)
	m.sess.Unlock()
	if mode == "" {
		mode = session.ModeAgent
	}
	if label == "/" || label == "" {
		label = m.model
	}
	return renderSidebar(height, sid, m.agent, mode, label, m.workdir, used, limit, in, out, cost, known)
}

func (m *tuiModel) View() string {
	m.sess.Lock()
	mode := m.sess.Mode
	modelLabel := runtime.ActiveModelLabel(m.sess.Provider, m.sess.ModelID)
	m.sess.Unlock()
	if mode == "" {
		mode = session.ModeAgent
	}
	if modelLabel == "/" || modelLabel == "" {
		modelLabel = m.model
	}

	header := headerBar(max(40, m.mainWidth()), mode, m.agent, m.workdir)

	body := m.vp.View()
	footer := m.footerStack(mode, modelLabel)

	var rest string
	switch {
	case m.secretOpen:
		rest = lipgloss.JoinVertical(lipgloss.Left, body, "", m.secretDialog(), "", footer)
	case m.askOpen:
		rest = lipgloss.JoinVertical(lipgloss.Left, body, "", m.permissionDialog(), "", footer)
	case m.planAskOpen:
		rest = lipgloss.JoinVertical(lipgloss.Left, body, "", m.planAskDialog(), "", footer)
	default:
		rest = lipgloss.JoinVertical(lipgloss.Left, body, "", footer)
	}

	// Header is flush on the main column only; sidebar sits full-height beside it.
	rightPad := padX
	if m.sidebarVisible() {
		rightPad = 1
	}
	padded := lipgloss.NewStyle().
		Padding(0, rightPad, padY, padX).
		Render(rest)
	left := lipgloss.JoinVertical(lipgloss.Left, header, padded)
	mainW := m.mainWidth()
	if mainW < 40 {
		mainW = 40
	}
	left = lipgloss.NewStyle().Width(mainW).MaxWidth(mainW).Render(left)
	if !m.sidebarVisible() {
		return left
	}
	side := m.sidebarView(lipgloss.Height(left))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, side)
}

func (m *tuiModel) secretDialog() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(1, 2).
		Width(min(72, max(40, m.contentWidth())))
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("Secret required")
	desc := m.secretDesc
	if desc == "" {
		desc = m.secretName
	}
	masked := strings.Repeat("•", len([]rune(m.secretBuf)))
	if masked == "" {
		masked = " "
	}
	body := fmt.Sprintf("%s (%s)\n\n%s\n\n[enter] store   [esc] cancel", desc, m.secretName, masked)
	return box.Render(title + "\n\n" + body)
}

func (m *tuiModel) permissionDialog() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(1, 2).
		Width(min(72, max(40, m.contentWidth())))

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("Permission required")
	actionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	if m.askAction == "shell" {
		actionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	}
	body := fmt.Sprintf("%s\n%s\n\n[o] once   [a] always   [r] reject",
		actionStyle.Render(m.askAction),
		"resource: "+m.askResource,
	)
	return box.Render(title + "\n\n" + body)
}

func (m *tuiModel) planAskDialog() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("42")).
		Padding(1, 2).
		Width(min(72, max(40, m.contentWidth())))
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render("Existing plan")
	body := "Edit the current plan, or start a new one?\n\n[e] edit current   [n] new plan"
	return box.Render(title + "\n\n" + body)
}

func (m *tuiModel) waitAsk(action, resource, toolCallID string) session.AskDecision {
	if m.prog != nil {
		m.prog.Send(askReqMsg{action: action, resource: resource, toolCallID: toolCallID})
	}
	d := <-m.askCh
	return d
}

func (m *tuiModel) waitSecret(name, description, reason string) (string, error) {
	if m.prog != nil {
		m.prog.Send(secretReqMsg{name: name, description: description, reason: reason})
	}
	v := <-m.secretCh
	if strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("missing_secret: %s", name)
	}
	return v, nil
}

func runTUI(rt *runtime.Runtime, sess *session.Session, agentID, modelID, workdir, orProviderID, orBaseURL, orAPIKey string) error {
	m := newTUI(rt, sess, agentID, modelID, workdir, orProviderID, orBaseURL, orAPIKey)
	// No WithMouseCellMotion: mouse tracking steals click-drag so conversation
	// text cannot be selected. Scroll with pgup/pgdn (ctrl+u/d). Newline = ctrl+j.
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.prog = p

	evCh := make(chan runtime.Event, 256)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range evCh {
			p.Send(evMsg(ev))
		}
	}()
	rt.OnEvent = func(ev runtime.Event) {
		evCh <- ev
	}
	rt.Ask = func(sessionID, action, resource, toolCallID string) session.AskDecision {
		return m.waitAsk(action, resource, toolCallID)
	}
	rt.SecretAsk = func(name, description, reason string) (string, error) {
		return m.waitSecret(name, description, reason)
	}
	rt.InitSecrets()
	// MCP connect runs from Init (sync before first submit); may secret.ask via TUI.

	_, err := p.Run()
	rt.CloseMCP()
	close(evCh)
	<-done
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
