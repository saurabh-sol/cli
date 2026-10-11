# GoCode

A Claude Code style AI coding assistant for your terminal, written in Go.
You bring your own Anthropic API key and pick the Claude model you want.

## 1. Install

Copy one line into your terminal.

**Mac or Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/saurabh-sol/cli/main/install.sh | sh
```

**Windows** (open PowerShell)

```powershell
irm https://raw.githubusercontent.com/saurabh-sol/cli/main/install.ps1 | iex
```

That's it. Nothing else to install. If you prefer, download a file from the
[Releases page](https://github.com/saurabh-sol/cli/releases) instead.

## 2. Get an API key

1. Go to https://console.anthropic.com/settings/keys
2. Click "Create Key" and copy it (it starts with `sk-ant-`).

You pay Anthropic for what you use. There is no other cost.

## 3. Run it

Open a terminal in your project folder and type:

```
gocode
```

The first time it will:

1. Ask for your API key. Paste it and press Enter.
2. Show a list of models. Type a number, or press Enter for the default.

Then just type what you want, for example:

```
> explain this project
> add a login page
> fix the failing tests
```

GoCode reads and edits files and runs commands for you. Before it runs a
command or changes a file it asks you:

```
[y] yes   [a] always for Bash this session   [n] no
```

## Useful commands

| Type this | What it does |
|---|---|
| `/model` | Switch to another Claude model |
| `/key` | Change your API key |
| `/clear` | Start a new conversation |
| `/cost` | See how much this session has cost |
| `/auto` | Stop asking for permission (turn on or off) |
| `/help` | See all commands |
| `/exit` | Quit |
| `!ls` | Run any shell command yourself (put `!` in front) |

Press **Ctrl+C** to stop a reply that is taking too long.

## Problems?

| What you see | What to do |
|---|---|
| `Authentication failed (401)` | The key is not valid. It must come from https://console.anthropic.com/settings/keys and start with `sk-ant-api03-`. Keys from claude.ai do not work. Create a new key, then type `/key` and paste it. |
| `Model not found (404)` | Type `/model` and pick one from the list. |
| `gocode: command not found` | Close the terminal and open a new one. |
| Strange characters or no colors | On Windows use **Windows Terminal** (free in the Microsoft Store). |
| Windows warns about an unknown app | Click "More info", then "Run anyway". |

## Want more detail?

Everything else (building from source, config file, all tools, Windows notes,
how releases work) is in [docs/REFERENCE.md](docs/REFERENCE.md).

## License

MIT
