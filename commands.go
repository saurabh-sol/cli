package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// handleCommand processes slash commands. Returns (handled, quit).
func (a *Agent) handleCommand(line string) (bool, bool) {
	if !strings.HasPrefix(line, "/") {
		return false, false
	}
	fields := strings.Fields(line)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "/exit", "/quit", "/q":
		return true, true

	case "/help", "/?":
		printHelp()

	case "/clear", "/new":
		a.messages = nil
		clearScreen()
		printBanner(a.cfg, a.cwd)
		printOK("Conversation cleared.")

	case "/model":
		if len(args) > 0 {
			a.setModel(args[0])
		} else {
			a.pickModel(false)
		}

	case "/effort":
		if len(args) == 0 {
			printInfo("Current effort: " + a.cfg.Effort + ". Options: " + strings.Join(validEfforts, ", "))
			break
		}
		e := strings.ToLower(args[0])
		if !isValidEffort(e) {
			printError("Invalid effort. Options: " + strings.Join(validEfforts, ", "))
			break
		}
		a.cfg.Effort = e
		_ = saveConfig(a.cfg)
		printOK("Effort set to " + e)

	case "/cost":
		a.printCost()

	case "/key":
		if err := a.promptAPIKey(true); err != nil {
			printError(err.Error())
		}

	case "/cwd", "/cd":
		if len(args) == 0 {
			printInfo("cwd: " + a.cwd)
			break
		}
		p := a.resolvePath(strings.Join(args, " "))
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			printError("Not a directory: " + p)
			break
		}
		a.cwd = p
		_ = os.Chdir(p)
		printOK("cwd: " + p)

	case "/auto":
		a.cfg.AutoApprove = !a.cfg.AutoApprove
		_ = saveConfig(a.cfg)
		if a.cfg.AutoApprove {
			printOK("Auto-approve ON: tools run without asking. Use /auto again to turn off.")
		} else {
			printOK("Auto-approve OFF: you will be asked before commands and file writes.")
		}

	case "/compact":
		if err := a.compact(context.Background()); err != nil {
			printError(apiErrorMessage(err))
		} else {
			printOK("Conversation compacted.")
		}

	case "/status":
		a.printStatus()

	default:
		printError("Unknown command: " + cmd + ". Type /help.")
	}
	return true, false
}

func printHelp() {
	rows := [][2]string{
		{"/help", "Show this help"},
		{"/model [id]", "Choose a model (interactive picker, or pass an id)"},
		{"/effort <lvl>", "Set effort: low, medium, high, xhigh, max"},
		{"/key", "Enter a new Anthropic API key"},
		{"/auto", "Toggle auto-approval of commands and file writes"},
		{"/cwd [path]", "Show or change the working directory"},
		{"/compact", "Summarize the conversation to free up context"},
		{"/clear", "Start a fresh conversation"},
		{"/cost", "Show token usage and estimated cost"},
		{"/status", "Show session status"},
		{"/exit", "Quit"},
		{"!<cmd>", "Run a shell command directly"},
	}
	fmt.Println()
	fmt.Println(cOrange + cBold + "  Commands" + cReset)
	for _, r := range rows {
		fmt.Printf("  %s%-16s%s %s%s%s\n", cAmber, r[0], cReset, cGray, r[1], cReset)
	}
	fmt.Println()
	fmt.Println(cGray + "  Keyboard: Ctrl+C interrupts a response, Ctrl+D or /exit quits, ↑/↓ browse history." + cReset)
	fmt.Println()
}

func (a *Agent) printCost() {
	u := a.usage
	fmt.Println()
	fmt.Println(cOrange + cBold + "  Session usage" + cReset)
	fmt.Printf("  %sModel:%s          %s\n", cGray, cReset, a.cfg.Model)
	fmt.Printf("  %sTurns:%s          %d\n", cGray, cReset, u.Turns)
	fmt.Printf("  %sInput tokens:%s   %s\n", cGray, cReset, fmtInt(u.In))
	fmt.Printf("  %sOutput tokens:%s  %s\n", cGray, cReset, fmtInt(u.Out))
	fmt.Printf("  %sCache read:%s     %s\n", cGray, cReset, fmtInt(u.CacheRead))
	fmt.Printf("  %sCache write:%s    %s\n", cGray, cReset, fmtInt(u.CacheWrite))
	fmt.Printf("  %sEstimated cost:%s %s$%.4f%s\n", cGray, cReset, cAmber, u.Cost, cReset)
	fmt.Printf("  %sDuration:%s       %s\n", cGray, cReset, time.Since(a.start).Round(time.Second))
	fmt.Println()
}

func (a *Agent) printStatus() {
	m := lookupModel(a.cfg.Model)
	fmt.Println()
	fmt.Println(cOrange + cBold + "  Status" + cReset)
	fmt.Printf("  %sVersion:%s      %s %s\n", cGray, cReset, appName, appVersion)
	fmt.Printf("  %sModel:%s        %s (%s)\n", cGray, cReset, m.ID, m.Name)
	fmt.Printf("  %sEffort:%s       %s\n", cGray, cReset, a.cfg.Effort)
	fmt.Printf("  %sAuto-approve:%s %v\n", cGray, cReset, a.cfg.AutoApprove)
	fmt.Printf("  %scwd:%s          %s\n", cGray, cReset, a.cwd)
	fmt.Printf("  %sMessages:%s     %d\n", cGray, cReset, len(a.messages))
	fmt.Printf("  %sAPI key:%s      %s\n", cGray, cReset, maskKey(a.cfg.APIKey))
	fmt.Println()
}

func fmtInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

func maskKey(k string) string {
	if len(k) < 12 {
		return "(not set)"
	}
	return k[:10] + "…" + k[len(k)-4:]
}

// ---------------------------------------------------------------------------
// Model selection
// ---------------------------------------------------------------------------

func (a *Agent) setModel(id string) {
	a.cfg.Model = id
	_ = saveConfig(a.cfg)
	m := lookupModel(id)
	printOK(fmt.Sprintf("Model set to %s%s%s (%s)", cAmber, id, cReset+cGreen, m.Name))
}

// fetchLiveModels asks the Models API for the account's available models.
func (a *Agent) fetchLiveModels(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	page, err := a.client.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(100)})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range page.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// pickModel shows an interactive numbered list. If firstRun, the current model is just a default.
func (a *Agent) pickModel(firstRun bool) {
	sp := startSpinner("Fetching models…")
	live, err := a.fetchLiveModels(context.Background())
	sp.Stop()

	// Build the list: known models first (in our order), then any extra live ones.
	var list []modelInfo
	liveSet := map[string]bool{}
	for _, id := range live {
		liveSet[id] = true
	}
	for _, m := range knownModels {
		if len(live) == 0 || liveSet[m.ID] {
			list = append(list, m)
		}
	}
	known := map[string]bool{}
	for _, m := range knownModels {
		known[m.ID] = true
	}
	var extra []string
	for _, id := range live {
		if !known[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		list = append(list, modelInfo{ID: id, Name: id, Desc: "Available on your account"})
	}
	if len(list) == 0 {
		list = knownModels
	}

	fmt.Println()
	fmt.Println(cOrange + cBold + "  Select a model" + cReset)
	if err != nil {
		fmt.Println(cGray + "  (could not fetch live list: " + apiErrorMessage(err) + ")" + cReset)
	}
	fmt.Println()
	for i, m := range list {
		marker := "  "
		if m.ID == a.cfg.Model {
			marker = cOrange + "❯ " + cReset
		}
		price := ""
		if m.InPrice > 0 {
			price = fmt.Sprintf("  $%g/$%g per MTok", m.InPrice, m.OutPrice)
		}
		fmt.Printf("  %s%s%2d.%s %s%-20s%s %s%s%s%s%s\n", marker, cOrange, i+1, cReset, cWhite+cBold, m.ID, cReset, cGray, m.Name, "  ·  "+m.Desc, price, cReset)
	}
	fmt.Println()
	ans, err := a.ask(cOrange + "  Model number or id (Enter to keep " + a.cfg.Model + "): " + cReset)
	if err != nil {
		return
	}
	ans = strings.TrimSpace(ans)
	if ans == "" {
		if firstRun {
			_ = saveConfig(a.cfg)
		}
		printInfo("Keeping " + a.cfg.Model)
		return
	}
	if n, err := strconv.Atoi(ans); err == nil {
		if n >= 1 && n <= len(list) {
			a.setModel(list[n-1].ID)
			return
		}
		printError("Number out of range.")
		return
	}
	a.setModel(ans)
}

// ---------------------------------------------------------------------------
// API key onboarding
// ---------------------------------------------------------------------------

func (a *Agent) promptAPIKey(force bool) error {
	if a.cfg.APIKey != "" && !force {
		return nil
	}
	fmt.Println()
	printBox([]string{
		cOrange + cBold + "Anthropic API key required" + cReset,
		"",
		cGray + "Create one at " + cReset + cAmber + "https://console.anthropic.com/settings/keys" + cReset,
		cGray + "It is stored locally in ~/.gocode/config.json (0600)." + cReset,
	})
	fmt.Println()
	for attempt := 0; attempt < 3; attempt++ {
		keyBytes, err := a.rl.ReadPassword(cOrange + "  API key (sk-ant-…): " + cReset)
		if err != nil {
			return err
		}
		key := strings.TrimSpace(string(keyBytes))
		if key == "" {
			printError("Key cannot be empty.")
			continue
		}
		sp := startSpinner("Validating key…")
		err = validateKey(key)
		sp.Stop()
		if err != nil {
			printError("Key rejected: " + apiErrorMessage(err))
			continue
		}
		a.cfg.APIKey = key
		a.resetClient()
		if err := saveConfig(a.cfg); err != nil {
			return err
		}
		printOK("API key saved.")
		return nil
	}
	return fmt.Errorf("no valid API key provided")
}

func validateKey(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(0))
	_, err := c.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(1)})
	return err
}
