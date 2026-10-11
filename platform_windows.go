//go:build windows

package main

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// enableVT turns on ANSI escape processing and UTF-8 output in the Windows console
// so the orange/black theme and box-drawing glyphs render correctly. It reports
// whether native ANSI processing is active on stdout.
func enableVT() bool {
	ok := false
	for i, f := range []*os.File{os.Stdout, os.Stderr} {
		h := windows.Handle(f.Fd())
		var mode uint32
		if err := windows.GetConsoleMode(h, &mode); err != nil {
			continue
		}
		if err := windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err == nil && i == 0 {
			ok = true
		}
	}
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)
	return ok
}

// shellName reports the shell used by the bash tool, for the system prompt.
func shellName() string {
	if p, err := exec.LookPath("pwsh"); err == nil && p != "" {
		return "PowerShell 7 (pwsh)"
	}
	if p, err := exec.LookPath("powershell"); err == nil && p != "" {
		return "Windows PowerShell"
	}
	return "cmd.exe"
}

// shellCommand builds the exec.Cmd that runs a command string on Windows.
func shellCommand(command string) *exec.Cmd {
	if p, err := exec.LookPath("pwsh"); err == nil && p != "" {
		return exec.Command(p, "-NoProfile", "-NonInteractive", "-Command", command)
	}
	if p, err := exec.LookPath("powershell"); err == nil && p != "" {
		return exec.Command(p, "-NoProfile", "-NonInteractive", "-Command", command)
	}
	comspec := os.Getenv("COMSPEC")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	return exec.Command(comspec, "/C", command)
}
