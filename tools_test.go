package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"**/*.go", "main.go", true},
		{"**/*.go", "a/b/c.go", true},
		{"*.go", "a/b.go", false},
		{"src/**/*.ts", "src/x/y/z.ts", true},
		{"src/**/*.ts", "lib/z.ts", false},
		{"*.{go,md}", "README.md", true},
		{"*.{go,md}", "x.py", false},
		{"file?.txt", "file1.txt", true},
	}
	for _, c := range cases {
		re, err := globToRegexp(c.glob)
		if err != nil {
			t.Fatal(err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("glob %q path %q: got %v want %v (re=%s)", c.glob, c.path, got, c.want, re)
		}
	}
}

func testAgent(t *testing.T) *Agent {
	dir := t.TempDir()
	a := &Agent{cfg: &Config{Model: defaultModel}, cwd: dir, tools: buildTools(), allowed: map[string]bool{}}
	return a
}

func TestFileTools(t *testing.T) {
	a := testAgent(t)
	ctx := context.Background()
	run := func(name string, in map[string]any) (string, error) {
		return a.tools[name].Run(ctx, a, in)
	}

	if _, err := run("write_file", map[string]any{"path": "sub/hello.go", "content": "package main\n\nfunc main() {}\n"}); err != nil {
		t.Fatal(err)
	}
	out, err := run("read_file", map[string]any{"path": "sub/hello.go"})
	if err != nil || !strings.Contains(out, "1\tpackage main") {
		t.Fatalf("read_file: %v %q", err, out)
	}
	if _, err := run("edit_file", map[string]any{"path": "sub/hello.go", "old_string": "func main() {}", "new_string": "func main() { println(1) }"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(a.cwd, "sub/hello.go"))
	if !strings.Contains(string(data), "println(1)") {
		t.Fatalf("edit not applied: %s", data)
	}
	if _, err := run("edit_file", map[string]any{"path": "sub/hello.go", "old_string": "nope", "new_string": "x"}); err == nil {
		t.Fatal("expected not-found error")
	}
	out, err = run("glob", map[string]any{"pattern": "**/*.go"})
	if err != nil || !strings.Contains(out, "sub/hello.go") {
		t.Fatalf("glob: %v %q", err, out)
	}
	out, err = run("grep", map[string]any{"pattern": "println", "include": "*.go"})
	if err != nil || !strings.Contains(out, "sub/hello.go:3:") {
		t.Fatalf("grep: %v %q", err, out)
	}
	out, err = run("list_dir", map[string]any{"path": "sub"})
	if err != nil || !strings.Contains(out, "hello.go") {
		t.Fatalf("list_dir: %v %q", err, out)
	}
	bashCmd := "echo hi && exit 3"
	if runtime.GOOS == "windows" {
		bashCmd = "Write-Output hi; exit 3"
	}
	out, err = run("bash", map[string]any{"command": bashCmd})
	if err == nil || !strings.Contains(out, "hi") || !strings.Contains(out, "exit code 3") {
		t.Fatalf("bash: %v %q", err, out)
	}
}

func TestToolParamsDeterministic(t *testing.T) {
	a := testAgent(t)
	p1 := toolParams(a.tools)
	p2 := toolParams(a.tools)
	if len(p1) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(p1))
	}
	for i := range p1 {
		if p1[i].OfTool.Name != p2[i].OfTool.Name {
			t.Fatal("tool order not deterministic")
		}
	}
}

func TestRendererDoesNotPanic(t *testing.T) {
	r := newRenderer()
	r.Write("# Title\nSome **bold** and `code`.\n- item one\n- item two\n```go\nfmt.Println(1)\n```\n1. first\ntrailing without newline")
	r.Flush()
}
