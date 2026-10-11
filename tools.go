package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const maxToolOutput = 40000 // chars sent back to the model

// toolDef pairs a schema with its executor and display metadata.
type toolDef struct {
	Param    anthropic.ToolParam
	ReadOnly bool
	// Header returns the short argument shown after the tool name.
	Header func(in map[string]any) string
	Run    func(ctx context.Context, a *Agent, in map[string]any) (string, error)
}

func strArg(in map[string]any, k string) string {
	if v, ok := in[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intArg(in map[string]any, k string, def int) int {
	if v, ok := in[k]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return def
}

func boolArg(in map[string]any, k string) bool {
	if v, ok := in[k]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (a *Agent) resolvePath(p string) string {
	if p == "" {
		return a.cwd
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(a.cwd, p)
}

func truncateOutput(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	return s[:maxToolOutput] + fmt.Sprintf("\n… [output truncated, %d more chars]", len(s)-maxToolOutput)
}

func schema(props map[string]any, required ...string) anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{Properties: props, Required: required}
}

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".venv": true, "__pycache__": true, "dist": true, "build": true, ".next": true, "target": true}

func buildTools() map[string]*toolDef {
	tools := map[string]*toolDef{}

	tools["bash"] = &toolDef{
		Param: anthropic.ToolParam{
			Name:        "bash",
			Description: anthropic.String("Run a shell command (on Windows: PowerShell) in the working directory and return stdout+stderr. Use for builds, tests, git, package managers and anything not covered by the file tools. Avoid using it to read or search files; prefer read_file, grep and glob."),
			InputSchema: schema(map[string]any{
				"command":     prop("string", "The shell command to run."),
				"description": prop("string", "Short (5-10 word) description of what the command does."),
				"timeout":     prop("integer", "Timeout in seconds (default 120, max 600)."),
			}, "command"),
		},
		Header: func(in map[string]any) string { return strArg(in, "command") },
		Run: func(ctx context.Context, a *Agent, in map[string]any) (string, error) {
			cmd := strArg(in, "command")
			if cmd == "" {
				return "", fmt.Errorf("command is required")
			}
			t := intArg(in, "timeout", 120)
			if t <= 0 || t > 600 {
				t = 120
			}
			return a.runShell(ctx, cmd, time.Duration(t)*time.Second)
		},
	}

	tools["read_file"] = &toolDef{
		ReadOnly: true,
		Param: anthropic.ToolParam{
			Name:        "read_file",
			Description: anthropic.String("Read a text file. Returns numbered lines. Use offset/limit for large files."),
			InputSchema: schema(map[string]any{
				"path":   prop("string", "Path to the file (absolute or relative to cwd)."),
				"offset": prop("integer", "1-based line to start from (default 1)."),
				"limit":  prop("integer", "Max lines to return (default 2000)."),
			}, "path"),
		},
		Header: func(in map[string]any) string { return strArg(in, "path") },
		Run: func(ctx context.Context, a *Agent, in map[string]any) (string, error) {
			p := a.resolvePath(strArg(in, "path"))
			f, err := os.Open(p)
			if err != nil {
				return "", err
			}
			defer f.Close()
			offset := intArg(in, "offset", 1)
			limit := intArg(in, "limit", 2000)
			if offset < 1 {
				offset = 1
			}
			var sb strings.Builder
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
			n := 0
			shown := 0
			for sc.Scan() {
				n++
				if n < offset {
					continue
				}
				if shown >= limit {
					sb.WriteString(fmt.Sprintf("… (more lines; use offset=%d)\n", n))
					break
				}
				sb.WriteString(fmt.Sprintf("%6d\t%s\n", n, sc.Text()))
				shown++
			}
			if err := sc.Err(); err != nil {
				return "", err
			}
			if shown == 0 {
				return "(empty file)", nil
			}
			return truncateOutput(sb.String()), nil
		},
	}

	tools["write_file"] = &toolDef{
		Param: anthropic.ToolParam{
			Name:        "write_file",
			Description: anthropic.String("Create or overwrite a file with the given content. Parent directories are created. Prefer edit_file for small changes to existing files."),
			InputSchema: schema(map[string]any{
				"path":    prop("string", "Path to write."),
				"content": prop("string", "Full file content."),
			}, "path", "content"),
		},
		Header: func(in map[string]any) string { return strArg(in, "path") },
		Run: func(ctx context.Context, a *Agent, in map[string]any) (string, error) {
			p := a.resolvePath(strArg(in, "path"))
			content := strArg(in, "content")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return "", err
			}
			_, existed := os.Stat(p)
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				return "", err
			}
			lines := strings.Count(content, "\n")
			if len(content) > 0 && !strings.HasSuffix(content, "\n") {
				lines++
			}
			verb := "Created"
			if existed == nil {
				verb = "Updated"
			}
			return fmt.Sprintf("%s %s (%d lines)", verb, p, lines), nil
		},
	}

	tools["edit_file"] = &toolDef{
		Param: anthropic.ToolParam{
			Name:        "edit_file",
			Description: anthropic.String("Replace an exact string in a file. old_string must match exactly (including whitespace) and be unique unless replace_all is true."),
			InputSchema: schema(map[string]any{
				"path":        prop("string", "Path to the file."),
				"old_string":  prop("string", "Exact text to find."),
				"new_string":  prop("string", "Replacement text."),
				"replace_all": prop("boolean", "Replace every occurrence (default false)."),
			}, "path", "old_string", "new_string"),
		},
		Header: func(in map[string]any) string { return strArg(in, "path") },
		Run: func(ctx context.Context, a *Agent, in map[string]any) (string, error) {
			p := a.resolvePath(strArg(in, "path"))
			oldS, newS := strArg(in, "old_string"), strArg(in, "new_string")
			if oldS == "" {
				return "", fmt.Errorf("old_string must not be empty")
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return "", err
			}
			s := string(data)
			count := strings.Count(s, oldS)
			if count == 0 {
				return "", fmt.Errorf("old_string not found in %s", p)
			}
			if count > 1 && !boolArg(in, "replace_all") {
				return "", fmt.Errorf("old_string matches %d times; provide more context or set replace_all", count)
			}
			var out string
			if boolArg(in, "replace_all") {
				out = strings.ReplaceAll(s, oldS, newS)
			} else {
				out = strings.Replace(s, oldS, newS, 1)
			}
			if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
				return "", err
			}
			printDiff(oldS, newS)
			return fmt.Sprintf("Edited %s (%d replacement(s))", p, count), nil
		},
	}

	tools["list_dir"] = &toolDef{
		ReadOnly: true,
		Param: anthropic.ToolParam{
			Name:        "list_dir",
			Description: anthropic.String("List files and directories at a path (non-recursive). Directories end with /."),
			InputSchema: schema(map[string]any{
				"path": prop("string", "Directory path (default cwd)."),
			}),
		},
		Header: func(in map[string]any) string { return strArg(in, "path") },
		Run: func(ctx context.Context, a *Agent, in map[string]any) (string, error) {
			p := a.resolvePath(strArg(in, "path"))
			entries, err := os.ReadDir(p)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			sb.WriteString(p + "\n")
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				sb.WriteString("  " + name + "\n")
			}
			return truncateOutput(sb.String()), nil
		},
	}

	tools["glob"] = &toolDef{
		ReadOnly: true,
		Param: anthropic.ToolParam{
			Name:        "glob",
			Description: anthropic.String("Find files by glob pattern, e.g. '**/*.go' or 'src/**/*.ts'. Returns matching paths sorted by modification time (newest first)."),
			InputSchema: schema(map[string]any{
				"pattern": prop("string", "Glob pattern. ** matches any directory depth."),
				"path":    prop("string", "Directory to search (default cwd)."),
			}, "pattern"),
		},
		Header: func(in map[string]any) string { return strArg(in, "pattern") },
		Run: func(ctx context.Context, a *Agent, in map[string]any) (string, error) {
			root := a.resolvePath(strArg(in, "path"))
			pat := strArg(in, "pattern")
			re, err := globToRegexp(pat)
			if err != nil {
				return "", err
			}
			type hit struct {
				path string
				mod  time.Time
			}
			var hits []hit
			_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					if p != root && skipDirs[d.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				rel, _ := filepath.Rel(root, p)
				rel = filepath.ToSlash(rel)
				if re.MatchString(rel) {
					info, _ := d.Info()
					var mt time.Time
					if info != nil {
						mt = info.ModTime()
					}
					hits = append(hits, hit{rel, mt})
				}
				if len(hits) >= 2000 {
					return filepath.SkipAll
				}
				return nil
			})
			sort.Slice(hits, func(i, j int) bool { return hits[i].mod.After(hits[j].mod) })
			if len(hits) == 0 {
				return "No files matched.", nil
			}
			var sb strings.Builder
			for _, h := range hits {
				sb.WriteString(h.path + "\n")
			}
			return truncateOutput(sb.String()), nil
		},
	}

	tools["grep"] = &toolDef{
		ReadOnly: true,
		Param: anthropic.ToolParam{
			Name:        "grep",
			Description: anthropic.String("Search file contents with a regular expression. Returns 'path:line: text' matches."),
			InputSchema: schema(map[string]any{
				"pattern":     prop("string", "Regular expression (Go RE2 syntax)."),
				"path":        prop("string", "File or directory to search (default cwd)."),
				"include":     prop("string", "Only search files matching this glob, e.g. '*.go'."),
				"ignore_case": prop("boolean", "Case-insensitive search."),
			}, "pattern"),
		},
		Header: func(in map[string]any) string {
			s := strArg(in, "pattern")
			if inc := strArg(in, "include"); inc != "" {
				s += "  in " + inc
			}
			return s
		},
		Run: func(ctx context.Context, a *Agent, in map[string]any) (string, error) {
			pat := strArg(in, "pattern")
			if boolArg(in, "ignore_case") {
				pat = "(?i)" + pat
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return "", err
			}
			var incRe *regexp.Regexp
			if inc := strArg(in, "include"); inc != "" {
				incRe, err = globToRegexp(inc)
				if err != nil {
					return "", err
				}
			}
			root := a.resolvePath(strArg(in, "path"))
			var sb strings.Builder
			matches := 0
			_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					if p != root && skipDirs[d.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				if incRe != nil && !incRe.MatchString(d.Name()) && !incRe.MatchString(filepath.ToSlash(p)) {
					return nil
				}
				info, err := d.Info()
				if err != nil || info.Size() > 5*1024*1024 {
					return nil
				}
				f, err := os.Open(p)
				if err != nil {
					return nil
				}
				defer f.Close()
				sc := bufio.NewScanner(f)
				sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
				ln := 0
				rel, _ := filepath.Rel(a.cwd, p)
				rel = filepath.ToSlash(rel)
				for sc.Scan() {
					ln++
					line := sc.Text()
					if ln == 1 && strings.ContainsRune(line, 0) {
						return nil // binary
					}
					if re.MatchString(line) {
						if len(line) > 300 {
							line = line[:300] + "…"
						}
						sb.WriteString(fmt.Sprintf("%s:%d: %s\n", rel, ln, line))
						matches++
						if matches >= 500 {
							return filepath.SkipAll
						}
					}
				}
				return nil
			})
			if matches == 0 {
				return "No matches.", nil
			}
			return truncateOutput(sb.String()), nil
		},
	}

	return tools
}

// globToRegexp converts a glob with ** support into a regexp anchored on the whole path.
func globToRegexp(glob string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	runes := []rune(glob)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch c {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				i++
				if i+1 < len(runes) && runes[i+1] == '/' {
					i++
					sb.WriteString("(?:.*/)?")
				} else {
					sb.WriteString(".*")
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		case '{':
			j := strings.IndexRune(string(runes[i:]), '}')
			if j > 0 {
				alts := strings.Split(string(runes[i+1:i+j]), ",")
				for k := range alts {
					alts[k] = regexp.QuoteMeta(alts[k])
				}
				sb.WriteString("(?:" + strings.Join(alts, "|") + ")")
				i += j
			} else {
				sb.WriteString(regexp.QuoteMeta(string(c)))
			}
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}

func (a *Agent) runShell(ctx context.Context, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	base := shellCommand(command)
	cmd := exec.CommandContext(ctx, base.Path, base.Args[1:]...)
	cmd.Dir = a.cwd
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	s := string(out)
	if ctx.Err() == context.DeadlineExceeded {
		return truncateOutput(s), fmt.Errorf("command timed out after %s", timeout)
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return truncateOutput(s) + fmt.Sprintf("\n[exit code %d]", ee.ExitCode()), fmt.Errorf("exit code %d", ee.ExitCode())
		}
		return truncateOutput(s), err
	}
	return truncateOutput(s), nil
}

func toolParams(tools map[string]*toolDef) []anthropic.ToolUnionParam {
	names := make([]string, 0, len(tools))
	for n := range tools {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic order keeps the prompt cache stable
	out := make([]anthropic.ToolUnionParam, 0, len(names))
	for _, n := range names {
		p := tools[n].Param
		out = append(out, anthropic.ToolUnionParam{OfTool: &p})
	}
	return out
}

func parseToolInput(raw string) (map[string]any, error) {
	in := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return in, nil
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, fmt.Errorf("invalid tool input JSON: %w", err)
	}
	return in, nil
}
