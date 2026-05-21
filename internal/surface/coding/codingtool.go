package coding

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/tools"
	"github.com/jabreeflor/conduit/internal/tools/websearch"
)

// maxSleepSeconds caps the sleep tool so the agent cannot block a session
// indefinitely. 60 s is generous enough for polling loops and CI waits.
const maxSleepSeconds = 60.0

// maxBashSeconds caps bash execution time to guard against runaway commands.
const maxBashSeconds = 120

// maxReadBytes caps the bytes returned by read_file / web_fetch to avoid
// flooding the context window with enormous files. 512 KiB ≈ ~128 k tokens.
const maxReadBytes = 512 * 1024

// maxGrepMatches caps results from grep_search so the context window stays
// manageable. The agent should narrow the search if this is hit.
const maxGrepMatches = 200

// maxGlobResults caps results from glob_search.
const maxGlobResults = 500

// LiveCodingTools returns the PRD §6.24.4 coding tool set with real runners,
// replacing the stubs in DefaultCodingTools. tool_search closes over reg so
// the REPL can inject the full registered-tool slice after building it.
//
// Pass nil for reg to disable tool_search (the tool will return an error
// explaining that no registry was provided).
func LiveCodingTools(wsCfg websearch.Config, reg []tools.Tool) []TieredTool {
	return LiveCodingToolsForSession(wsCfg, reg, nil)
}

// LiveCodingToolsForSession wires session-aware tools such as worktree_enter
// and worktree_exit in addition to the standard coding tool set.
func LiveCodingToolsForSession(wsCfg websearch.Config, reg []tools.Tool, session *Session) []TieredTool {
	return []TieredTool{
		{Tool: listDirTool(), Tier: contracts.CodingTierAlways},
		{Tool: readFileTool(), Tier: contracts.CodingTierAlways},
		{Tool: globSearchTool(), Tier: contracts.CodingTierAlways},
		{Tool: grepSearchTool(), Tier: contracts.CodingTierAlways},
		{Tool: webFetchTool(), Tier: contracts.CodingTierAlways},
		{Tool: websearch.New(wsCfg), Tier: contracts.CodingTierAlways},
		{Tool: toolSearchTool(reg), Tier: contracts.CodingTierAlways},
		{Tool: sleepTool(), Tier: contracts.CodingTierAlways},
		{Tool: writeFileTool(), Tier: contracts.CodingTierRequiresWrite},
		{Tool: editFileTool(), Tier: contracts.CodingTierRequiresWrite},
		{Tool: notebookEditTool(), Tier: contracts.CodingTierRequiresWrite},
		{Tool: bashTool(), Tier: contracts.CodingTierRequiresShell},
		{Tool: worktreeEnterTool(session), Tier: contracts.CodingTierRequiresShell},
		{Tool: worktreeExitTool(session), Tier: contracts.CodingTierRequiresShell},
	}
}

// ── worktree_enter / worktree_exit ───────────────────────────────────────

func worktreeEnterTool(session *Session) tools.Tool {
	return tools.Tool{
		Name:        "worktree_enter",
		Description: "Create and switch this coding session into a managed git worktree. Subsequent relative file and shell tools run from that worktree.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repository_root": map[string]any{
					"type":        "string",
					"description": "Git repository root. Defaults to the session repository or current working directory.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Worktree path. Defaults to ~/.conduit/worktrees/<session-id>.",
				},
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to create for the worktree. Defaults to conduit/<session-id>.",
				},
				"base_branch": map[string]any{
					"type":        "string",
					"description": "Base branch or commit. Defaults to the session branch.",
				},
			},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (tools.Result, error) {
			if session == nil {
				return tools.Result{IsError: true, Text: "worktree_enter: no coding session attached"}, nil
			}
			var p struct {
				RepositoryRoot string `json:"repository_root"`
				Path           string `json:"path"`
				Branch         string `json:"branch"`
				BaseBranch     string `json:"base_branch"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &p); err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("worktree_enter: bad args: %v", err)}, nil
				}
			}
			state, err := session.WorktreeEnter(ctx, WorktreeEnterOptions{
				RepositoryRoot: p.RepositoryRoot,
				Path:           p.Path,
				Branch:         p.Branch,
				BaseBranch:     p.BaseBranch,
			})
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("worktree_enter: %v", err)}, nil
			}
			return tools.Result{Text: fmt.Sprintf("entered worktree %s on %s", state.WorktreePath, state.WorktreeBranch)}, nil
		},
	}
}

func worktreeExitTool(session *Session) tools.Tool {
	return tools.Tool{
		Name:        "worktree_exit",
		Description: "Exit the active managed git worktree and optionally keep it on disk.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keep": map[string]any{
					"type":        "boolean",
					"description": "Keep the worktree instead of removing it. Defaults to false.",
				},
			},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (tools.Result, error) {
			if session == nil {
				return tools.Result{IsError: true, Text: "worktree_exit: no coding session attached"}, nil
			}
			var p struct {
				Keep bool `json:"keep"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &p); err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("worktree_exit: bad args: %v", err)}, nil
				}
			}
			state, err := session.WorktreeExit(ctx, WorktreeExitOptions{Keep: p.Keep})
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("worktree_exit: %v", err)}, nil
			}
			return tools.Result{Text: fmt.Sprintf("exited worktree; cwd=%s", state.CWD)}, nil
		},
	}
}

// ── list_dir ──────────────────────────────────────────────────────────────

func listDirTool() tools.Tool {
	return tools.Tool{
		Name:        "list_dir",
		Description: "List files and directories at path. Returns names, types (file/dir), and sizes.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path to list. Defaults to the current working directory.",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "If true, list recursively. Defaults to false.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Path      string `json:"path"`
				Recursive bool   `json:"recursive"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &p); err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("list_dir: bad args: %v", err)}, nil
				}
			}
			dir := p.Path
			if dir == "" {
				var err error
				dir, err = os.Getwd()
				if err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("list_dir: getwd: %v", err)}, nil
				}
			}

			var sb strings.Builder
			if p.Recursive {
				if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil
					}
					rel, _ := filepath.Rel(dir, path)
					if rel == "." {
						return nil
					}
					kind := "file"
					size := ""
					if info.IsDir() {
						kind = "dir"
						rel += "/"
					} else {
						size = fmt.Sprintf(" (%s)", humanSize(info.Size()))
					}
					fmt.Fprintf(&sb, "%s [%s]%s\n", rel, kind, size)
					return nil
				}); err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("list_dir: walk: %v", err)}, nil
				}
			} else {
				entries, err := os.ReadDir(dir)
				if err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("list_dir: %v", err)}, nil
				}
				for _, e := range entries {
					info, _ := e.Info()
					kind := "file"
					size := ""
					name := e.Name()
					if e.IsDir() {
						kind = "dir"
						name += "/"
					} else if info != nil {
						size = fmt.Sprintf(" (%s)", humanSize(info.Size()))
					}
					fmt.Fprintf(&sb, "%s [%s]%s\n", name, kind, size)
				}
			}
			return tools.Result{Text: sb.String()}, nil
		},
	}
}

// ── read_file ─────────────────────────────────────────────────────────────

func readFileTool() tools.Tool {
	return tools.Tool{
		Name:        "read_file",
		Description: "Read file contents, optionally bounded to a line range. Returns content with 1-based line numbers prefixed.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"path"},
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative file path.",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"description": "First line to return (1-based, inclusive). Defaults to 1.",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"description": "Last line to return (1-based, inclusive). Omit to read to EOF.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Path      string `json:"path"`
				StartLine int    `json:"start_line"`
				EndLine   int    `json:"end_line"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("read_file: bad args: %v", err)}, nil
			}
			if p.Path == "" {
				return tools.Result{IsError: true, Text: "read_file: path is required"}, nil
			}
			f, err := os.Open(p.Path)
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("read_file: %v", err)}, nil
			}
			defer f.Close()

			start := p.StartLine
			if start < 1 {
				start = 1
			}
			end := p.EndLine

			var sb strings.Builder
			scanner := bufio.NewScanner(io.LimitReader(f, maxReadBytes))
			lineNum := 0
			truncated := false
			for scanner.Scan() {
				lineNum++
				if lineNum < start {
					continue
				}
				if end > 0 && lineNum > end {
					break
				}
				fmt.Fprintf(&sb, "%d\t%s\n", lineNum, scanner.Text())
				if sb.Len() > maxReadBytes {
					truncated = true
					break
				}
			}
			if err := scanner.Err(); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("read_file: scan: %v", err)}, nil
			}
			out := sb.String()
			if truncated {
				out += fmt.Sprintf("\n[truncated after %d bytes]\n", maxReadBytes)
			}
			return tools.Result{Text: out}, nil
		},
	}
}

// ── glob_search ───────────────────────────────────────────────────────────

func globSearchTool() tools.Tool {
	return tools.Tool{
		Name:        "glob_search",
		Description: "Find files matching a glob pattern. Returns matching paths, sorted, up to 500 results.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"pattern"},
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern applied to file names, e.g. '*.go' or '*.ts'.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Root directory to search. Defaults to the current working directory.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("glob_search: bad args: %v", err)}, nil
			}
			if p.Pattern == "" {
				return tools.Result{IsError: true, Text: "glob_search: pattern is required"}, nil
			}
			root := p.Path
			if root == "" {
				var err error
				root, err = os.Getwd()
				if err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("glob_search: getwd: %v", err)}, nil
				}
			}

			var matches []string
			if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil || len(matches) >= maxGlobResults {
					return nil
				}
				rel, _ := filepath.Rel(root, path)
				// match against relative path and base name
				ok, _ := filepath.Match(p.Pattern, rel)
				if !ok {
					ok, _ = filepath.Match(p.Pattern, d.Name())
				}
				if ok {
					matches = append(matches, rel)
				}
				return nil
			}); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("glob_search: walk: %v", err)}, nil
			}

			if len(matches) == 0 {
				return tools.Result{Text: "No files matched the pattern."}, nil
			}
			suffix := ""
			if len(matches) >= maxGlobResults {
				suffix = fmt.Sprintf("\n[results capped at %d]\n", maxGlobResults)
			}
			return tools.Result{Text: strings.Join(matches, "\n") + suffix}, nil
		},
	}
}

// ── grep_search ───────────────────────────────────────────────────────────

func grepSearchTool() tools.Tool {
	return tools.Tool{
		Name:        "grep_search",
		Description: "Search file contents by regex, returning file:line:text matches. Up to 200 results.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"pattern"},
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Regular expression to search for.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory or file to search. Defaults to CWD.",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "Glob pattern to filter file names, e.g. '*.go'. Empty matches all files.",
				},
				"case_insensitive": map[string]any{
					"type":        "boolean",
					"description": "If true, match case-insensitively. Defaults to false.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Pattern         string `json:"pattern"`
				Path            string `json:"path"`
				Include         string `json:"include"`
				CaseInsensitive bool   `json:"case_insensitive"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("grep_search: bad args: %v", err)}, nil
			}
			if p.Pattern == "" {
				return tools.Result{IsError: true, Text: "grep_search: pattern is required"}, nil
			}

			rxStr := p.Pattern
			if p.CaseInsensitive {
				rxStr = "(?i)" + rxStr
			}
			rx, err := regexp.Compile(rxStr)
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("grep_search: invalid regex: %v", err)}, nil
			}

			root := p.Path
			if root == "" {
				root, _ = os.Getwd()
			}

			var results []string
			grepFile := func(path string) {
				if len(results) >= maxGrepMatches {
					return
				}
				f, err := os.Open(path)
				if err != nil {
					return
				}
				defer f.Close()

				scanner := bufio.NewScanner(io.LimitReader(f, maxReadBytes))
				lineNum := 0
				for scanner.Scan() {
					lineNum++
					if rx.MatchString(scanner.Text()) {
						results = append(results, fmt.Sprintf("%s:%d:%s", path, lineNum, scanner.Text()))
						if len(results) >= maxGrepMatches {
							break
						}
					}
				}
			}

			info, err := os.Stat(root)
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("grep_search: %v", err)}, nil
			}
			if !info.IsDir() {
				grepFile(root)
			} else {
				_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
					if err != nil || d.IsDir() || len(results) >= maxGrepMatches {
						return nil
					}
					if p.Include != "" {
						ok, _ := filepath.Match(p.Include, d.Name())
						if !ok {
							return nil
						}
					}
					if isBinaryExt(d.Name()) {
						return nil
					}
					grepFile(path)
					return nil
				})
			}

			if len(results) == 0 {
				return tools.Result{Text: "No matches found."}, nil
			}
			suffix := ""
			if len(results) >= maxGrepMatches {
				suffix = fmt.Sprintf("\n[results capped at %d]\n", maxGrepMatches)
			}
			return tools.Result{Text: strings.Join(results, "\n") + suffix}, nil
		},
	}
}

// ── web_fetch ─────────────────────────────────────────────────────────────

func webFetchTool() tools.Tool {
	return tools.Tool{
		Name:        "web_fetch",
		Description: "Fetch a URL (http/https) or local file (file:// or plain path) and return its text content.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"url"},
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "HTTP/HTTPS URL, file:// URI, or local filesystem path.",
				},
				"timeout_s": map[string]any{
					"type":        "integer",
					"description": "Request timeout in seconds. Defaults to 15.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				URL      string `json:"url"`
				TimeoutS int    `json:"timeout_s"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("web_fetch: bad args: %v", err)}, nil
			}
			if p.URL == "" {
				return tools.Result{IsError: true, Text: "web_fetch: url is required"}, nil
			}
			timeout := time.Duration(p.TimeoutS) * time.Second
			if timeout <= 0 {
				timeout = 15 * time.Second
			}

			if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
				localPath := strings.TrimPrefix(p.URL, "file://")
				data, err := os.ReadFile(localPath)
				if err != nil {
					return tools.Result{IsError: true, Text: fmt.Sprintf("web_fetch: %v", err)}, nil
				}
				if len(data) > maxReadBytes {
					data = data[:maxReadBytes]
				}
				return tools.Result{Text: string(data)}, nil
			}

			client := &http.Client{Timeout: timeout}
			resp, err := client.Get(p.URL)
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("web_fetch: %v", err)}, nil
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(io.LimitReader(resp.Body, maxReadBytes))
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("web_fetch: read body: %v", err)}, nil
			}
			return tools.Result{Text: string(body)}, nil
		},
	}
}

// ── tool_search ───────────────────────────────────────────────────────────

func toolSearchTool(reg []tools.Tool) tools.Tool {
	return tools.Tool{
		Name:        "tool_search",
		Description: "Search the active tool registry by keyword. Returns matching tool names and descriptions.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Keyword or phrase to search for in tool names and descriptions.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("tool_search: bad args: %v", err)}, nil
			}
			if p.Query == "" {
				return tools.Result{IsError: true, Text: "tool_search: query is required"}, nil
			}
			if reg == nil {
				return tools.Result{IsError: true, Text: "tool_search: no tool registry available"}, nil
			}
			q := strings.ToLower(p.Query)
			var hits []string
			for _, t := range reg {
				if strings.Contains(strings.ToLower(t.Name), q) ||
					strings.Contains(strings.ToLower(t.Description), q) {
					hits = append(hits, fmt.Sprintf("%-20s %s", t.Name, t.Description))
				}
			}
			if len(hits) == 0 {
				return tools.Result{Text: fmt.Sprintf("No tools matched %q.", p.Query)}, nil
			}
			return tools.Result{Text: strings.Join(hits, "\n")}, nil
		},
	}
}

// ── sleep ─────────────────────────────────────────────────────────────────

func sleepTool() tools.Tool {
	return tools.Tool{
		Name:        "sleep",
		Description: fmt.Sprintf("Pause execution for up to %.0f seconds. Useful for polling loops or rate-limit back-off.", maxSleepSeconds),
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"seconds"},
			"properties": map[string]any{
				"seconds": map[string]any{
					"type":        "number",
					"description": fmt.Sprintf("Seconds to sleep (max %.0f).", maxSleepSeconds),
				},
			},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Seconds float64 `json:"seconds"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("sleep: bad args: %v", err)}, nil
			}
			d := p.Seconds
			if d <= 0 {
				return tools.Result{Text: "slept 0 seconds"}, nil
			}
			if d > maxSleepSeconds {
				d = maxSleepSeconds
			}
			select {
			case <-time.After(time.Duration(d * float64(time.Second))):
				return tools.Result{Text: fmt.Sprintf("slept %.2f seconds", d)}, nil
			case <-ctx.Done():
				return tools.Result{IsError: true, Text: "sleep: context cancelled"}, nil
			}
		},
	}
}

// ── write_file ────────────────────────────────────────────────────────────

func writeFileTool() tools.Tool {
	return tools.Tool{
		Name:        "write_file",
		Description: "Write or overwrite a file with the given content. Creates parent directories as needed.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"path", "content"},
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path to write.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "File content to write.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("write_file: bad args: %v", err)}, nil
			}
			if p.Path == "" {
				return tools.Result{IsError: true, Text: "write_file: path is required"}, nil
			}
			if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("write_file: mkdir: %v", err)}, nil
			}
			if err := os.WriteFile(p.Path, []byte(p.Content), 0o644); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("write_file: %v", err)}, nil
			}
			lines := strings.Count(p.Content, "\n")
			if p.Content != "" && !strings.HasSuffix(p.Content, "\n") {
				lines++
			}
			return tools.Result{Text: fmt.Sprintf("wrote %d bytes (%d lines) to %s", len(p.Content), lines, p.Path)}, nil
		},
	}
}

// ── edit_file ─────────────────────────────────────────────────────────────

func editFileTool() tools.Tool {
	return tools.Tool{
		Name:        "edit_file",
		Description: "Edit a file by replacing the first occurrence of old_str with new_str. Fails if old_str is not found or occurs more than once.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"path", "old_str", "new_str"},
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path to edit.",
				},
				"old_str": map[string]any{
					"type":        "string",
					"description": "Exact string to find and replace.",
				},
				"new_str": map[string]any{
					"type":        "string",
					"description": "Replacement string.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Path   string `json:"path"`
				OldStr string `json:"old_str"`
				NewStr string `json:"new_str"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("edit_file: bad args: %v", err)}, nil
			}
			if p.Path == "" {
				return tools.Result{IsError: true, Text: "edit_file: path is required"}, nil
			}
			data, err := os.ReadFile(p.Path)
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("edit_file: %v", err)}, nil
			}
			content := string(data)
			count := strings.Count(content, p.OldStr)
			if count == 0 {
				return tools.Result{IsError: true, Text: fmt.Sprintf("edit_file: old_str not found in %s", p.Path)}, nil
			}
			if count > 1 {
				return tools.Result{IsError: true, Text: fmt.Sprintf("edit_file: old_str found %d times in %s — provide more context to make it unique", count, p.Path)}, nil
			}
			updated := strings.Replace(content, p.OldStr, p.NewStr, 1)
			if err := os.WriteFile(p.Path, []byte(updated), 0o644); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("edit_file: write: %v", err)}, nil
			}
			delta := len(updated) - len(content)
			sign := "+"
			if delta < 0 {
				sign = ""
			}
			return tools.Result{Text: fmt.Sprintf("edited %s (%s%d bytes)", p.Path, sign, delta)}, nil
		},
	}
}

// ── notebook_edit ─────────────────────────────────────────────────────────

func notebookEditTool() tools.Tool {
	return tools.Tool{
		Name:        "notebook_edit",
		Description: "Edit a Jupyter notebook (.ipynb) cell. Specify cell_index (0-based) and new_source. Optionally change cell_type (code/markdown/raw).",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"path", "cell_index", "new_source"},
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the .ipynb file.",
				},
				"cell_index": map[string]any{
					"type":        "integer",
					"description": "0-based index of the cell to edit.",
				},
				"new_source": map[string]any{
					"type":        "string",
					"description": "New source content for the cell.",
				},
				"cell_type": map[string]any{
					"type":        "string",
					"enum":        []string{"code", "markdown", "raw"},
					"description": "Cell type. If omitted, the existing type is preserved.",
				},
			},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Path      string `json:"path"`
				CellIndex int    `json:"cell_index"`
				NewSource string `json:"new_source"`
				CellType  string `json:"cell_type"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("notebook_edit: bad args: %v", err)}, nil
			}
			if p.Path == "" {
				return tools.Result{IsError: true, Text: "notebook_edit: path is required"}, nil
			}

			data, err := os.ReadFile(p.Path)
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("notebook_edit: %v", err)}, nil
			}

			var nb map[string]any
			if err := json.Unmarshal(data, &nb); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("notebook_edit: parse notebook: %v", err)}, nil
			}

			cellsRaw, ok := nb["cells"]
			if !ok {
				return tools.Result{IsError: true, Text: "notebook_edit: notebook has no 'cells' field"}, nil
			}
			cells, ok := cellsRaw.([]any)
			if !ok {
				return tools.Result{IsError: true, Text: "notebook_edit: 'cells' is not an array"}, nil
			}
			if p.CellIndex < 0 || p.CellIndex >= len(cells) {
				return tools.Result{IsError: true, Text: fmt.Sprintf("notebook_edit: cell_index %d out of range (notebook has %d cells)", p.CellIndex, len(cells))}, nil
			}
			cell, ok := cells[p.CellIndex].(map[string]any)
			if !ok {
				return tools.Result{IsError: true, Text: fmt.Sprintf("notebook_edit: cell %d is not an object", p.CellIndex)}, nil
			}

			cell["source"] = splitLines(p.NewSource)
			if p.CellType != "" {
				cell["cell_type"] = p.CellType
			}
			cells[p.CellIndex] = cell
			nb["cells"] = cells

			out, err := json.MarshalIndent(nb, "", " ")
			if err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("notebook_edit: marshal: %v", err)}, nil
			}
			if err := os.WriteFile(p.Path, out, 0o644); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("notebook_edit: write: %v", err)}, nil
			}
			return tools.Result{Text: fmt.Sprintf("updated cell %d in %s", p.CellIndex, p.Path)}, nil
		},
	}
}

// ── bash ──────────────────────────────────────────────────────────────────

func bashTool() tools.Tool {
	return tools.Tool{
		Name:        "bash",
		Description: fmt.Sprintf("Execute a shell command via bash -c. Captures stdout+stderr. Max timeout: %d seconds.", maxBashSeconds),
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"command"},
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute.",
				},
				"timeout_s": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("Timeout in seconds (max %d). Defaults to %d.", maxBashSeconds, maxBashSeconds),
				},
			},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (tools.Result, error) {
			var p struct {
				Command  string `json:"command"`
				TimeoutS int    `json:"timeout_s"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Result{IsError: true, Text: fmt.Sprintf("bash: bad args: %v", err)}, nil
			}
			if p.Command == "" {
				return tools.Result{IsError: true, Text: "bash: command is required"}, nil
			}
			timeout := time.Duration(p.TimeoutS) * time.Second
			if timeout <= 0 || timeout > maxBashSeconds*time.Second {
				timeout = maxBashSeconds * time.Second
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf

			runErr := cmd.Run()
			out := buf.String()
			if len(out) > maxReadBytes {
				out = out[:maxReadBytes] + fmt.Sprintf("\n[truncated after %d bytes]", maxReadBytes)
			}

			if runErr != nil {
				exitCode := -1
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
				return tools.Result{
					IsError: true,
					Text:    fmt.Sprintf("exit %d\n%s", exitCode, out),
				}, nil
			}
			return tools.Result{Text: out}, nil
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".svg": true, ".pdf": true, ".zip": true,
	".tar": true, ".gz": true, ".br": true, ".zst": true, ".xz": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true,
	".wasm": true, ".bin": true, ".dat": true, ".db": true, ".sqlite": true,
	".mp3": true, ".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
}

func isBinaryExt(name string) bool {
	return binaryExts[strings.ToLower(filepath.Ext(name))]
}

// splitLines turns a multi-line string into []string where each element
// ends with "\n" except the last line (nbformat convention).
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	scanner := bufio.NewScanner(strings.NewReader(s))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text()+"\n")
	}
	if len(lines) > 0 {
		lines[len(lines)-1] = strings.TrimSuffix(lines[len(lines)-1], "\n")
	}
	return lines
}
