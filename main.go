package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
)

func main() {
	vtOK = enableVT()
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(appName, appVersion)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Printf("%s %s - a Claude Code style terminal agent in Go\n\nUsage: gocode [--model <id>] [--auto]\n\n  --model <id>  use this model for the session\n  --auto        auto-approve tool calls for the session\n  --version     print version\n", appName, appVersion)
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not load config:", err)
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	// CLI flags (minimal, so no extra dependency).
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--model", "-m":
			if i+1 < len(os.Args) {
				cfg.Model = os.Args[i+1]
				i++
			}
		case "--auto", "--dangerously-skip-permissions":
			cfg.AutoApprove = true
		}
	}

	completer := readline.NewPrefixCompleter(
		readline.PcItem("/help"), readline.PcItem("/model"), readline.PcItem("/effort",
			readline.PcItem("low"), readline.PcItem("medium"), readline.PcItem("high"), readline.PcItem("xhigh"), readline.PcItem("max")),
		readline.PcItem("/key"), readline.PcItem("/auto"), readline.PcItem("/cwd"), readline.PcItem("/compact"),
		readline.PcItem("/clear"), readline.PcItem("/cost"), readline.PcItem("/status"), readline.PcItem("/exit"),
	)
	rlCfg := &readline.Config{
		Prompt:            promptStyle(cOrange+cBold, "> "),
		HistoryFile:       historyPath(),
		HistoryLimit:      2000,
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "",
		HistorySearchFold: true,
	}
	if vtOK {
		// The console understands ANSI natively, so bypass readline's Windows
		// color emulator (it panics on 24-bit color codes). No-op on Unix.
		rlCfg.Stdout = os.Stdout
		rlCfg.Stderr = os.Stderr
	}
	rl, err := readline.NewEx(rlCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to init terminal:", err)
		os.Exit(1)
	}
	defer rl.Close()

	agent := newAgent(cfg, rl)

	clearScreen()
	printBanner(cfg, agent.cwd)

	// First-run onboarding: API key, then model choice.
	firstRun := cfg.APIKey == ""
	if firstRun {
		if err := agent.promptAPIKey(false); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, readline.ErrInterrupt) {
				fmt.Println(dim("  Goodbye."))
				return
			}
			printError(err.Error())
			os.Exit(1)
		}
		agent.pickModel(true)
		fmt.Println()
		printInfo("You're all set. Ask me anything about your code.")
		fmt.Println()
	}

	lastCtrlC := time.Time{}
	for {
		line, err := rl.Readline()
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) {
				if time.Since(lastCtrlC) < 2*time.Second {
					fmt.Println(dim("  Goodbye."))
					return
				}
				lastCtrlC = time.Now()
				fmt.Println(dim("  (Press Ctrl+C again to exit, or type /exit)"))
				continue
			}
			if errors.Is(err, io.EOF) {
				fmt.Println(dim("  Goodbye."))
				return
			}
			printError(err.Error())
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Direct shell command.
		if strings.HasPrefix(line, "!") {
			cmd := strings.TrimSpace(line[1:])
			if cmd == "" {
				continue
			}
			printToolHeader("Bash", cmd)
			out, err := agent.runShell(nil2ctx(), cmd, 120*time.Second)
			printToolResult(out, err != nil, 40)
			fmt.Println()
			continue
		}

		handled, quit := agent.handleCommand(line)
		if quit {
			fmt.Println(dim("  Goodbye."))
			return
		}
		if handled {
			continue
		}

		fmt.Println()
		if err := agent.Run(line); err != nil {
			printError(apiErrorMessage(err))
		}
		fmt.Println()
	}
}

// nil2ctx returns a background context (kept separate for readability at call sites).
func nil2ctx() context.Context { return context.Background() }
