package main

import (
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
)

// ---------------------------------------------------------------------------
// Theme: orange on black
// ---------------------------------------------------------------------------

const (
	cReset     = "\x1b[0m"
	cBold      = "\x1b[1m"
	cDim       = "\x1b[2m"
	cItalic    = "\x1b[3m"
	cUnderline = "\x1b[4m"

	cOrange    = "\x1b[38;2;255;140;0m"   // primary accent
	cOrangeDim = "\x1b[38;2;205;120;40m"  // secondary accent
	cAmber     = "\x1b[38;2;255;190;90m"  // highlights / inline code
	cWhite     = "\x1b[38;2;235;235;235m" // body text
	cGray      = "\x1b[38;2;125;125;125m" // muted
	cRed       = "\x1b[38;2;255;95;95m"
	cGreen     = "\x1b[38;2;120;205;120m"

	cBlackBg = "\x1b[48;2;0;0;0m"
	cCodeBg  = "\x1b[48;2;24;24;24m"
	cDiffAdd = "\x1b[48;2;20;60;20m"
	cDiffDel = "\x1b[48;2;70;20;20m"
)

// vtOK is false only when the console cannot process ANSI escapes natively
// (very old Windows). Prompts handed to readline must then stay uncolored,
// because readline's Windows fallback emulator understands only 8 colors.
var vtOK = true

// promptStyle colors text that will be passed to readline as a prompt.
func promptStyle(style, s string) string {
	if !vtOK {
		return s
	}
	return style + s + cReset
}

func orange(s string) string { return cOrange + s + cReset }
func dim(s string) string    { return cGray + s + cReset }
func bold(s string) string   { return cBold + s + cReset }

func termWidth() int {
	w := readline.GetScreenWidth()
	if w <= 0 {
		return 80
	}
	return w
}

// Clear the screen and paint a black background.
func clearScreen() {
	fmt.Print(cBlackBg + "\x1b[2J\x1b[H")
}

// ---------------------------------------------------------------------------
// Banner
// ---------------------------------------------------------------------------

const appName = "GoCode"

var appVersion = "0.1.1" // overridden at release time via -ldflags "-X main.appVersion=..."

func printBanner(cfg *Config, cwd string) {
	lines := []string{
		cOrange + cBold + "✻ Welcome to " + appName + "!" + cReset,
		"",
		cGray + "  /help for help, /model to switch model, /exit to quit" + cReset,
		"",
		cGray + "  cwd:    " + cReset + cWhite + cwd + cReset,
		cGray + "  model:  " + cReset + cAmber + cfg.Model + cReset + cGray + "  (effort: " + cfg.Effort + ")" + cReset,
	}
	printBox(lines)
	fmt.Println()
	fmt.Println(cGray + "  Tips: type a message to chat, " + cAmber + "!cmd" + cGray + " to run a shell command, Ctrl+C to interrupt." + cReset)
	fmt.Println()
}

// printBox draws an orange rounded border around lines (which may contain ANSI).
func printBox(lines []string) {
	maxw := 0
	for _, l := range lines {
		if w := visibleLen(l); w > maxw {
			maxw = w
		}
	}
	tw := termWidth() - 4
	if maxw > tw {
		maxw = tw
	}
	bar := strings.Repeat("─", maxw+2)
	fmt.Println(cOrange + "╭" + bar + "╮" + cReset)
	for _, l := range lines {
		pad := maxw - visibleLen(l)
		if pad < 0 {
			pad = 0
		}
		fmt.Println(cOrange + "│ " + cReset + l + strings.Repeat(" ", pad) + cOrange + " │" + cReset)
	}
	fmt.Println(cOrange + "╰" + bar + "╯" + cReset)
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func visibleLen(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}

// ---------------------------------------------------------------------------
// Spinner
// ---------------------------------------------------------------------------

var spinnerFrames = []string{"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"}

var spinnerVerbs = []string{
	"Thinking", "Pondering", "Brewing", "Cogitating", "Noodling", "Mulling",
	"Percolating", "Scheming", "Synthesizing", "Computing", "Reasoning",
	"Crafting", "Deliberating", "Musing", "Working",
}

type spinner struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
}

var spinnerMu sync.Mutex // serialize stdout writes between spinner and renderer

func startSpinner(label string) *spinner {
	if label == "" {
		label = spinnerVerbs[rand.Intn(len(spinnerVerbs))] + "…"
	}
	s := &spinner{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		i := 0
		start := time.Now()
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				spinnerMu.Lock()
				fmt.Print("\r\x1b[2K")
				spinnerMu.Unlock()
				return
			case <-t.C:
				el := int(time.Since(start).Seconds())
				spinnerMu.Lock()
				fmt.Printf("\r\x1b[2K%s%s %s%s %s(%ds · ctrl+c to interrupt)%s",
					cOrange, spinnerFrames[i%len(spinnerFrames)], label, cReset, cGray, el, cReset)
				spinnerMu.Unlock()
				i++
			}
		}
	}()
	return s
}

func (s *spinner) Stop() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.stop)
		<-s.done
	})
}

// ---------------------------------------------------------------------------
// Streaming markdown-lite renderer
// ---------------------------------------------------------------------------

type mdRenderer struct {
	buf      strings.Builder
	inCode   bool
	started  bool
	anyText  bool
	indent   string
	prefixed bool
}

func newRenderer() *mdRenderer {
	return &mdRenderer{indent: "  "}
}

// Write accepts streamed text and renders completed lines.
func (r *mdRenderer) Write(s string) {
	for _, ch := range s {
		if ch == '\n' {
			r.emitLine(r.buf.String())
			r.buf.Reset()
		} else {
			r.buf.WriteRune(ch)
		}
	}
}

func (r *mdRenderer) Flush() {
	if r.buf.Len() > 0 {
		r.emitLine(r.buf.String())
		r.buf.Reset()
	}
	if r.anyText {
		fmt.Println()
	}
	r.started = false
	r.prefixed = false
}

func (r *mdRenderer) prefix() string {
	if !r.prefixed {
		r.prefixed = true
		return cOrange + "⏺ " + cReset
	}
	return r.indent
}

var (
	reBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reCode   = regexp.MustCompile("`([^`]+)`")
	reItalic = regexp.MustCompile(`(^|[^*\w])\*([^*\n]+)\*`)
	reHeader = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reBullet = regexp.MustCompile(`^(\s*)[-*]\s+(.*)$`)
	reNum    = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
)

func inlineFormat(s string) string {
	s = reCode.ReplaceAllString(s, cAmber+"$1"+cReset+cWhite)
	s = reBold.ReplaceAllString(s, cBold+"$1"+cReset+cWhite)
	s = reItalic.ReplaceAllString(s, "$1"+cItalic+"$2"+cReset+cWhite)
	return cWhite + s + cReset
}

func (r *mdRenderer) emitLine(line string) {
	r.anyText = true
	spinnerMu.Lock()
	defer spinnerMu.Unlock()

	trim := strings.TrimSpace(line)
	if strings.HasPrefix(trim, "```") {
		lang := strings.TrimSpace(strings.TrimPrefix(trim, "```"))
		r.inCode = !r.inCode
		if r.inCode {
			fmt.Println(r.prefix() + cGray + "┌─ " + lang + cReset)
		} else {
			fmt.Println(r.prefix() + cGray + "└─" + cReset)
		}
		return
	}
	if r.inCode {
		fmt.Println(r.prefix() + cGray + "│ " + cReset + cCodeBg + cAmber + line + cReset)
		return
	}
	if m := reHeader.FindStringSubmatch(line); m != nil {
		fmt.Println(r.prefix() + cOrange + cBold + m[2] + cReset)
		return
	}
	if m := reBullet.FindStringSubmatch(line); m != nil {
		fmt.Println(r.prefix() + m[1] + cOrange + "• " + cReset + inlineFormat(m[2]))
		return
	}
	if m := reNum.FindStringSubmatch(line); m != nil {
		fmt.Println(r.prefix() + m[1] + cOrange + m[2] + "." + cReset + " " + inlineFormat(m[3]))
		return
	}
	if trim == "" {
		if r.prefixed {
			fmt.Println()
		}
		return
	}
	fmt.Println(r.prefix() + inlineFormat(line))
}

// ---------------------------------------------------------------------------
// Tool call display helpers (Claude Code style ⏺ / ⎿)
// ---------------------------------------------------------------------------

func printToolHeader(name, arg string) {
	spinnerMu.Lock()
	defer spinnerMu.Unlock()
	maxArg := termWidth() - len(name) - 8
	if maxArg < 20 {
		maxArg = 20
	}
	arg = strings.ReplaceAll(arg, "\n", " ")
	if len([]rune(arg)) > maxArg {
		arg = string([]rune(arg)[:maxArg-1]) + "…"
	}
	fmt.Printf("%s⏺ %s%s%s(%s%s%s)%s\n", cOrange, cReset+cBold+cWhite, name, cReset+cGray, cReset+cWhite, arg, cGray, cReset)
}

// printToolResult prints up to maxLines of output under a ⎿ marker.
func printToolResult(out string, isErr bool, maxLines int) {
	spinnerMu.Lock()
	defer spinnerMu.Unlock()
	col := cGray
	if isErr {
		col = cRed
	}
	out = strings.TrimRight(out, "\n")
	if out == "" {
		out = "(no output)"
	}
	lines := strings.Split(out, "\n")
	w := termWidth() - 8
	if w < 20 {
		w = 20
	}
	shown := lines
	extra := 0
	if len(lines) > maxLines {
		shown = lines[:maxLines]
		extra = len(lines) - maxLines
	}
	for i, l := range shown {
		if len([]rune(l)) > w {
			l = string([]rune(l)[:w-1]) + "…"
		}
		if i == 0 {
			fmt.Printf("  %s⎿  %s%s\n", cGray, col+l, cReset)
		} else {
			fmt.Printf("     %s%s%s\n", col, l, cReset)
		}
	}
	if extra > 0 {
		fmt.Printf("     %s… +%d lines%s\n", cGray, extra, cReset)
	}
}

func printInfo(s string) {
	fmt.Println(cGray + "  " + s + cReset)
}

func printError(s string) {
	fmt.Println(cRed + "  ✗ " + s + cReset)
}

func printOK(s string) {
	fmt.Println(cGreen + "  ✓ " + s + cReset)
}

func printDiff(oldS, newS string) {
	spinnerMu.Lock()
	defer spinnerMu.Unlock()
	const cap = 12
	show := func(lines []string, sign, bg, fg string) {
		for i, l := range lines {
			if i >= cap {
				fmt.Printf("     %s… +%d lines%s\n", cGray, len(lines)-cap, cReset)
				return
			}
			fmt.Printf("     %s%s%s %s%s\n", bg, fg, sign, l, cReset)
		}
	}
	show(strings.Split(strings.TrimRight(oldS, "\n"), "\n"), "-", cDiffDel, cRed)
	show(strings.Split(strings.TrimRight(newS, "\n"), "\n"), "+", cDiffAdd, cGreen)
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
