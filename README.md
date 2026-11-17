# zendawn-go

Go implementation of the [zendawn](../README.md) agent harness.

Normative behavior: [`../specs`](../specs).

## Module

```text
github.com/kasra-sh/zendawn-go
```

## Packages

| Package | Role |
| --- | --- |
| `config` | YAML/JSON load, validation, model profile policy |
| `session` | Sessions and transcript items |
| `permission` | Allow / ask / deny |
| `tool` | Registry, snapshot, executors |
| `tool/local` | Local read/write/glob/grep/shell for the CLI |
| `provider` | Provider interface, mock, OpenAI-compatible HTTP |
| `hook` | Named lifecycle hooks (drain/step/tool + session/plan/execution) |
| `paths` | Workspace `.zendawn` DMZ + global XDG config (`plans/`, `skills/`) |
| `skill` | `SKILL.md` loader (DMZ → skillsDirs → global) |
| `mcp` | Optional MCP client (stdio + HTTP) |
| `secret` | Secret vault + `secret.ask` for `${VAR}` templates |
| `runtime` | Drain/step loop, orchestration (`task` / `handoff`) |
| `conformance` | Shared vector runner |

## Example config

Bundled example (OpenRouter coding + optional Tavily MCP):

```text
examples/agent.yaml
```

Copy or point `-config` at it. Set `OPENROUTER_API_KEY` (and optionally `TAVILY_API_KEY`, or let the TUI prompt).

## Test

From this directory (sandbox off / normal local Go cache):

```bash
go test ./...
```

Conformance loads `../specs/conformance/*.json`.

## CLI (Bubble Tea TUI)

Default UI is an OpenCode/Claude-style TUI: streaming chat, **ask / plan / agent** modes (Tab), and a graphical permission dialog for shell/edits/MCP.

```bash
cd zendawn-go   # repo folder name; module is github.com/kasra-sh/zendawn-go
go run ./cmd/zendawn \
  -config examples/agent.yaml \
  -workdir .
```

API key resolution (startup): `-api-key` → config `apiKey` → env (`providers.*.apiKeyEnv`, e.g. `OPENROUTER_API_KEY`). If still missing, the CLI **prompts** (hidden input on a TTY), then sets the key for the process before the TUI starts.

On TUI start with an OpenRouter `baseUrl`, the app refreshes the models catalog from OpenRouter (free + zero data retention + chat, sorted by coding). The session switches to the first entry; `^p` cycles that list. YAML `models` / `model.id` are the offline fallback if the fetch fails.

| Key | Action |
| --- | --- |
| Tab | Cycle mode: ask → plan → agent |
| Enter | Submit message |
| o / a / r | Permission once / always / reject |
| ctrl+p | Cycle model |
| ctrl+a | Abort drain |
| ctrl+c | Quit |

Modes: **ask** (read-only + MCP tools), **plan** (inspect, no edits), **agent** (full tools + approvals).

Classic line REPL: add `-plain`.

## Release bundles (Linux / macOS / Windows)

One script cross-compiles a folder + zip with the binary, `agent.yaml`, README, and a run helper:

```bash
cd zendawn-go
chmod +x scripts/bundle.sh

# Linux
./scripts/bundle.sh linux amd64
# → dist/linux-amd64/{zendawn,agent.yaml,README.txt,run.sh}
# → dist/zendawn-linux-amd64.zip

./scripts/bundle.sh linux arm64

# macOS
./scripts/bundle.sh darwin amd64   # Intel
./scripts/bundle.sh darwin arm64   # Apple Silicon
# → dist/darwin-arm64/...
# → dist/zendawn-darwin-arm64.zip

# Windows
./scripts/bundle.sh windows amd64
# or: ./scripts/bundle-windows.sh
# → dist/windows-amd64/{zendawn.exe,agent.yaml,README.txt,run.bat}
# → dist/zendawn-windows-amd64.zip
```

Run from the unpacked folder:

```bash
# Linux / macOS
./run.sh
# or: ./zendawn -config agent.yaml -workdir .

# Windows
run.bat
# or: zendawn.exe -config agent.yaml -workdir .
```

The app prompts for `OPENROUTER_API_KEY` if unset. Optional MCP search may prompt for `TAVILY_API_KEY`.

## Minimal host sketch

```go
doc, _ := config.LoadFile("examples/agent.yaml")
rt := runtime.New(doc)
p := provider.NewOpenAI("openai", "https://openrouter.ai/api/v1", "OPENROUTER_API_KEY")
rt.RegisterProvider(p)
sess, _ := rt.Create("openrouter-coding")
reason, _ := rt.Submit(ctx, sess.ID, session.UserInput{Text: "Hello"})
_ = reason
```
