# GoCode

A Claude Code style terminal coding agent written in Go, themed orange on black.
It talks to the Claude API with **your own Anthropic API key** and lets you **choose the model**.

```
╭─────────────────────────────────────────────────────────╮
│ ✻ Welcome to GoCode!                                    │
│                                                         │
│   /help for help, /model to switch model, /exit to quit │
│                                                         │
│   cwd:    /Users/you/project                            │
│   model:  claude-opus-5-5  (effort: high)               │
╰─────────────────────────────────────────────────────────╯
```

## Table of contents

- [Features](#features)
- [Requirements](#requirements)
- [Install](#install)
- [First run](#first-run)
- [Usage](#usage)
- [Commands](#commands)
- [Tools and permissions](#tools-and-permissions)
- [Configuration](#configuration)
- [Models and cost](#models-and-cost)
- [Platform notes](#platform-notes)
- [Build from source](#build-from-source)
- [Development](#development)
- [Project layout](#project-layout)
- [Troubleshooting](#troubleshooting)
- [Releasing](#releasing-maintainers)
- [License](#license)

## Features

- Streaming responses rendered as lightweight markdown (headers, bullets, code blocks, bold, inline code)
- Agentic tool loop: `bash`, `read_file`, `write_file`, `edit_file`, `list_dir`, `glob`, `grep`
- Claude Code style output: `⏺ Tool(arg)` headers with `⎿ result` previews and inline diffs for edits
- Permission prompts before commands and file writes (`y` / `a` always / `n`), or `/auto` to skip them
- First-run onboarding: enter and validate an API key, then choose a model from the live Models API
- `/model` picker, `/effort` control, `/compact` context summarization, `/cost` tracking
- Prompt caching on the system prompt and a deterministic tool list to keep cache hits high
- Command history, tab completion for slash commands, Ctrl+C to interrupt a response
- Runs on macOS, Linux and Windows (Windows Terminal recommended)

## Requirements

| Requirement | Needed for | Notes |
|---|---|---|
| **Anthropic API key** | Everything | Create one at https://console.anthropic.com/settings/keys. Keys start with `sk-ant-`. Usage is billed to your Anthropic account. |
| **Internet access** | Everything | GoCode calls `api.anthropic.com`. |
| **A terminal with 24-bit color and UTF-8** | Correct rendering | macOS Terminal/iTerm2, any modern Linux terminal, or Windows Terminal. Legacy Windows `conhost` renders glyphs poorly. |
| **Go 1.22+** | Only if building from source | Tested with Go 1.27. Not needed if you use a prebuilt binary or the install scripts. |
| **Git** | Optional | Used to show the current branch and change count in the system prompt. Everything works without it. |

Prebuilt binaries are fully static (`CGO_ENABLED=0`), so there are no runtime libraries to install.

### Go module dependencies

These are fetched automatically by `go build` / `go install`; you do not install them by hand.

| Module | Purpose |
|---|---|
| `github.com/anthropics/anthropic-sdk-go` | Official Claude API SDK (streaming, tools, models) |
| `github.com/chzyer/readline` | Line editing, history, tab completion, masked key input |
| `golang.org/x/sys` | Enabling ANSI colors and UTF-8 on the Windows console |

## Install

Pick one.

### macOS / Linux (one line)

```sh
curl -fsSL https://raw.githubusercontent.com/saurabh-sol/cli/main/install.sh | sh
```

Installs the latest release to `/usr/local/bin/gocode` (or `~/.local/bin/gocode` if that is not writable).

### Windows (PowerShell, one line)

```powershell
irm https://raw.githubusercontent.com/saurabh-sol/cli/main/install.ps1 | iex
```

Installs to `%LOCALAPPDATA%\gocode\gocode.exe` and adds that folder to your user PATH. Open a new terminal afterwards.

### Manual download

Download the archive for your OS and CPU from the [Releases page](https://github.com/saurabh-sol/cli/releases),
extract it, and put `gocode` (or `gocode.exe`) somewhere on your PATH.

| File | Platform |
|---|---|
| `gocode_darwin_arm64.tar.gz` | macOS, Apple Silicon |
| `gocode_darwin_amd64.tar.gz` | macOS, Intel |
| `gocode_linux_amd64.tar.gz` | Linux x86-64 |
| `gocode_linux_arm64.tar.gz` | Linux ARM64 |
| `gocode_windows_amd64.zip` | Windows x64 |
| `gocode_windows_arm64.zip` | Windows on ARM |

### With Go

```sh
go install github.com/saurabh-sol/cli@latest
```

This installs a binary named `cli` (the repository name) into `$(go env GOPATH)/bin`. Rename it or
alias it to `gocode` if you prefer. To get the `gocode` name directly, build from source below.

## First run

1. Open a terminal in the project folder you want to work on and run `gocode`.
2. GoCode asks for your **Anthropic API key**. Paste it (input is hidden) and press Enter.
   The key is validated against the API, then saved to the config file with owner-only permissions.
3. Pick a **model** from the numbered list, or press Enter to keep the default `claude-opus-5-5`.
4. Type your first message.

Instead of entering the key interactively you can export it before starting:

```sh
export ANTHROPIC_API_KEY=sk-ant-...     # macOS / Linux
$env:ANTHROPIC_API_KEY = "sk-ant-..."   # Windows PowerShell
```

## Usage

```sh
gocode                              # start in the current directory
gocode --model claude-sonnet-5-5    # use a specific model for this session
gocode --auto                       # skip permission prompts for this session
gocode --version
gocode --help
```

Type a message and press Enter. The model streams its answer, calls tools as needed, and asks
before running commands or editing files. Press Ctrl+C to interrupt a response. Use the arrow
keys to browse history and Tab to complete slash commands.

Prefix a line with `!` to run a shell command yourself without involving the model:

```
> !git status
```

## Commands

| Command | Description |
|---|---|
| `/help` | Show help |
| `/model [id]` | Interactive model picker, or set a model id directly |
| `/effort <low\|medium\|high\|xhigh\|max>` | Set reasoning effort (default `high`) |
| `/key` | Enter a new API key |
| `/auto` | Toggle auto-approval of tool calls (persisted) |
| `/cwd [path]` | Show or change the working directory |
| `/compact` | Summarize the conversation to free context |
| `/clear` | Start a new conversation |
| `/cost` | Token usage and estimated cost for the session |
| `/status` | Model, effort, cwd, message count, masked key |
| `/exit` | Quit (Ctrl+D or Ctrl+C twice also quits) |
| `!cmd` | Run a shell command directly |

## Tools and permissions

The model can call these tools. Read-only tools run immediately; the others ask first unless
auto-approve is on.

| Tool | Asks permission | What it does |
|---|---|---|
| `read_file` | No | Read a file with line numbers (supports offset/limit) |
| `list_dir` | No | List a directory |
| `glob` | No | Find files by pattern, e.g. `**/*.go` |
| `grep` | No | Regex search across files |
| `bash` | Yes | Run a shell command in the working directory |
| `write_file` | Yes | Create or overwrite a file |
| `edit_file` | Yes | Replace an exact string in a file (shows a diff) |

At a permission prompt answer `y` (allow once), `a` (allow this tool for the rest of the session),
or `n` (deny; the model is told and asked to choose another approach).

## Configuration

Config file location:

| OS | Path |
|---|---|
| macOS / Linux | `~/.gocode/config.json` |
| Windows | `C:\Users\<you>\.gocode\config.json` |

```json
{
  "api_key": "sk-ant-...",
  "model": "claude-opus-5-5",
  "effort": "high",
  "auto_approve": false
}
```

Command history is stored next to it in `~/.gocode/history`. Delete the `.gocode` folder to reset everything.

Environment variables:

| Variable | Effect |
|---|---|
| `ANTHROPIC_API_KEY` | Used when the config file has no key |
| `SHELL` | Shell used by the `bash` tool on macOS/Linux (default `/bin/sh`) |

## Models and cost

Any model id your account can access works. Known models with list prices (USD per million tokens):

| Model id | Input | Output | Notes |
|---|---|---|---|
| `claude-opus-5-5` | $4 | $20 | Default |
| `claude-fable-5-1` | $10 | $50 | Most capable |
| `claude-sonnet-5-5` | $2 | $10 | Fast everyday coding |
| `claude-haiku-5-5` | $0.10 | $0.50 | Cheapest |
| `claude-opus-5`, `claude-opus-4-8`, `claude-opus-4-7`, `claude-opus-4-6` | $5 | $25 | Previous generations |
| `claude-sonnet-5` | $2 | $10 | Previous generation |
| `claude-sonnet-4-6` | $3 | $15 | Previous generation |
| `claude-haiku-4-5` | $1 | $5 | No adaptive thinking |

Adaptive thinking is enabled on models that support it, and `/effort` maps to the API's
`output_config.effort`. `/cost` shows an estimate based on these prices; your Anthropic console
is the source of truth for billing.

## Platform notes

### macOS / Linux
- Commands run through `$SHELL -c` (zsh, bash, etc.).
- The install script needs `curl` and `tar`, which are present on all common systems.

### Windows
- Use **Windows Terminal** (free in the Microsoft Store) or another console that supports ANSI
  colors and UTF-8. GoCode enables virtual-terminal processing and the UTF-8 code page on startup.
- The `bash` tool runs commands through PowerShell 7 (`pwsh`) if installed, otherwise Windows
  PowerShell, otherwise `cmd.exe`. The model is told which shell is active and writes PowerShell syntax.
- SmartScreen may warn about an unsigned download. Choose "More info" then "Run anyway", or build from source.

## Build from source

Requires Go 1.22 or newer (https://go.dev/dl/). Dependencies download automatically on first build.

macOS / Linux:

```sh
git clone https://github.com/saurabh-sol/cli.git
cd cli
go build -o gocode .
./gocode
```

Windows (PowerShell):

```powershell
git clone https://github.com/saurabh-sol/cli.git
cd cli
go build -o gocode.exe .
.\gocode.exe
```

Build every platform at once into `dist/` (from macOS or Linux):

```sh
./build.sh
```

## Development

```sh
go vet ./...          # static checks
go test ./...         # unit tests (tools, glob matcher, renderer)
gofmt -l .            # formatting check
GOOS=windows GOARCH=amd64 go build -o /dev/null .   # cross-compile check
```

CI runs vet, tests and a build on Ubuntu, macOS and Windows for every push and pull request
(`.github/workflows/ci.yml`).

## Project layout

| File | Purpose |
|---|---|
| `main.go` | Entry point, readline REPL, first-run onboarding, CLI flags |
| `agent.go` | Streaming request loop, tool execution, permissions, usage tracking, compaction |
| `tools.go` | Tool schemas and implementations |
| `commands.go` | Slash commands, model picker, API key prompt |
| `ui.go` | Orange/black theme, banner, spinner, markdown renderer |
| `config.go` | Config persistence and model catalog with pricing |
| `platform_windows.go` / `platform_other.go` | Console setup and shell selection per OS |
| `tools_test.go` | Unit tests |
| `install.sh` / `install.ps1` | One-line installers |
| `build.sh` | Local multi-platform build |
| `.goreleaser.yaml`, `.github/workflows/` | Release automation and CI |

## Troubleshooting

| Symptom | Fix |
|---|---|
| `Authentication failed (401)` | Re-enter the key with `/key`. Make sure it starts with `sk-ant-` and is active in the Anthropic console. |
| `Model not found (404)` | Run `/model` and pick from the list of models your account can access. |
| `Rate limited (429)` / `overloaded (529)` | Wait a moment and retry, or switch to a smaller model with `/model`. |
| No colors or broken box characters | Use a modern terminal (Windows Terminal, iTerm2, GNOME Terminal). |
| Response hit the max_tokens limit | Type "continue". |
| Conversation feels slow or expensive | Run `/compact` to summarize history, or lower `/effort`. |
| `gocode: command not found` after install | Open a new terminal, or add the install directory to your PATH as the installer printed. |
| Antivirus flags the `.exe` | Unsigned Go binaries sometimes trigger this. Build from source or add an exclusion. |

## Releasing (maintainers)

Push a version tag and GitHub Actions (GoReleaser) builds and publishes binaries and checksums
for all six platforms to the Releases page:

```sh
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT. See [LICENSE](LICENSE).
