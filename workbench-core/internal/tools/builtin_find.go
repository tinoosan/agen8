package tools

import (
	"context"
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

var builtinFindManifest = []byte(`{
  "id": "builtin.find",
  "version": "0.1.0",
  "kind": "builtin",
  "displayName": "Builtin Find",
  "description": "Finds files and directories under a host-configured root directory using simple glob patterns and returns relative paths.",
  "exposeAsFunctions": true,
  "actions": [
    {
      "id": "files",
      "displayName": "Find files",
      "description": "Find files/directories under cwd matching pattern. Pattern is a glob. If pattern contains '/', it matches against the full root-relative path; otherwise it matches against the basename. A leading '**/' matches anywhere.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cwd": { "type": "string" },
          "pattern": { "type": "string" },
          "type": { "type": "string", "enum": ["f","d"] },
          "maxResults": { "type": "integer" }
        },
        "required": ["pattern"]
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "paths": { "type": "array", "items": { "type": "string" } },
          "truncated": { "type": "boolean" },
          "limit": { "type": "string" }
        },
        "required": ["paths", "truncated"]
      }
    }
  ]
}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.find"),
		Manifest: builtinFindManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			return NewBuiltinFindInvoker(cfg.BashRootDir)
		},
	})
}

const (
	defaultFindMaxResults = 2000
	maxFindMaxResults     = 10000
)

type BuiltinFindInvoker struct {
	RootDir string
}

func NewBuiltinFindInvoker(rootDir string) *BuiltinFindInvoker {
	return &BuiltinFindInvoker{RootDir: rootDir}
}

type findFilesInput struct {
	Cwd        string `json:"cwd,omitempty"`
	Pattern    string `json:"pattern"`
	Type       string `json:"type,omitempty"` // "f" or "d"
	MaxResults int    `json:"maxResults,omitempty"`
}

type findFilesOutput struct {
	Paths     []string `json:"paths"`
	Truncated bool     `json:"truncated"`
	Limit     string   `json:"limit,omitempty"`
}

func (f *BuiltinFindInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if f == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.find invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "files" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: files)", req.ActionID)}
	}

	// Safety: if callers omit timeoutMs and the context has no deadline, apply a default.
	if req.TimeoutMs == 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
		}
	}

	root := strings.TrimSpace(f.RootDir)
	if root == "" {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "rootDir is required"}
	}
	if !filepath.IsAbs(root) {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("rootDir must be absolute, got %q", root)}
	}

	var in findFilesInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}

	in.Cwd = strings.TrimSpace(in.Cwd)
	if in.Cwd == "" {
		in.Cwd = "."
	}
	absDir, err := vfsutil.SafeJoinBaseDir(root, in.Cwd)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	pat := strings.TrimSpace(in.Pattern)
	if pat == "" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "pattern is required"}
	}
	pat = strings.ReplaceAll(pat, "\\", "/")

	wantType := strings.TrimSpace(in.Type)
	if wantType != "" && wantType != "f" && wantType != "d" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "type must be \"f\" or \"d\""}
	}

	maxResults := in.MaxResults
	if maxResults == 0 {
		maxResults = defaultFindMaxResults
	}
	if maxResults < 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "maxResults must be >= 0"}
	}
	if maxResults > maxFindMaxResults {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("maxResults exceeds max %d", maxFindMaxResults)}
	}

	out := findFilesOutput{Paths: []string{}, Truncated: false}
	// Walk from absDir, but always return paths relative to root (not cwd).
	err = filepath.WalkDir(absDir, func(p string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Ignore unreadable paths (best-effort).
			return nil
		}
		// Always skip .git; huge and not helpful for agent file discovery.
		if d != nil && d.IsDir() && d.Name() == ".git" {
			return iofs.SkipDir
		}

		// Respect cancellation.
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}

		rel, relErr := vfsutil.RelUnderBaseDir(root, p)
		if relErr != nil {
			// Shouldn't happen since we only walk under absDir, but be defensive.
			return nil
		}
		if rel == "." {
			return nil
		}

		isDir := d != nil && d.IsDir()
		if wantType == "f" && isDir {
			return nil
		}
		if wantType == "d" && !isDir {
			return nil
		}

		if matchFindPattern(pat, rel) {
			out.Paths = append(out.Paths, rel)
			if maxResults > 0 && len(out.Paths) >= maxResults {
				out.Truncated = true
				out.Limit = "maxResults"
				return iofs.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		// If WalkDir aborted due to context cancellation/timeout, return a structured timeout.
		if ctx != nil && ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return ToolCallResult{}, &InvokeError{Code: "timeout", Message: "find timed out", Retryable: true, Err: ctx.Err()}
			}
			return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "find cancelled", Retryable: true, Err: ctx.Err()}
		}
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON}, nil
}

func matchFindPattern(pattern string, rootRelPath string) bool {
	pat := strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	p := strings.TrimSpace(strings.ReplaceAll(rootRelPath, "\\", "/"))
	p = path.Clean(p)
	if p == "." {
		return false
	}

	if strings.HasPrefix(pat, "**/") {
		pat = strings.TrimPrefix(pat, "**/")
		if strings.TrimSpace(pat) == "" {
			return true
		}
		// If the remaining pattern contains '/', treat it as a suffix pattern against any segment boundary.
		if strings.Contains(pat, "/") {
			return matchAnySuffix(pat, p)
		}
		// Otherwise match basename.
		return matchGlob(pat, path.Base(p))
	}

	if strings.Contains(pat, "/") {
		return matchGlob(pat, p)
	}
	return matchGlob(pat, path.Base(p))
}

func matchAnySuffix(pattern string, p string) bool {
	parts := strings.Split(p, "/")
	for i := 0; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], "/")
		if matchGlob(pattern, suffix) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, target string) bool {
	ok, err := path.Match(pattern, target)
	if err != nil {
		// Invalid pattern -> no match.
		return false
	}
	return ok
}
