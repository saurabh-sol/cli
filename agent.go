package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/chzyer/readline"
)

type usageTotals struct {
	In, Out, CacheRead, CacheWrite int64
	Cost                           float64
	Turns                          int
}

type Agent struct {
	client   anthropic.Client
	cfg      *Config
	cwd      string
	messages []anthropic.MessageParam
	tools    map[string]*toolDef
	allowed  map[string]bool // tools approved for the whole session
	usage    usageTotals
	rl       *readline.Instance
	start    time.Time
}

func newAgent(cfg *Config, rl *readline.Instance) *Agent {
	cwd, _ := os.Getwd()
	a := &Agent{
		cfg:     cfg,
		cwd:     cwd,
		tools:   buildTools(),
		allowed: map[string]bool{},
		rl:      rl,
		start:   time.Now(),
	}
	a.resetClient()
	return a
}

func (a *Agent) resetClient() {
	a.client = anthropic.NewClient(
		option.WithAPIKey(a.cfg.APIKey),
		option.WithMaxRetries(2),
	)
}

// ---------------------------------------------------------------------------
// System prompt
// ---------------------------------------------------------------------------

func (a *Agent) gitInfo() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = a.cwd
	out, err := cmd.Output()
	if err != nil {
		return "not a git repository"
	}
	branch := strings.TrimSpace(string(out))
	st := exec.Command("git", "status", "--porcelain")
	st.Dir = a.cwd
	so, _ := st.Output()
	n := 0
	if len(so) > 0 {
		n = strings.Count(strings.TrimRight(string(so), "\n"), "\n") + 1
	}
	return fmt.Sprintf("git branch %s, %d changed file(s)", branch, n)
}

func (a *Agent) systemPrompt() string {
	return fmt.Sprintf(`You are %s, an interactive CLI coding agent running in the user's terminal. You help with software engineering tasks: understanding codebases, writing and editing code, running commands, debugging and explaining.

Environment:
- Working directory: %s
- Platform: %s/%s
- Shell used by the bash tool: %s
- %s
- Today's date: %s

Tools:
- Use read_file, grep, glob and list_dir to explore. Prefer them over bash for reading and searching.
- Use edit_file for targeted changes and write_file for new files or full rewrites. Always read a file before editing it.
- Use bash for builds, tests, git and other commands. Write commands for the shell named above (PowerShell syntax on Windows, POSIX sh elsewhere). Never run destructive commands (rm -rf, git reset --hard, force push) without the user explicitly asking.
- You may call several independent tools in one turn.
- Tool results that fail include an error message; adapt instead of retrying the same call verbatim.

Style:
- Output is rendered as simple markdown in a terminal. Keep answers concise and direct. Use short bullet lists for parallel items and fenced code blocks for code.
- Do not use emojis unless asked.
- Reference code as path:line when useful.
- When you finish a task, give a brief summary of what changed and anything the user should verify.
- If a request is ambiguous in a way that changes the work materially, ask a short clarifying question; otherwise make reasonable assumptions and proceed.`,
		appName, a.cwd, runtime.GOOS, runtime.GOARCH, shellName(), a.gitInfo(), time.Now().Format("2006-01-02"))
}

// ---------------------------------------------------------------------------
// Request construction
// ---------------------------------------------------------------------------

func (a *Agent) buildParams() anthropic.MessageNewParams {
	m := lookupModel(a.cfg.Model)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.cfg.Model),
		MaxTokens: 32000,
		System: []anthropic.TextBlockParam{{
			Text:         a.systemPrompt(),
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: a.messages,
		Tools:    toolParams(a.tools),
	}
	if m.Adaptive {
		adaptive := anthropic.ThinkingConfigAdaptiveParam{}
		params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
	}
	if m.Effort && isValidEffort(a.cfg.Effort) {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(a.cfg.Effort)}
	}
	return params
}

// ---------------------------------------------------------------------------
// Main agentic turn
// ---------------------------------------------------------------------------

// Run sends the user's input and loops over tool calls until the model stops.
func (a *Agent) Run(input string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.messages = append(a.messages, anthropic.NewUserMessage(anthropic.NewTextBlock(input)))

	for iter := 0; iter < 200; iter++ {
		msg, err := a.streamOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Drop the dangling user message / partial turn so history stays valid.
				a.rollbackToLastComplete()
				fmt.Println(dim("\n  Interrupted."))
				return nil
			}
			a.rollbackToLastComplete()
			return err
		}
		a.messages = append(a.messages, msg.ToParam())
		a.trackUsage(msg.Usage)

		switch msg.StopReason {
		case anthropic.StopReasonToolUse:
			results, err := a.executeTools(ctx, msg)
			if err != nil {
				if ctx.Err() != nil {
					a.rollbackToLastComplete()
					fmt.Println(dim("\n  Interrupted."))
					return nil
				}
				return err
			}
			a.messages = append(a.messages, anthropic.NewUserMessage(results...))
			continue
		case anthropic.StopReasonMaxTokens:
			printError("Response hit the max_tokens limit. Ask me to continue.")
		case anthropic.StopReasonRefusal:
			cat := msg.StopDetails.Category
			if cat == "" {
				cat = "unspecified"
			}
			printError(fmt.Sprintf("The model declined this request (category: %s). %s", cat, msg.StopDetails.Explanation))
		}
		return nil
	}
	return fmt.Errorf("too many tool iterations")
}

// rollbackToLastComplete trims history so it ends with a complete assistant turn
// (or is empty). Used after an interrupt or error mid-turn.
func (a *Agent) rollbackToLastComplete() {
	for len(a.messages) > 0 {
		last := a.messages[len(a.messages)-1]
		if last.Role == anthropic.MessageParamRoleAssistant && !hasToolUse(last) {
			return
		}
		a.messages = a.messages[:len(a.messages)-1]
	}
}

func hasToolUse(m anthropic.MessageParam) bool {
	for _, b := range m.Content {
		if b.OfToolUse != nil {
			return true
		}
	}
	return false
}

// streamOnce performs a single streaming request, rendering text as it arrives.
func (a *Agent) streamOnce(ctx context.Context) (*anthropic.Message, error) {
	params := a.buildParams()
	stream := a.client.Messages.NewStreaming(ctx, params)

	sp := startSpinner("")
	r := newRenderer()
	msg := anthropic.Message{}
	gotFirst := false
	curType := ""

	for stream.Next() {
		ev := stream.Current()
		if err := msg.Accumulate(ev); err != nil {
			sp.Stop()
			return nil, err
		}
		switch e := ev.AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			curType = e.ContentBlock.Type
			if curType == "text" || curType == "tool_use" {
				if !gotFirst {
					sp.Stop()
					gotFirst = true
				}
			}
		case anthropic.ContentBlockDeltaEvent:
			switch d := e.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if !gotFirst {
					sp.Stop()
					gotFirst = true
				}
				r.Write(d.Text)
			}
		case anthropic.ContentBlockStopEvent:
			if curType == "text" {
				r.Flush()
			}
			curType = ""
		}
	}
	sp.Stop()
	if err := stream.Err(); err != nil {
		r.Flush()
		return nil, err
	}
	r.Flush()
	return &msg, nil
}

// ---------------------------------------------------------------------------
// Tool execution + permissions
// ---------------------------------------------------------------------------

func (a *Agent) executeTools(ctx context.Context, msg *anthropic.Message) ([]anthropic.ContentBlockParamUnion, error) {
	var results []anthropic.ContentBlockParamUnion
	for _, block := range msg.Content {
		tu, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		def := a.tools[tu.Name]
		raw := tu.JSON.Input.Raw()
		in, perr := parseToolInput(raw)
		if def == nil {
			results = append(results, anthropic.NewToolResultBlock(tu.ID, "unknown tool: "+tu.Name, true))
			continue
		}
		header := ""
		if perr == nil && def.Header != nil {
			header = def.Header(in)
		}
		printToolHeader(displayName(tu.Name), header)
		if perr != nil {
			printToolResult(perr.Error(), true, 3)
			results = append(results, anthropic.NewToolResultBlock(tu.ID, perr.Error(), true))
			continue
		}

		if !def.ReadOnly && !a.cfg.AutoApprove && !a.allowed[tu.Name] {
			ok, err := a.askPermission(tu.Name, in)
			if err != nil {
				return nil, err
			}
			if !ok {
				printToolResult("User denied permission for this action.", true, 1)
				results = append(results, anthropic.NewToolResultBlock(tu.ID,
					"The user denied permission for this tool call. Do not retry it; ask the user how to proceed or choose another approach.", true))
				continue
			}
		}

		out, err := def.Run(ctx, a, in)
		if err != nil {
			if out == "" {
				out = err.Error()
			} else if !strings.Contains(out, err.Error()) {
				out = out + "\nError: " + err.Error()
			}
			printToolResult(out, true, 8)
			results = append(results, anthropic.NewToolResultBlock(tu.ID, out, true))
			continue
		}
		if tu.Name == "edit_file" {
			printToolResult(out, false, 1)
		} else {
			printToolResult(out, false, 6)
		}
		results = append(results, anthropic.NewToolResultBlock(tu.ID, out, false))
	}
	return results, nil
}

func displayName(tool string) string {
	switch tool {
	case "bash":
		return "Bash"
	case "read_file":
		return "Read"
	case "write_file":
		return "Write"
	case "edit_file":
		return "Edit"
	case "list_dir":
		return "List"
	case "glob":
		return "Glob"
	case "grep":
		return "Grep"
	}
	return tool
}

// askPermission prompts the user to approve a side-effecting tool call.
func (a *Agent) askPermission(tool string, in map[string]any) (bool, error) {
	var detail string
	switch tool {
	case "bash":
		detail = strArg(in, "command")
		if d := strArg(in, "description"); d != "" {
			detail += "\n" + cGray + "    " + d + cReset
		}
	case "write_file":
		detail = fmt.Sprintf("%s (%d bytes)", strArg(in, "path"), len(strArg(in, "content")))
	case "edit_file":
		detail = strArg(in, "path")
	default:
		detail = fmt.Sprint(in)
	}
	fmt.Println()
	fmt.Printf("  %s%s wants to run %s%s%s\n", cOrange, appName, cReset+cBold, displayName(tool), cReset)
	for _, l := range strings.Split(detail, "\n") {
		fmt.Printf("    %s%s%s\n", cWhite, l, cReset)
	}
	fmt.Printf("  %s[y]%s yes   %s[a]%s always for %s this session   %s[n]%s no\n",
		cOrange, cReset, cOrange, cReset, displayName(tool), cOrange, cReset)

	ans, err := a.ask(promptStyle(cOrange, "  › "))
	if err != nil {
		return false, err
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	switch ans {
	case "y", "yes", "":
		return true, nil
	case "a", "always":
		a.allowed[tool] = true
		return true, nil
	}
	return false, nil
}

// ask reads one line from the user with a temporary prompt.
func (a *Agent) ask(prompt string) (string, error) {
	old := a.rl.Config.Prompt
	a.rl.SetPrompt(prompt)
	defer a.rl.SetPrompt(old)
	line, err := a.rl.Readline()
	if err != nil {
		if errors.Is(err, readline.ErrInterrupt) {
			return "n", nil
		}
		return "", err
	}
	return line, nil
}

// ---------------------------------------------------------------------------
// Usage / cost
// ---------------------------------------------------------------------------

func (a *Agent) trackUsage(u anthropic.Usage) {
	m := lookupModel(a.cfg.Model)
	a.usage.Turns++
	a.usage.In += u.InputTokens
	a.usage.Out += u.OutputTokens
	a.usage.CacheRead += u.CacheReadInputTokens
	a.usage.CacheWrite += u.CacheCreationInputTokens
	a.usage.Cost += float64(u.InputTokens)/1e6*m.InPrice +
		float64(u.OutputTokens)/1e6*m.OutPrice +
		float64(u.CacheReadInputTokens)/1e6*m.CacheRead +
		float64(u.CacheCreationInputTokens)/1e6*m.InPrice*1.25
}

// ---------------------------------------------------------------------------
// Compaction (client-side summary)
// ---------------------------------------------------------------------------

func (a *Agent) compact(ctx context.Context) error {
	if len(a.messages) == 0 {
		return fmt.Errorf("nothing to compact")
	}
	req := a.messages
	req = append(req, anthropic.NewUserMessage(anthropic.NewTextBlock(
		"Summarize this conversation so far for your own future reference. Include: the user's goals, decisions made, files touched with key details, current state of the work, and any open items. Be thorough but compact. Output only the summary.")))
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.cfg.Model),
		MaxTokens: 8000,
		Messages:  req,
	}
	if lookupModel(a.cfg.Model).Adaptive {
		adaptive := anthropic.ThinkingConfigAdaptiveParam{}
		params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
	}
	sp := startSpinner("Compacting…")
	resp, err := a.client.Messages.New(ctx, params)
	sp.Stop()
	if err != nil {
		return err
	}
	a.trackUsage(resp.Usage)
	var sb strings.Builder
	for _, b := range resp.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	a.messages = []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Context summary of our earlier conversation (continue from here):\n\n" + sb.String())),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Understood. I have the context and will continue from here.")),
	}
	return nil
}

// apiErrorMessage formats SDK errors for display.
func apiErrorMessage(err error) string {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case 401:
			return "Authentication failed (401). Check your API key with /key."
		case 403:
			return "Permission denied (403): " + apierr.Error()
		case 404:
			return "Model not found (404). Use /model to pick a valid model."
		case 429:
			return "Rate limited (429). Wait a moment and try again."
		case 529:
			return "API overloaded (529). Try again shortly."
		}
		return fmt.Sprintf("API error %d: %s", apierr.StatusCode, apierr.Error())
	}
	return err.Error()
}
