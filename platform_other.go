//go:build !windows

package main

import (
	"os"
	"os/exec"
)

func enableVT() {}

func shellPath() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

func shellName() string { return shellPath() }

func shellCommand(command string) *exec.Cmd {
	return exec.Command(shellPath(), "-c", command)
}
