package codexcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/pkg/harness"
	"github.com/tinoosan/agen8/pkg/types"
)

const AdapterID = "codex-cli"

// Config controls codex exec invocation.
type Config struct {
	Binary           string
	Model            string
	Profile          string
	ExtraArgs        []string
	SkipGitRepoCheck bool
	Ephemeral        bool
}

type Adapter struct {
	cfg Config
}

func New(cfg Config) *Adapter {
	if strings.TrimSpace(cfg.Binary) == "" {
		cfg.Binary = "codex"
	}
	if !cfg.SkipGitRepoCheck {
		cfg.SkipGitRepoCheck = true
	}
	if !cfg.Ephemeral {
		cfg.Ephemeral = true
	}
	return &Adapter{cfg: cfg}
}

func (a *Adapter) ID() string { return AdapterID }

func (a *Adapter) RunTask(ctx context.Context, req harness.TaskRequest) (harness.TaskResult, error) {
	if a == nil {
		return harness.TaskResult{}, fmt.Errorf("codex adapter is nil")
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		return harness.TaskResult{}, fmt.Errorf("task goal is required")
	}

	lastMsgFile, err := os.CreateTemp("", "agen8-codex-last-message-*.txt")
	if err != nil {
		return harness.TaskResult{}, err
	}
	lastMsgPath := lastMsgFile.Name()
	_ = lastMsgFile.Close()
	defer os.Remove(lastMsgPath)

	args := []string{"exec", "--json", "--output-last-message", lastMsgPath}
	if a.cfg.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	if a.cfg.Ephemeral {
		args = append(args, "--ephemeral")
	}
	if v := strings.TrimSpace(a.cfg.Model); v != "" {
		args = append(args, "--model", v)
	}
	if v := strings.TrimSpace(a.cfg.Profile); v != "" {
		args = append(args, "--profile", v)
	}
	workdir := strings.TrimSpace(req.Workdir)
	if workdir != "" {
		args = append(args, "--cd", workdir)
	}
	for _, extra := range a.cfg.ExtraArgs {
		extra = strings.TrimSpace(extra)
		if extra == "" {
			continue
		}
		args = append(args, extra)
	}
	args = append(args, goal)

	cmd := exec.CommandContext(ctx, strings.TrimSpace(a.cfg.Binary), args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	usage := parseUsage(stdout.Bytes())
	message := readTrimmedFile(lastMsgPath)
	if message == "" {
		message = strings.TrimSpace(stdout.String())
	}
	if message == "" {
		message = strings.TrimSpace(stderr.String())
	}

	result := harness.NormalizeResult(harness.TaskResult{
		Status:       types.TaskStatusSucceeded,
		Text:         message,
		InputTokens:  usage.inputTokens,
		OutputTokens: usage.outputTokens,
		TotalTokens:  usage.totalTokens,
		CostUSD:      usage.costUSD,
		AdapterRunID: usage.runRef,
	})

	if err == nil {
		return result, nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		result.Status = types.TaskStatusCanceled
		if strings.TrimSpace(result.Error) == "" {
			result.Error = "stopped by user"
		}
		return result, nil
	}
	result.Status = types.TaskStatusFailed
	if strings.TrimSpace(result.Error) == "" {
		result.Error = strings.TrimSpace(stderr.String())
	}
	if strings.TrimSpace(result.Error) == "" {
		result.Error = err.Error()
	}
	return result, nil
}

type usageParse struct {
	inputTokens  int
	outputTokens int
	totalTokens  int
	costUSD      float64
	runRef       string
}

func parseUsage(raw []byte) usageParse {
	var out usageParse
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if out.runRef == "" {
			out.runRef = firstString(payload, "run_id", "runId", "session_id", "sessionId", "id")
		}
		if n, ok := maxNumber(payload, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens"); ok && int(n) > out.inputTokens {
			out.inputTokens = int(n)
		}
		if n, ok := maxNumber(payload, "output_tokens", "outputTokens", "completion_tokens", "completionTokens"); ok && int(n) > out.outputTokens {
			out.outputTokens = int(n)
		}
		if n, ok := maxNumber(payload, "total_tokens", "totalTokens"); ok && int(n) > out.totalTokens {
			out.totalTokens = int(n)
		}
		if n, ok := maxNumber(payload, "cost_usd", "costUSD", "total_cost_usd", "totalCostUSD"); ok && n > out.costUSD {
			out.costUSD = n
		}
	}
	if out.totalTokens <= 0 {
		out.totalTokens = out.inputTokens + out.outputTokens
	}
	return out
}

func maxNumber(v any, keys ...string) (float64, bool) {
	keySet := map[string]struct{}{}
	for _, k := range keys {
		keySet[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}
	return findNumber(v, keySet)
}

func findNumber(v any, keys map[string]struct{}) (float64, bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, value := range t {
			if _, ok := keys[strings.ToLower(strings.TrimSpace(k))]; ok {
				if n, ok := asFloat(value); ok {
					return n, true
				}
			}
			if n, ok := findNumber(value, keys); ok {
				return n, true
			}
		}
	case []any:
		for _, item := range t {
			if n, ok := findNumber(item, keys); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func firstString(v any, keys ...string) string {
	keySet := map[string]struct{}{}
	for _, k := range keys {
		keySet[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}
	return findString(v, keySet)
}

func findString(v any, keys map[string]struct{}) string {
	switch t := v.(type) {
	case map[string]any:
		for k, value := range t {
			if _, ok := keys[strings.ToLower(strings.TrimSpace(k))]; ok {
				if s := strings.TrimSpace(fmt.Sprint(value)); s != "" {
					return s
				}
			}
			if s := findString(value, keys); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range t {
			if s := findString(item, keys); s != "" {
				return s
			}
		}
	}
	return ""
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0, false
		}
		f, err := json.Number(n).Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func readTrimmedFile(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs := path
	if !filepath.IsAbs(abs) {
		if resolved, err := filepath.Abs(abs); err == nil {
			abs = resolved
		}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
