package builtins

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pkgtools "github.com/tinoosan/agen8/pkg/tools"
	"github.com/tinoosan/agen8/pkg/types"
)

//go:embed code_exec_python_wrapper.py
var codeExecPythonWrapper string

type CodeExecBridge func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
type CodeExecDispatch func(ctx context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error)

const (
	defaultCodeExecTimeoutMs    = 120_000
	maxCodeExecTimeoutMs        = 420_000
	defaultCodeExecMaxOutput    = 64 * 1024
	maxCodeExecMaxOutput        = 256 * 1024
	defaultCodeExecMaxToolCalls = 50
	maxCodeExecMaxToolCalls     = 300
	defaultCodeExecCwd          = "/workspace"
	codeExecFramePrefix         = "__WBX_CODE_EXEC__"
	codeExecPolicyDirectFSWrite = "direct_fs_write"
)

// BuiltinCodeExecInvoker runs Python code with an in-process tools bridge.
type BuiltinCodeExecInvoker struct {
	RootDir    string
	MountRoots map[string]string // mount name -> absolute host root
	PythonBin  string

	DefaultTimeoutMs   int
	MaxTimeoutMs       int
	DefaultMaxOutput   int
	MaxOutputBytes     int
	DefaultMaxToolCall int
	MaxToolCalls       int

	EnvAllowlist    []string
	RequiredImports []string

	mu       sync.RWMutex
	bridge   CodeExecBridge
	dispatch CodeExecDispatch
	allowed  map[string]struct{}
}

func NewBuiltinCodeExecInvoker(rootDir string, mountRoots map[string]string) *BuiltinCodeExecInvoker {
	cp := make(map[string]string, len(mountRoots))
	for k, v := range mountRoots {
		cp[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if _, ok := cp["project"]; !ok && strings.TrimSpace(rootDir) != "" {
		cp["project"] = strings.TrimSpace(rootDir)
	}
	return &BuiltinCodeExecInvoker{
		RootDir:            strings.TrimSpace(rootDir),
		MountRoots:         cp,
		PythonBin:          "python3",
		DefaultTimeoutMs:   defaultCodeExecTimeoutMs,
		MaxTimeoutMs:       maxCodeExecTimeoutMs,
		DefaultMaxOutput:   defaultCodeExecMaxOutput,
		MaxOutputBytes:     maxCodeExecMaxOutput,
		DefaultMaxToolCall: defaultCodeExecMaxToolCalls,
		MaxToolCalls:       maxCodeExecMaxToolCalls,
		EnvAllowlist:       defaultCodeExecEnvAllowlist(),
		allowed:            map[string]struct{}{},
	}
}

func (i *BuiltinCodeExecInvoker) SetBridge(bridge CodeExecBridge) {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.bridge = bridge
	i.mu.Unlock()
}

func (i *BuiltinCodeExecInvoker) SetDispatcher(dispatch CodeExecDispatch) {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.dispatch = dispatch
	i.mu.Unlock()
}

func (i *BuiltinCodeExecInvoker) SetToolAllowlist(names []string) {
	if i == nil {
		return
	}
	next := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		next[name] = struct{}{}
	}
	i.mu.Lock()
	i.allowed = next
	i.mu.Unlock()
}

func (i *BuiltinCodeExecInvoker) SetRequiredImports(imports []string) {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.RequiredImports = normalizeCodeExecImports(imports)
	i.mu.Unlock()
}

// EnsureReady validates Python/runtime prerequisites for code_exec.
func (i *BuiltinCodeExecInvoker) EnsureReady(ctx context.Context) error {
	if i == nil {
		return fmt.Errorf("builtin.code_exec invoker is nil")
	}
	i.mu.RLock()
	pythonBin := strings.TrimSpace(i.PythonBin)
	envAllow := append([]string(nil), i.EnvAllowlist...)
	requiredImports := append([]string(nil), i.RequiredImports...)
	i.mu.RUnlock()

	if pythonBin == "" {
		pythonBin = "python3"
	}
	if _, err := exec.LookPath(pythonBin); err != nil {
		return fmt.Errorf("code_exec preflight: python binary %q not found: %w", pythonBin, err)
	}

	requiredImports = requiredCodeExecImports(requiredImports)
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	snippet := "import importlib.util,sys; missing=[m for m in sys.argv[1:] if importlib.util.find_spec(m) is None]; print(','.join(missing)); raise SystemExit(0 if not missing else 2)"
	args := append([]string{"-c", snippet}, requiredImports...)
	cmd := exec.CommandContext(probeCtx, pythonBin, args...)
	cmd.Env = filteredCodeExecEnv(os.Environ(), envAllow)
	out, err := cmd.CombinedOutput()
	missing := strings.TrimSpace(string(out))
	if err != nil {
		if missing != "" {
			return fmt.Errorf("code_exec preflight: missing python module(s): %s", missing)
		}
		return fmt.Errorf("code_exec preflight: import probe failed: %w (%s)", err, trimmedOutput(string(out)))
	}
	if missing != "" {
		return fmt.Errorf("code_exec preflight: missing python module(s): %s", missing)
	}

	if err := runCodeExecWrapperPreflight(ctx, pythonBin, envAllow); err != nil {
		return err
	}
	return nil
}

func (i *BuiltinCodeExecInvoker) Invoke(ctx context.Context, req pkgtools.ToolRequest) (pkgtools.ToolCallResult, error) {
	if i == nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "builtin.code_exec invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "run" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: run)", req.ActionID)}
	}

	var in struct {
		Language       string `json:"language"`
		Code           string `json:"code"`
		Cwd            string `json:"cwd,omitempty"`
		TimeoutMs      int    `json:"timeoutMs,omitempty"`
		MaxOutputBytes int    `json:"maxOutputBytes,omitempty"`
		MaxToolCalls   int    `json:"maxToolCalls,omitempty"`
	}
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}

	lang := strings.ToLower(strings.TrimSpace(in.Language))
	if lang != "python" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: `language must be "python"`}
	}
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "code is required"}
	}

	i.mu.RLock()
	bridge := i.bridge
	dispatch := i.dispatch
	timeoutDef := i.DefaultTimeoutMs
	timeoutMax := i.MaxTimeoutMs
	maxOutDef := i.DefaultMaxOutput
	maxOutCap := i.MaxOutputBytes
	maxToolDef := i.DefaultMaxToolCall
	maxToolCap := i.MaxToolCalls
	pythonBin := strings.TrimSpace(i.PythonBin)
	rootDir := strings.TrimSpace(i.RootDir)
	mountRoots := make(map[string]string, len(i.MountRoots))
	for k, v := range i.MountRoots {
		mountRoots[k] = v
	}
	allow := make([]string, 0, len(i.allowed))
	for name := range i.allowed {
		allow = append(allow, name)
	}
	envAllow := append([]string(nil), i.EnvAllowlist...)
	i.mu.RUnlock()

	if bridge == nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "code_exec bridge is not configured"}
	}
	if dispatch == nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "code_exec dispatcher is not configured"}
	}

	timeoutMs := in.TimeoutMs
	if req.TimeoutMs > 0 {
		timeoutMs = req.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = timeoutDef
	}
	if timeoutMs > timeoutMax {
		timeoutMs = timeoutMax
	}

	maxOutput := in.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = maxOutDef
	}
	if maxOutput > maxOutCap {
		maxOutput = maxOutCap
	}
	if maxOutput <= 0 {
		maxOutput = defaultCodeExecMaxOutput
	}

	maxToolCalls := in.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = maxToolDef
	}
	if maxToolCalls > maxToolCap {
		maxToolCalls = maxToolCap
	}
	if maxToolCalls <= 0 {
		maxToolCalls = defaultCodeExecMaxToolCalls
	}

	if pythonBin == "" {
		pythonBin = "python3"
	}

	cwd, logicalCwd, err := resolveCodeExecCwd(rootDir, mountRoots, strings.TrimSpace(in.Cwd))
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("cwd %q is not a directory", logicalCwd)}
	}

	out, err := i.runPython(ctx, pythonBin, cwd, codeExecRunConfig{
		Code:         code,
		Allowlist:    allow,
		MaxToolCalls: maxToolCalls,
		TimeoutMs:    timeoutMs,
		MaxOutput:    maxOutput,
		Dispatch:     dispatch,
		Bridge:       bridge,
		EnvAllowlist: envAllow,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "timeout", Message: "code_exec timed out", Retryable: true, Err: err}
		}
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	b, err := json.Marshal(out)
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return pkgtools.ToolCallResult{Output: b}, nil
}

type codeExecRunConfig struct {
	Code         string
	Allowlist    []string
	MaxToolCalls int
	TimeoutMs    int
	MaxOutput    int
	Dispatch     CodeExecDispatch
	Bridge       CodeExecBridge
	EnvAllowlist []string
}

type codeExecOutput struct {
	OK              bool   `json:"ok"`
	Error           string `json:"error,omitempty"`
	PolicyViolation bool   `json:"policyViolation,omitempty"`
	ViolationType   string `json:"violationType,omitempty"`
	Result          any    `json:"result,omitempty"`
	ResultTruncated bool   `json:"resultTruncated,omitempty"`

	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	StdoutTruncated bool   `json:"stdoutTruncated,omitempty"`
	StderrTruncated bool   `json:"stderrTruncated,omitempty"`

	ToolCallCount int   `json:"toolCallCount"`
	RuntimeMs     int64 `json:"runtimeMs"`
	ExitCode      int   `json:"exitCode"`
}

type codeExecToolCallFrame struct {
	Type string          `json:"type"`
	ID   int             `json:"id"`
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
}

type codeExecFinalFrame struct {
	Type          string          `json:"type"`
	OK            bool            `json:"ok"`
	Error         string          `json:"error"`
	Result        json.RawMessage `json:"result"`
	Stdout        string          `json:"stdout"`
	Stderr        string          `json:"stderr"`
	ToolCallCount int             `json:"toolCallCount"`
	RuntimeMs     int64           `json:"runtimeMs"`
}

type codeExecFatalFrame struct {
	Type      string `json:"type"`
	Error     string `json:"error"`
	Traceback string `json:"traceback"`
}

type codeExecToolResultFrame struct {
	Type     string                `json:"type"`
	ID       int                   `json:"id"`
	OK       bool                  `json:"ok"`
	Error    string                `json:"error,omitempty"`
	Response *types.HostOpResponse `json:"response,omitempty"`
}

func (i *BuiltinCodeExecInvoker) runPython(parent context.Context, pythonBin, cwd string, cfg codeExecRunConfig) (codeExecOutput, error) {
	runCtx, cancel := context.WithTimeout(parent, time.Duration(cfg.TimeoutMs)*time.Millisecond)
	defer cancel()

	wrapperPath, cleanup, err := writeEmbeddedWrapper(codeExecPythonWrapper)
	if err != nil {
		return codeExecOutput{}, err
	}
	defer cleanup()

	cmd := exec.CommandContext(runCtx, pythonBin, wrapperPath)
	cmd.Dir = cwd
	cmd.Env = filteredCodeExecEnv(os.Environ(), cfg.EnvAllowlist)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return codeExecOutput{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return codeExecOutput{}, err
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return codeExecOutput{}, err
	}

	var procStderr bytes.Buffer
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		_, _ = io.Copy(&procStderr, stderrPipe)
	}()

	if err := cmd.Start(); err != nil {
		return codeExecOutput{}, err
	}

	enc := json.NewEncoder(stdinPipe)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{
		"type":           "init",
		"code":           cfg.Code,
		"allowed_tools":  cfg.Allowlist,
		"max_tool_calls": cfg.MaxToolCalls,
	}); err != nil {
		_ = stdinPipe.Close()
		_ = cmd.Wait()
		stderrWG.Wait()
		return codeExecOutput{}, err
	}

	var final *codeExecFinalFrame
	sc := bufio.NewScanner(stdoutPipe)
	buf := make([]byte, 0, 1024*64)
	sc.Buffer(buf, maxCodeExecMaxOutput*4)
	toolCalls := 0

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		payload, ok := codeExecFramePayload(line)
		if !ok {
			continue
		}
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &kind); err != nil {
			continue
		}
		switch kind.Type {
		case "tool_call":
			var call codeExecToolCallFrame
			if err := json.Unmarshal(payload, &call); err != nil {
				_ = enc.Encode(codeExecToolResultFrame{
					Type:  "tool_result",
					ID:    0,
					OK:    false,
					Error: "invalid tool_call frame: " + err.Error(),
				})
				continue
			}
			toolCalls++
			result := handleCodeExecToolCall(runCtx, call, toolCalls, cfg.MaxToolCalls, cfg.Allowlist, cfg.Dispatch, cfg.Bridge)
			if err := enc.Encode(result); err != nil {
				_ = stdinPipe.Close()
				_ = cmd.Wait()
				stderrWG.Wait()
				return codeExecOutput{}, err
			}
		case "final":
			frame := &codeExecFinalFrame{}
			if err := json.Unmarshal(payload, frame); err != nil {
				_ = stdinPipe.Close()
				_ = cmd.Wait()
				stderrWG.Wait()
				return codeExecOutput{}, fmt.Errorf("invalid final frame: %w", err)
			}
			final = frame
		case "fatal":
			frame := codeExecFatalFrame{}
			if err := json.Unmarshal(payload, &frame); err == nil {
				_ = stdinPipe.Close()
				_ = cmd.Wait()
				stderrWG.Wait()
				return codeExecOutput{}, fmt.Errorf("python wrapper fatal: %s", strings.TrimSpace(frame.Error))
			}
		}
	}
	if scanErr := sc.Err(); scanErr != nil {
		_ = stdinPipe.Close()
		_ = cmd.Wait()
		stderrWG.Wait()
		return codeExecOutput{}, scanErr
	}

	_ = stdinPipe.Close()
	waitErr := cmd.Wait()
	stderrWG.Wait()
	if waitErr != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return codeExecOutput{}, context.DeadlineExceeded
	}
	if final == nil {
		msg := strings.TrimSpace(procStderr.String())
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		if msg == "" {
			msg = "python wrapper exited without final frame"
		}
		return codeExecOutput{}, errors.New(msg)
	}

	out := codeExecOutput{
		OK:            final.OK,
		Error:         strings.TrimSpace(final.Error),
		ToolCallCount: final.ToolCallCount,
		RuntimeMs:     final.RuntimeMs,
		ExitCode:      0,
	}
	if !out.OK {
		out.ExitCode = 1
		if out.Error == "" {
			out.Error = "code_exec failed"
		}
	}
	if violationType, cleaned := parseCodeExecPolicyViolation(out.Error); violationType != "" {
		out.PolicyViolation = true
		out.ViolationType = violationType
		out.Error = cleaned
	}

	half := cfg.MaxOutput / 2
	if half <= 0 {
		half = cfg.MaxOutput
	}
	out.Stdout, out.StdoutTruncated = capBytes(final.Stdout, half)
	out.Stderr, out.StderrTruncated = capBytes(final.Stderr, half)
	out.Error = appendCodeExecMissingModuleHint(out.Error, out.Stderr)

	if len(final.Result) != 0 && string(final.Result) != "null" {
		var decoded any
		if err := json.Unmarshal(final.Result, &decoded); err != nil {
			decoded = string(final.Result)
		}
		encoded, err := json.Marshal(decoded)
		if err != nil {
			decoded = fmt.Sprintf("%v", decoded)
			encoded = []byte(decoded.(string))
		}
		if cfg.MaxOutput > 0 && len(encoded) > cfg.MaxOutput {
			out.ResultTruncated = true
			out.Result = string(encoded[:cfg.MaxOutput]) + " ...[truncated]"
		} else {
			out.Result = decoded
		}
	}
	return out, nil
}

func appendCodeExecMissingModuleHint(errMsg, stderr string) string {
	msg := strings.TrimSpace(errMsg)
	stderr = strings.TrimSpace(stderr)
	combined := strings.ToLower(msg + "\n" + stderr)
	if !strings.Contains(combined, "modulenotfounderror") && !strings.Contains(combined, "no module named") {
		return msg
	}
	hint := "Missing Python module. Add package to [code_exec].required_packages in config.toml; daemon auto-reconciles on save."
	if msg == "" {
		return hint
	}
	if strings.Contains(strings.ToLower(msg), strings.ToLower(hint)) {
		return msg
	}
	return msg + " " + hint
}

func parseCodeExecPolicyViolation(errMsg string) (violationType, cleanedError string) {
	msg := strings.TrimSpace(errMsg)
	if msg == "" {
		return "", ""
	}
	const prefix = "policy_violation:"
	if !strings.HasPrefix(msg, prefix) {
		return "", msg
	}
	rest := strings.TrimSpace(strings.TrimPrefix(msg, prefix))
	if rest == "" {
		return "", msg
	}
	parts := strings.SplitN(rest, ":", 2)
	violationType = strings.TrimSpace(parts[0])
	if violationType == "" {
		return "", msg
	}
	cleanedError = msg
	if len(parts) == 2 {
		if text := strings.TrimSpace(parts[1]); text != "" {
			cleanedError = text
		}
	}
	return violationType, cleanedError
}

func handleCodeExecToolCall(
	ctx context.Context,
	call codeExecToolCallFrame,
	toolCalls int,
	maxToolCalls int,
	allowlist []string,
	dispatch CodeExecDispatch,
	bridge CodeExecBridge,
) codeExecToolResultFrame {
	if call.ID <= 0 {
		return codeExecToolResultFrame{Type: "tool_result", ID: call.ID, OK: false, Error: "invalid call id"}
	}
	tool := strings.TrimSpace(call.Tool)
	if tool == "" {
		return codeExecToolResultFrame{Type: "tool_result", ID: call.ID, OK: false, Error: "tool name is required"}
	}
	if toolCalls > maxToolCalls {
		return codeExecToolResultFrame{Type: "tool_result", ID: call.ID, OK: false, Error: fmt.Sprintf("max tool calls exceeded (%d)", maxToolCalls)}
	}
	if tool == "code_exec" {
		return codeExecToolResultFrame{Type: "tool_result", ID: call.ID, OK: false, Error: "nested code_exec is not allowed"}
	}
	if !containsToolName(allowlist, tool) {
		return codeExecToolResultFrame{Type: "tool_result", ID: call.ID, OK: false, Error: "tool is not allowlisted for code_exec: " + tool}
	}

	opReq, err := dispatch(ctx, tool, call.Args)
	if err != nil {
		return codeExecToolResultFrame{Type: "tool_result", ID: call.ID, OK: false, Error: "dispatch tool call: " + err.Error()}
	}
	if strings.TrimSpace(opReq.Op) == types.HostOpCodeExec {
		return codeExecToolResultFrame{Type: "tool_result", ID: call.ID, OK: false, Error: "nested code_exec is not allowed"}
	}
	opReq.Tag = "code_exec_bridge"
	resp := bridge(ctx, opReq)
	if !resp.Ok {
		return codeExecToolResultFrame{
			Type:     "tool_result",
			ID:       call.ID,
			OK:       false,
			Error:    strings.TrimSpace(resp.Error),
			Response: &resp,
		}
	}
	return codeExecToolResultFrame{
		Type:     "tool_result",
		ID:       call.ID,
		OK:       true,
		Response: &resp,
	}
}

func containsToolName(allowlist []string, tool string) bool {
	for _, it := range allowlist {
		if strings.TrimSpace(it) == tool {
			return true
		}
	}
	return false
}

func writeEmbeddedWrapper(contents string) (string, func(), error) {
	f, err := os.CreateTemp("", "agen8-code-exec-*.py")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	if _, err := f.WriteString(contents); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(path) }
	return path, cleanup, nil
}

func resolveCodeExecCwd(rootDir string, mountRoots map[string]string, cwd string) (absDir string, logical string, err error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || cwd == "." {
		cwd = defaultCodeExecCwd
	}
	if strings.HasPrefix(cwd, "/") {
		trimmed := strings.TrimPrefix(filepath.Clean(cwd), "/")
		if trimmed == "" || strings.HasPrefix(trimmed, "..") {
			return "", "", fmt.Errorf("invalid cwd %q", cwd)
		}
		parts := strings.SplitN(trimmed, string(filepath.Separator), 2)
		mount := strings.TrimSpace(parts[0])
		root := strings.TrimSpace(mountRoots[mount])
		if root == "" {
			return "", "", fmt.Errorf("cwd mount %q is not available", mount)
		}
		rest := ""
		if len(parts) == 2 {
			rest = parts[1]
		}
		target := filepath.Join(root, rest)
		if !isWithinRoot(target, root) {
			return "", "", fmt.Errorf("cwd %q escapes mount root", cwd)
		}
		return target, cwd, nil
	}
	base := strings.TrimSpace(rootDir)
	if base == "" {
		return "", "", fmt.Errorf("rootDir is required for relative cwd")
	}
	target := filepath.Join(base, cwd)
	if !isWithinRoot(target, base) {
		return "", "", fmt.Errorf("cwd %q escapes root", cwd)
	}
	return target, cwd, nil
}

func isWithinRoot(candidate, root string) bool {
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func filteredCodeExecEnv(env []string, allowlist []string) []string {
	keys := map[string]struct{}{}
	for _, k := range allowlist {
		k = strings.TrimSpace(k)
		if k != "" {
			keys[k] = struct{}{}
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AGEN8_CODE_EXEC_ENV_ALLOWLIST")); raw != "" {
		for _, k := range strings.Split(raw, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				keys[k] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(keys))
	for _, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, ok := keys[strings.TrimSpace(key)]; ok {
			out = append(out, kv)
		}
	}
	sort.Strings(out)
	return out
}

func defaultCodeExecEnvAllowlist() []string {
	return []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "PYTHONPATH", "PYTHONHOME"}
}

func capBytes(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	return s[:max], true
}

func codeExecFramePayload(line string) ([]byte, bool) {
	if !strings.HasPrefix(line, codeExecFramePrefix) {
		return nil, false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, codeExecFramePrefix))
	if payload == "" {
		return nil, false
	}
	return []byte(payload), true
}

func requiredCodeExecImports(extraImports []string) []string {
	base := []string{"json", "re", "io", "contextlib"}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(base))
	for _, mod := range base {
		mod = strings.TrimSpace(mod)
		if mod == "" {
			continue
		}
		if _, ok := seen[mod]; ok {
			continue
		}
		seen[mod] = struct{}{}
		out = append(out, mod)
	}
	for _, mod := range extraImports {
		mod = strings.TrimSpace(mod)
		if mod == "" {
			continue
		}
		if _, ok := seen[mod]; ok {
			continue
		}
		seen[mod] = struct{}{}
		out = append(out, mod)
	}
	sort.Strings(out)
	return out
}

func normalizeCodeExecImports(imports []string) []string {
	if len(imports) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(imports))
	for _, mod := range imports {
		mod = strings.TrimSpace(mod)
		if mod == "" {
			continue
		}
		if _, ok := seen[mod]; ok {
			continue
		}
		seen[mod] = struct{}{}
		out = append(out, mod)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func runCodeExecWrapperPreflight(parent context.Context, pythonBin string, envAllow []string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	wrapperPath, cleanup, err := writeEmbeddedWrapper(codeExecPythonWrapper)
	if err != nil {
		return fmt.Errorf("code_exec preflight: write wrapper: %w", err)
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, pythonBin, wrapperPath)
	cmd.Env = filteredCodeExecEnv(os.Environ(), envAllow)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("code_exec preflight: stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("code_exec preflight: stderr pipe: %w", err)
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("code_exec preflight: stdin pipe: %w", err)
	}

	var procStderr bytes.Buffer
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		_, _ = io.Copy(&procStderr, stderrPipe)
	}()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("code_exec preflight: start wrapper: %w", err)
	}
	enc := json.NewEncoder(stdinPipe)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{
		"type":           "init",
		"code":           "result = {'ready': True}",
		"allowed_tools":  []string{},
		"max_tool_calls": 1,
	}); err != nil {
		_ = stdinPipe.Close()
		_ = cmd.Wait()
		stderrWG.Wait()
		return fmt.Errorf("code_exec preflight: init frame: %w", err)
	}

	sc := bufio.NewScanner(stdoutPipe)
	sc.Buffer(make([]byte, 0, 64*1024), maxCodeExecMaxOutput*4)
	finalSeen := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		payload, ok := codeExecFramePayload(line)
		if !ok {
			continue
		}
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &kind); err != nil {
			continue
		}
		switch kind.Type {
		case "fatal":
			var frame codeExecFatalFrame
			if err := json.Unmarshal(payload, &frame); err == nil {
				_ = stdinPipe.Close()
				_ = cmd.Wait()
				stderrWG.Wait()
				return fmt.Errorf("code_exec preflight: wrapper fatal: %s", strings.TrimSpace(frame.Error))
			}
		case "final":
			var frame codeExecFinalFrame
			if err := json.Unmarshal(payload, &frame); err != nil {
				_ = stdinPipe.Close()
				_ = cmd.Wait()
				stderrWG.Wait()
				return fmt.Errorf("code_exec preflight: invalid final frame: %w", err)
			}
			if !frame.OK {
				_ = stdinPipe.Close()
				_ = cmd.Wait()
				stderrWG.Wait()
				return fmt.Errorf("code_exec preflight: wrapper execution failed: %s", strings.TrimSpace(frame.Error))
			}
			finalSeen = true
		}
		if finalSeen {
			break
		}
	}
	_ = stdinPipe.Close()
	waitErr := cmd.Wait()
	stderrWG.Wait()
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("code_exec preflight: wrapper handshake timed out")
		}
		return fmt.Errorf("code_exec preflight: wrapper exited with error: %w (%s)", waitErr, trimmedOutput(procStderr.String()))
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("code_exec preflight: scan wrapper output: %w", err)
	}
	if !finalSeen {
		return fmt.Errorf("code_exec preflight: wrapper did not emit final frame (%s)", trimmedOutput(procStderr.String()))
	}
	return nil
}

func trimmedOutput(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no output"
	}
	if len(s) > 240 {
		return s[:239] + "…"
	}
	return s
}
