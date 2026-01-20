package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

var builtinGitManifest = []byte(`{
  "id": "builtin.git",
  "version": "0.1.0",
  "kind": "builtin",
  "displayName": "Builtin Git",
  "description": "Runs common git operations inside the host-configured root directory and returns structured results with bounded stdout/stderr previews.",
  "exposeAsFunctions": true,
  "actions": [
    {
      "id": "status",
      "displayName": "Status",
      "description": "Get git working tree status (structured; uses porcelain v2).",
      "inputSchema": {
        "type": "object",
        "properties": { "cwd": { "type": "string" } }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" },
          "head": { "type": "string" },
          "upstream": { "type": "string" },
          "ahead": { "type": "integer" },
          "behind": { "type": "integer" },
          "staged": { "type": "array", "items": { "type": "string" } },
          "unstaged": { "type": "array", "items": { "type": "string" } },
          "untracked": { "type": "array", "items": { "type": "string" } },
          "conflicts": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["exitCode","stdout","stderr","staged","unstaged","untracked","conflicts"]
      }
    },
    {
      "id": "log",
      "displayName": "Log",
      "description": "List recent commits.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "n": { "type": "integer" },
          "format": { "type": "string" }
        }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" },
          "commits": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "hash": { "type": "string" },
                "subject": { "type": "string" },
                "authorName": { "type": "string" },
                "authorEmail": { "type": "string" },
                "date": { "type": "string" }
              },
              "required": ["hash","subject","authorName","authorEmail","date"]
            }
          }
        },
        "required": ["exitCode","stdout","stderr","commits"]
      }
    },
    {
      "id": "diff",
      "displayName": "Diff",
      "description": "Show a diff patch (working tree or staged).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "staged": { "type": "boolean" },
          "file": { "type": "string" }
        }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" },
          "patch": { "type": "string" },
          "patchPath": { "type": "string" },
          "truncated": { "type": "boolean" },
          "limit": { "type": "string" }
        },
        "required": ["exitCode","stdout","stderr","patch","truncated"]
      }
    },
    {
      "id": "branch",
      "displayName": "Branch",
      "description": "List branches, or create a branch (no checkout).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "create": { "type": "string" }
        }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" },
          "branches": { "type": "array", "items": { "type": "string" } },
          "created": { "type": "string" }
        },
        "required": ["exitCode","stdout","stderr","branches"]
      }
    },
    {
      "id": "add",
      "displayName": "Add",
      "description": "Stage files.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "files": { "type": "array", "items": { "type": "string" }, "minItems": 1 }
        },
        "required": ["files"]
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" },
          "addedFiles": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["exitCode","stdout","stderr","addedFiles"]
      }
    },
    {
      "id": "commit",
      "displayName": "Commit",
      "description": "Create a commit with a message (hooks run; no --no-verify).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "message": { "type": "string" }
        },
        "required": ["message"]
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "exitCode": { "type": "integer" },
          "stdout": { "type": "string" },
          "stderr": { "type": "string" },
          "stdoutPath": { "type": "string" },
          "stderrPath": { "type": "string" },
          "committed": { "type": "boolean" },
          "head": { "type": "string" }
        },
        "required": ["exitCode","stdout","stderr","committed"]
      }
    }
  ]
}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.git"),
		Manifest: builtinGitManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			return NewBuiltinGitInvoker(cfg.BashRootDir)
		},
	})
}

const (
	defaultGitMaxOutputBytes = 64 * 1024
	maxGitLogN               = 200
)

type BuiltinGitInvoker struct {
	RootDir  string
	MaxBytes int
}

func NewBuiltinGitInvoker(rootDir string) *BuiltinGitInvoker {
	return &BuiltinGitInvoker{
		RootDir:  rootDir,
		MaxBytes: defaultGitMaxOutputBytes,
	}
}

func (g *BuiltinGitInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if g == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.git invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	// Safety: if callers omit timeoutMs and the context has no deadline, apply a default.
	if req.TimeoutMs == 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
		}
	}

	switch req.ActionID {
	case "status":
		return g.gitStatus(ctx, req)
	case "log":
		return g.gitLog(ctx, req)
	case "diff":
		return g.gitDiff(ctx, req)
	case "branch":
		return g.gitBranch(ctx, req)
	case "add":
		return g.gitAdd(ctx, req)
	case "commit":
		return g.gitCommit(ctx, req)
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q", req.ActionID)}
	}
}

type gitCwdInput struct {
	Cwd string `json:"cwd,omitempty"`
}

func (g *BuiltinGitInvoker) rootAndDir(cwd string) (root string, absDir string, err error) {
	root = strings.TrimSpace(g.RootDir)
	if root == "" {
		return "", "", fmt.Errorf("rootDir is required")
	}
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("rootDir must be absolute, got %q", root)
	}

	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = "."
	}
	absDir, err = vfsutil.SafeJoinBaseDir(root, cwd)
	if err != nil {
		return "", "", err
	}
	return root, absDir, nil
}

type execResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

func (g *BuiltinGitInvoker) runGit(ctx context.Context, absDir string, args ...string) (execResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return execResult{}, &InvokeError{Code: "tool_failed", Message: "git binary not found (install git)", Err: err}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = absDir

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode := 0
	runErr := cmd.Run()
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
			return execResult{}, &InvokeError{Code: "timeout", Message: "git timed out", Retryable: true, Err: runErr}
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return execResult{}, &InvokeError{Code: "tool_failed", Message: runErr.Error(), Err: runErr}
		}
	}

	return execResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
	}, nil
}

func (g *BuiltinGitInvoker) maxBytes() int {
	if g.MaxBytes <= 0 {
		return defaultGitMaxOutputBytes
	}
	return g.MaxBytes
}

func capString(s string, maxBytes int) (preview string, truncated bool) {
	if maxBytes <= 0 {
		return s, false
	}
	if len(s) <= maxBytes {
		return s, false
	}
	return s[:maxBytes], true
}

func capTextArtifact(path string, b []byte, maxBytes int) (preview string, artifact *ToolArtifactWrite, truncated bool) {
	full := string(b)
	preview, truncated = capString(full, maxBytes)
	if truncated {
		return preview, &ToolArtifactWrite{Path: path, Bytes: append([]byte(nil), b...), MediaType: "text/plain"}, true
	}
	return preview, nil, false
}

type gitStatusInput struct {
	Cwd string `json:"cwd,omitempty"`
}

type gitStatusOutput struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`

	Head     string `json:"head,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Ahead    int    `json:"ahead,omitempty"`
	Behind   int    `json:"behind,omitempty"`

	Staged    []string `json:"staged"`
	Unstaged  []string `json:"unstaged"`
	Untracked []string `json:"untracked"`
	Conflicts []string `json:"conflicts"`
}

func (g *BuiltinGitInvoker) gitStatus(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in gitStatusInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}

	_, absDir, err := g.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	res, err := g.runGit(ctx, absDir, "status", "--porcelain=v2", "--branch", "-z")
	if err != nil {
		return ToolCallResult{}, err
	}

	maxBytes := g.maxBytes()
	stdoutPreview, stdoutArtifact, _ := capTextArtifact("stdout.txt", res.Stdout, maxBytes)
	stderrPreview, stderrArtifact, _ := capTextArtifact("stderr.txt", res.Stderr, maxBytes)

	head, upstream, ahead, behind, staged, unstaged, untracked, conflicts := parseGitStatusPorcelainV2Z(res.Stdout)
	out := gitStatusOutput{
		ExitCode:  res.ExitCode,
		Stdout:    stdoutPreview,
		Stderr:    stderrPreview,
		Head:      head,
		Upstream:  upstream,
		Ahead:     ahead,
		Behind:    behind,
		Staged:    staged,
		Unstaged:  unstaged,
		Untracked: untracked,
		Conflicts: conflicts,
	}

	artifacts := make([]ToolArtifactWrite, 0, 2)
	if stdoutArtifact != nil {
		out.StdoutPath = stdoutArtifact.Path
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrArtifact != nil {
		out.StderrPath = stderrArtifact.Path
		artifacts = append(artifacts, *stderrArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

// parseGitStatusPorcelainV2Z parses a best-effort subset of `git status --porcelain=v2 --branch -z`.
func parseGitStatusPorcelainV2Z(b []byte) (head, upstream string, ahead, behind int, staged, unstaged, untracked, conflicts []string) {
	chunks := bytes.Split(b, []byte{0})
	staged = []string{}
	unstaged = []string{}
	untracked = []string{}
	conflicts = []string{}

	for _, c := range chunks {
		if len(c) == 0 {
			continue
		}
		s := string(c)
		if strings.HasPrefix(s, "# ") {
			// Examples:
			//   # branch.head main
			//   # branch.upstream origin/main
			//   # branch.ab +1 -2
			s = strings.TrimPrefix(s, "# ")
			switch {
			case strings.HasPrefix(s, "branch.head "):
				head = strings.TrimSpace(strings.TrimPrefix(s, "branch.head "))
			case strings.HasPrefix(s, "branch.upstream "):
				upstream = strings.TrimSpace(strings.TrimPrefix(s, "branch.upstream "))
			case strings.HasPrefix(s, "branch.ab "):
				ab := strings.TrimSpace(strings.TrimPrefix(s, "branch.ab "))
				parts := strings.Fields(ab)
				for _, p := range parts {
					if strings.HasPrefix(p, "+") {
						if n, err := strconv.Atoi(strings.TrimPrefix(p, "+")); err == nil {
							ahead = n
						}
					}
					if strings.HasPrefix(p, "-") {
						if n, err := strconv.Atoi(strings.TrimPrefix(p, "-")); err == nil {
							behind = n
						}
					}
				}
			}
			continue
		}

		switch {
		case strings.HasPrefix(s, "? "):
			p := strings.TrimSpace(strings.TrimPrefix(s, "? "))
			if p != "" {
				untracked = append(untracked, p)
			}
		case strings.HasPrefix(s, "u "):
			// "u <xy> <sub> <m1> <m2> <m3> <m4> <h1> <h2> <h3> <path>"
			parts := strings.SplitN(s, " ", 11)
			if len(parts) == 11 {
				p := strings.TrimSpace(parts[10])
				if p != "" {
					conflicts = append(conflicts, p)
				}
			}
		case strings.HasPrefix(s, "1 "):
			// "1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>"
			parts := strings.SplitN(s, " ", 9)
			if len(parts) != 9 {
				continue
			}
			xy := parts[1]
			p := strings.TrimSpace(parts[8])
			if len(xy) >= 2 && p != "" {
				if xy[0] != '.' {
					staged = append(staged, p)
				}
				if xy[1] != '.' {
					unstaged = append(unstaged, p)
				}
			}
		case strings.HasPrefix(s, "2 "):
			// "2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <Xscore> <path>" (orig path follows as next NUL chunk)
			parts := strings.SplitN(s, " ", 10)
			if len(parts) != 10 {
				continue
			}
			xy := parts[1]
			p := strings.TrimSpace(parts[9])
			if len(xy) >= 2 && p != "" {
				if xy[0] != '.' {
					staged = append(staged, p)
				}
				if xy[1] != '.' {
					unstaged = append(unstaged, p)
				}
			}
		}
	}

	return head, upstream, ahead, behind, uniqueStrings(staged), uniqueStrings(unstaged), uniqueStrings(untracked), uniqueStrings(conflicts)
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

type gitLogInput struct {
	Cwd    string `json:"cwd,omitempty"`
	N      int    `json:"n,omitempty"`
	Format string `json:"format,omitempty"`
}

type gitCommitInfo struct {
	Hash        string `json:"hash"`
	Subject     string `json:"subject"`
	AuthorName  string `json:"authorName"`
	AuthorEmail string `json:"authorEmail"`
	Date        string `json:"date"`
}

type gitLogOutput struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`

	Commits []gitCommitInfo `json:"commits"`
}

func (g *BuiltinGitInvoker) gitLog(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in gitLogInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := g.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	n := in.N
	if n == 0 {
		n = 20
	}
	if n < 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "n must be >= 0"}
	}
	if n > maxGitLogN {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("n exceeds max %d", maxGitLogN)}
	}

	// Safe, parseable default format. User-provided format is allowlisted.
	format := strings.TrimSpace(in.Format)
	switch format {
	case "", "default":
		format = "%H%x1f%an%x1f%ae%x1f%ad%x1f%s%x1e"
	case "oneline":
		format = "%H%x1f%ad%x1f%s%x1e"
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "unsupported format (allowed: default, oneline)"}
	}

	args := []string{"log", "-n", strconv.Itoa(n), "--date=iso-strict", "--pretty=format:" + format}
	res, err := g.runGit(ctx, absDir, args...)
	if err != nil {
		return ToolCallResult{}, err
	}

	maxBytes := g.maxBytes()
	stdoutPreview, stdoutArtifact, _ := capTextArtifact("stdout.txt", res.Stdout, maxBytes)
	stderrPreview, stderrArtifact, _ := capTextArtifact("stderr.txt", res.Stderr, maxBytes)

	commits := parseGitLogRecords(res.Stdout)
	out := gitLogOutput{
		ExitCode: res.ExitCode,
		Stdout:   stdoutPreview,
		Stderr:   stderrPreview,
		Commits:  commits,
	}
	artifacts := make([]ToolArtifactWrite, 0, 2)
	if stdoutArtifact != nil {
		out.StdoutPath = stdoutArtifact.Path
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrArtifact != nil {
		out.StderrPath = stderrArtifact.Path
		artifacts = append(artifacts, *stderrArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

func parseGitLogRecords(b []byte) []gitCommitInfo {
	// Records are separated by \x1e, fields by \x1f.
	rs := bytes.Split(b, []byte{0x1e})
	out := make([]gitCommitInfo, 0, len(rs))
	for _, r := range rs {
		if len(r) == 0 {
			continue
		}
		fs := bytes.Split(r, []byte{0x1f})
		// default: 5 fields; oneline: 3 fields
		if len(fs) == 5 {
			out = append(out, gitCommitInfo{
				Hash:        string(fs[0]),
				AuthorName:  string(fs[1]),
				AuthorEmail: string(fs[2]),
				Date:        string(fs[3]),
				Subject:     string(fs[4]),
			})
		} else if len(fs) == 3 {
			out = append(out, gitCommitInfo{
				Hash:    string(fs[0]),
				Date:    string(fs[1]),
				Subject: string(fs[2]),
				// Keep required fields present for schema compatibility.
				AuthorName:  "",
				AuthorEmail: "",
			})
		}
	}
	return out
}

type gitDiffInput struct {
	Cwd    string `json:"cwd,omitempty"`
	Staged bool   `json:"staged,omitempty"`
	File   string `json:"file,omitempty"`
}

type gitDiffOutput struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`

	Patch     string `json:"patch"`
	PatchPath string `json:"patchPath,omitempty"`
	Truncated bool   `json:"truncated"`
	Limit     string `json:"limit,omitempty"`
}

func (g *BuiltinGitInvoker) gitDiff(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in gitDiffInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := g.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	args := []string{"diff", "--no-color"}
	if in.Staged {
		args = append(args, "--staged")
	}
	if strings.TrimSpace(in.File) != "" {
		p := strings.TrimSpace(in.File)
		if _, err := vfsutil.CleanRelPath(p); err != nil {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid file: %v", err)}
		}
		args = append(args, "--", filepath.FromSlash(p))
	}

	res, err := g.runGit(ctx, absDir, args...)
	if err != nil {
		return ToolCallResult{}, err
	}

	maxBytes := g.maxBytes()
	stdoutPreview, stdoutArtifact, _ := capTextArtifact("stdout.txt", res.Stdout, maxBytes)
	stderrPreview, stderrArtifact, _ := capTextArtifact("stderr.txt", res.Stderr, maxBytes)

	patchPreview, patchArtifact, patchTruncated := capTextArtifact("diff.patch", res.Stdout, maxBytes)
	out := gitDiffOutput{
		ExitCode:  res.ExitCode,
		Stdout:    stdoutPreview,
		Stderr:    stderrPreview,
		Patch:     patchPreview,
		Truncated: patchTruncated,
	}

	artifacts := make([]ToolArtifactWrite, 0, 3)
	if stdoutArtifact != nil {
		out.StdoutPath = stdoutArtifact.Path
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrArtifact != nil {
		out.StderrPath = stderrArtifact.Path
		artifacts = append(artifacts, *stderrArtifact)
	}
	if patchArtifact != nil {
		out.PatchPath = patchArtifact.Path
		out.Limit = "maxBytes"
		artifacts = append(artifacts, *patchArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

type gitBranchInput struct {
	Cwd    string `json:"cwd,omitempty"`
	Create string `json:"create,omitempty"`
}

type gitBranchOutput struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`

	Branches []string `json:"branches"`
	Created  string   `json:"created,omitempty"`
}

func (g *BuiltinGitInvoker) gitBranch(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in gitBranchInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := g.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	created := ""
	if strings.TrimSpace(in.Create) != "" {
		name := strings.TrimSpace(in.Create)
		// Validate branch name using git itself.
		check, err := g.runGit(ctx, absDir, "check-ref-format", "--branch", name)
		if err != nil {
			return ToolCallResult{}, err
		}
		if check.ExitCode != 0 {
			msg := strings.TrimSpace(string(check.Stderr))
			if msg == "" {
				msg = "invalid branch name"
			}
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: msg}
		}
		res, err := g.runGit(ctx, absDir, "branch", name)
		if err != nil {
			return ToolCallResult{}, err
		}
		_ = res // creation output captured by list below
		created = name
	}

	res, err := g.runGit(ctx, absDir, "branch", "--format=%(refname:short)")
	if err != nil {
		return ToolCallResult{}, err
	}

	maxBytes := g.maxBytes()
	stdoutPreview, stdoutArtifact, _ := capTextArtifact("stdout.txt", res.Stdout, maxBytes)
	stderrPreview, stderrArtifact, _ := capTextArtifact("stderr.txt", res.Stderr, maxBytes)

	branches := []string{}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		branches = append(branches, line)
	}

	out := gitBranchOutput{
		ExitCode: res.ExitCode,
		Stdout:   stdoutPreview,
		Stderr:   stderrPreview,
		Branches: branches,
		Created:  created,
	}
	artifacts := make([]ToolArtifactWrite, 0, 2)
	if stdoutArtifact != nil {
		out.StdoutPath = stdoutArtifact.Path
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrArtifact != nil {
		out.StderrPath = stderrArtifact.Path
		artifacts = append(artifacts, *stderrArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

type gitAddInput struct {
	Cwd   string   `json:"cwd,omitempty"`
	Files []string `json:"files"`
}

type gitAddOutput struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`

	AddedFiles []string `json:"addedFiles"`
}

func (g *BuiltinGitInvoker) gitAdd(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in gitAddInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := g.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if len(in.Files) == 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "files is required"}
	}

	files := make([]string, 0, len(in.Files))
	for _, f := range in.Files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		clean, err := vfsutil.CleanRelPath(f)
		if err != nil {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid file path %q: %v", f, err)}
		}
		if clean == "." {
			return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "file path cannot be '.'"}
		}
		files = append(files, filepath.FromSlash(clean))
	}
	if len(files) == 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "files is required"}
	}

	args := append([]string{"add", "--"}, files...)
	res, err := g.runGit(ctx, absDir, args...)
	if err != nil {
		return ToolCallResult{}, err
	}

	maxBytes := g.maxBytes()
	stdoutPreview, stdoutArtifact, _ := capTextArtifact("stdout.txt", res.Stdout, maxBytes)
	stderrPreview, stderrArtifact, _ := capTextArtifact("stderr.txt", res.Stderr, maxBytes)

	out := gitAddOutput{
		ExitCode:   res.ExitCode,
		Stdout:     stdoutPreview,
		Stderr:     stderrPreview,
		AddedFiles: make([]string, 0, len(files)),
	}
	for _, f := range in.Files {
		f = strings.TrimSpace(f)
		if f != "" {
			out.AddedFiles = append(out.AddedFiles, f)
		}
	}

	artifacts := make([]ToolArtifactWrite, 0, 2)
	if stdoutArtifact != nil {
		out.StdoutPath = stdoutArtifact.Path
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrArtifact != nil {
		out.StderrPath = stderrArtifact.Path
		artifacts = append(artifacts, *stderrArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

type gitCommitInput struct {
	Cwd     string `json:"cwd,omitempty"`
	Message string `json:"message"`
}

type gitCommitOutput struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`

	StdoutPath string `json:"stdoutPath,omitempty"`
	StderrPath string `json:"stderrPath,omitempty"`

	Committed bool   `json:"committed"`
	Head      string `json:"head,omitempty"`
}

func (g *BuiltinGitInvoker) gitCommit(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in gitCommitInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	_, absDir, err := g.rootAndDir(in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "message is required"}
	}

	res, err := g.runGit(ctx, absDir, "commit", "-m", msg)
	if err != nil {
		return ToolCallResult{}, err
	}

	head := ""
	committed := res.ExitCode == 0
	if committed {
		rev, revErr := g.runGit(ctx, absDir, "rev-parse", "HEAD")
		if revErr == nil && rev.ExitCode == 0 {
			head = strings.TrimSpace(string(rev.Stdout))
		}
	}

	maxBytes := g.maxBytes()
	stdoutPreview, stdoutArtifact, _ := capTextArtifact("stdout.txt", res.Stdout, maxBytes)
	stderrPreview, stderrArtifact, _ := capTextArtifact("stderr.txt", res.Stderr, maxBytes)

	out := gitCommitOutput{
		ExitCode:  res.ExitCode,
		Stdout:    stdoutPreview,
		Stderr:    stderrPreview,
		Committed: committed,
		Head:      head,
	}
	artifacts := make([]ToolArtifactWrite, 0, 2)
	if stdoutArtifact != nil {
		out.StdoutPath = stdoutArtifact.Path
		artifacts = append(artifacts, *stdoutArtifact)
	}
	if stderrArtifact != nil {
		out.StderrPath = stderrArtifact.Path
		artifacts = append(artifacts, *stderrArtifact)
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}
