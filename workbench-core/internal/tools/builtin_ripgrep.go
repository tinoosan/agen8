package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

var builtinRipgrepManifest = []byte(`{"id":"builtin.ripgrep","version":"0.1.0","kind":"builtin","displayName":"Builtin Ripgrep","description":"Searches text under a host-configured root directory using ripgrep and returns structured matches.","exposeAsFunctions":true,"actions":[{"id":"search","displayName":"Search","description":"Search for query in files under the sandbox root. Returns structured match records (path, line, text).","inputSchema":{"type":"object","properties":{"query":{"type":"string"},"paths":{"type":"array","items":{"type":"string"}},"glob":{"type":"array","items":{"type":"string"}},"caseSensitive":{"type":"boolean"},"maxMatches":{"type":"integer"},"contextLines":{"type":"integer"}},"required":["query"]},"outputSchema":{"type":"object","properties":{"matches":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"line":{"type":"integer"},"text":{"type":"string"}},"required":["path","line","text"]}},"truncated":{"type":"boolean"},"limit":{"type":"string"}},"required":["matches","truncated"]}}]}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.ripgrep"),
		Manifest: builtinRipgrepManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			root := strings.TrimSpace(cfg.RipgrepRootDir)
			if root == "" {
				root = cfg.BashRootDir
			}
			return NewBuiltinRipgrepInvoker(root)
		},
	})
}

const (
	defaultRipgrepMaxMatches = 50
	maxRipgrepMaxMatches     = 2000
)

// BuiltinRipgrepInvoker implements the builtin tool "builtin.ripgrep" (action: "search").
//
// Why this exists (vs using builtin.bash rg ...):
//   - builtin.bash returns unstructured stdout, which is costly for the model to parse.
//   - builtin.ripgrep returns structured match records so the agent can reason cheaply
//     (path + line + text) and then decide what to open next.
//
// Confinement:
//   - RootDir is an absolute OS path configured by the host.
//   - Input paths are relative to RootDir. Absolute paths and ".." are rejected.
//
// Execution:
//   - This tool runs the "rg" binary directly (no shell) and requests JSON output via `rg --json`.
//   - Matches are parsed from rg's JSON stream and returned in ToolResponse.Output.
type BuiltinRipgrepInvoker struct {
	RootDir string

	// Hard caps to keep tool output bounded.
	DefaultMaxMatches int
}

func NewBuiltinRipgrepInvoker(rootDir string) *BuiltinRipgrepInvoker {
	return &BuiltinRipgrepInvoker{
		RootDir:           rootDir,
		DefaultMaxMatches: defaultRipgrepMaxMatches,
	}
}

type ripgrepSearchInput struct {
	Query         string   `json:"query"`
	Paths         []string `json:"paths,omitempty"`
	Glob          []string `json:"glob,omitempty"`
	CaseSensitive bool     `json:"caseSensitive,omitempty"`
	MaxMatches    int      `json:"maxMatches,omitempty"`
	ContextLines  int      `json:"contextLines,omitempty"`
}

type ripgrepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type ripgrepSearchOutput struct {
	Matches   []ripgrepMatch `json:"matches"`
	Truncated bool           `json:"truncated"`
	Limit     string         `json:"limit,omitempty"` // "maxMatches"
}

func (r *BuiltinRipgrepInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if r == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.ripgrep invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "search" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: search)", req.ActionID)}
	}
	root := strings.TrimSpace(r.RootDir)
	if root == "" {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "rootDir is required"}
	}
	if !filepath.IsAbs(root) {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("rootDir must be absolute, got %q", root)}
	}

	var in ripgrepSearchInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "query is required"}
	}

	paths := in.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	for _, p := range paths {
		if _, err := vfsutil.CleanRelPath(p); err != nil {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid paths: %v", err)}
		}
	}
	for _, g := range in.Glob {
		if strings.TrimSpace(g) == "" {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "glob entries must be non-empty"}
		}
	}
	if in.ContextLines < 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "contextLines must be >= 0"}
	}

	maxMatches := in.MaxMatches
	if maxMatches == 0 {
		maxMatches = r.DefaultMaxMatches
	}
	if maxMatches < 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "maxMatches must be >= 0"}
	}
	if maxMatches > maxRipgrepMaxMatches {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("maxMatches exceeds max %d", maxRipgrepMaxMatches)}
	}

	// Safety: callers may omit timeoutMs. To avoid indefinite hangs, apply a default
	// timeout when no timeout was provided and the caller context has no deadline.
	if req.TimeoutMs == 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
		}
	}

	argv := make([]string, 0, 16)
	argv = append(argv, "--json")
	if !in.CaseSensitive {
		argv = append(argv, "-i")
	}
	if in.ContextLines > 0 {
		argv = append(argv, "--context", strconv.Itoa(in.ContextLines))
	}
	if maxMatches > 0 {
		argv = append(argv, "--max-count", strconv.Itoa(maxMatches))
	}
	for _, g := range in.Glob {
		argv = append(argv, "--glob", g)
	}
	argv = append(argv, in.Query)
	argv = append(argv, paths...)

	// Use a derived context so we can cancel the process on early parse failures.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rg", argv...)
	cmd.Dir = root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("stdout pipe: %v", err), Err: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("stderr pipe: %v", err), Err: err}
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return ToolCallResult{}, &InvokeError{Code: "timeout", Message: "rg timed out", Retryable: true, Err: err}
		}
		if errors.Is(err, exec.ErrNotFound) {
			return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "rg binary not found (install ripgrep)"}
		}
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	// Drain stderr concurrently to avoid deadlocks when stdout/stderr pipes fill.
	// Capture up to 64KB for error reporting, and discard the rest.
	type stderrResult struct {
		b   []byte
		err error
	}
	stderrCh := make(chan stderrResult, 1)
	go func() {
		b, err := ioReadAllLimited(stderr, 64*1024)
		stderrCh <- stderrResult{b: b, err: err}
	}()

	// We treat maxMatches as a global cap on returned matches.
	// Note: ripgrep's --max-count is per-file, so we must stop ourselves.
	matchesCap := 0
	if maxMatches > 0 && maxMatches <= 256 {
		matchesCap = maxMatches
	}
	matches := make([]ripgrepMatch, 0, matchesCap)
	reachedMaxMatches := false
	sc := bufio.NewScanner(stdout)
	// Allow longer JSON lines than the default 64K.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		var env struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if env.Type != "match" {
			continue
		}
		var md struct {
			Path struct {
				Text string `json:"text"`
			} `json:"path"`
			Lines struct {
				Text string `json:"text"`
			} `json:"lines"`
			LineNumber int `json:"line_number"`
		}
		if err := json.Unmarshal(env.Data, &md); err != nil {
			continue
		}
		text := strings.TrimRight(md.Lines.Text, "\n")
		matches = append(matches, ripgrepMatch{
			Path: md.Path.Text,
			Line: md.LineNumber,
			Text: text,
		})

		if maxMatches > 0 && len(matches) >= maxMatches {
			reachedMaxMatches = true
			cancel()
			break
		}
	}

	if err := sc.Err(); err != nil {
		// Scanner errors are treated as a tool failure.
		// If we intentionally stopped after reaching maxMatches, ignore any read errors
		// caused by canceling the process.
		if reachedMaxMatches {
			err = nil
		}
		if err == nil {
			// proceed
		} else {
			cancel()
			_ = cmd.Wait()
			<-stderrCh
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ToolCallResult{}, &InvokeError{Code: "timeout", Message: "rg timed out", Retryable: true, Err: ctx.Err()}
			}
			return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("read rg output: %v", err), Err: err}
		}
	}

	waitErr := cmd.Wait()
	stderrRes := <-stderrCh
	stderrBytes := stderrRes.b

	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(waitErr, context.DeadlineExceeded) {
		timeoutErr := waitErr
		if timeoutErr == nil {
			timeoutErr = ctx.Err()
		}
		return ToolCallResult{}, &InvokeError{Code: "timeout", Message: "rg timed out", Retryable: true, Err: timeoutErr}
	}

	// If we canceled intentionally after collecting enough matches, treat any resulting
	// process termination error as expected (the output is intentionally truncated).
	if reachedMaxMatches {
		waitErr = nil
	}
	// rg returns exit code 1 when there are no matches; that is not a tool failure.
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			code := exitErr.ExitCode()
			if code != 1 {
				msg := strings.TrimSpace(string(stderrBytes))
				if msg == "" {
					msg = waitErr.Error()
				}
				return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: msg, Err: waitErr}
			}
		} else {
			return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: waitErr.Error(), Err: waitErr}
		}
	}

	out := ripgrepSearchOutput{
		Matches:   matches,
		Truncated: false,
	}
	if maxMatches > 0 && (reachedMaxMatches || len(matches) >= maxMatches) {
		out.Truncated = true
		out.Limit = "maxMatches"
	}
	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON}, nil
}

func ioReadAllLimited(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = 64 * 1024
	}
	var out bytes.Buffer
	_, err := io.Copy(&out, io.LimitReader(r, max))
	// Drain the remainder so the writer can't block on a full pipe.
	_, drainErr := io.Copy(io.Discard, r)
	if err == nil {
		err = drainErr
	}
	return out.Bytes(), err
}
