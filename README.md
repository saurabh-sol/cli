# GoCode

A Claude Code style terminal coding agent written in Go, themed orange on black.
It talks to the Claude API with your own Anthropic API key and lets you pick the model.

## Features

- Streaming responses rendered as lightweight markdown (headers, bullets, code blocks, bold, inline code)
- Agentic tool loop: `bash`, `read_file`, `write_file`, `edit_file`, `list_dir`, `glob`, `grep`
- Claude Code style output: `⏺ Tool(arg)` headers with `⎿ result` previews and inline diffs for edits
- Permission prompts before commands and file writes (`y` / `a` always / `n`), or `/auto` to skip them
- First-run onboarding: enter and validate an API key, then choose a model from the live Models API
- `/model` picker, `/effort` control, `/compact` context summarization, `/cost` tracking
- Prompt caching on the system prompt and a deterministic tool list to keep cache hits high
- Command history, tab completion for slash commands, Ctrl+C to interrupt a response

## Install

**macOS / Linux** (one line):

```sh
curl -fsSL https://raw.githubusercontent.com/saurabh-sol/cli/main/install.sh | sh
```

**Windows** (PowerShell, one line):

```powershell
irm https://raw.githubusercontent.com/saurabh-sol/cli/main/install.ps1 | iex
```

**Go users:**

```sh
go install github.com/saurabh-sol/cli@latest
```

**Manual:** download the archive for your platform from the
[Releases page](https://github.com/saurabh-sol/cli/releases), extract it, and put `gocode`
(or `gocode.exe`) somewhere on your PATH.

Then run `gocode` in any project folder. The first run asks for your Anthropic API key and lets
you choose a model.

## Build from source

macOS / Linux:

```sh
cd gocode
go build -o gocode .
./gocode
```

Windows (PowerShell):

```powershell
cd gocode
go build -o gocode.exe .
.\gocode.exe
```

Or build every platform at once into `dist/` with `./build.sh` (from macOS or Linux).

Requires Go 1.22 or newer. Tested with Go 1.27.

### Windows notes

- Use Windows Terminal or any console that supports ANSI colors and UTF-8. GoCode enables
  virtual-terminal processing and the UTF-8 code page automatically on startup; legacy
  `conhost` windows may still render box glyphs poorly.
- The `bash` tool runs commands through PowerShell 7 (`pwsh`) if installed, otherwise
  Windows PowerShell, otherwise `cmd.exe`. The model is told which shell is active so it
  writes PowerShell syntax on Windows.
- Config and history live in `%USERPROFILE%\.gocode\`.

## First run

1. GoCode asks for your Anthropic API key (created at https://console.anthropic.com/settings/keys).
   The key is validated against the API and stored in `~/.gocode/config.json` with mode 0600.
   You can also set `ANTHROPIC_API_KEY` in your environment instead.
2. Pick a model from the numbered list, or press Enter to keep the default `claude-opus-5-5`.

## Commands

| Command | Description |
|---|---|
| `/help` | Show help |
| `/model [id]` | Interactive model picker, or set a model id directly |
| `/effort <low\|medium\|high\|xhigh\|max>` | Set reasoning effort |
| `/key` | Enter a new API key |
| `/auto` | Toggle auto-approval of tool calls |
| `/cwd [path]` | Show or change working directory |
| `/compact` | Summarize the conversation to free context |
| `/clear` | Start a new conversation |
| `/cost` | Token usage and estimated cost |
| `/status` | Session status |
| `/exit` | Quit |
| `!cmd` | Run a shell command directly |

Flags: `--model <id>`, `--auto`, `--version`, `--help`.

## Layout

| File | Purpose |
|---|---|
| `main.go` | Entry point, readline REPL, first-run onboarding |
| `agent.go` | Streaming request loop, tool execution, permissions, usage tracking, compaction |
| `tools.go` | Tool schemas and implementations |
| `commands.go` | Slash commands, model picker, API key prompt |
| `ui.go` | Orange/black theme, banner, spinner, markdown renderer |
| `config.go` | Config persistence and model catalog with pricing |

## Releasing (maintainers)

Push a version tag and GitHub Actions builds and publishes binaries for all platforms:

```sh
git tag v0.1.0
git push origin v0.1.0
```

## Notes

- Adaptive thinking is enabled on models that support it; `/effort` maps to `output_config.effort`.
- Cost estimates use Anthropic list prices from the model catalog in `config.go`.
- Designed for dark terminals with 24-bit color support.
