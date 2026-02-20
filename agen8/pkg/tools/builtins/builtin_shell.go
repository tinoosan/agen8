package builtins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	pkgtools "github.com/tinoosan/agen8/pkg/tools"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfsutil"
)

const defaultShellMaxOutputBytes = 64 * 1024

// BuiltinShellInvoker runs a guarded argv under the host workdir.
type BuiltinShellInvoker struct {
	RootDir      string
	VFSMountName string
	MountRoots   map[string]string // mount name -> absolute host root (e.g. "skills" -> ~/.agents/skills)
	Deny         map[string]bool
	MaxBytes     int
	// EnableVFSPathTranslation translates known VFS absolute paths in argv/commands.
	EnableVFSPathTranslation bool
	// EnableScriptPathMitigation controls script anti-pattern normalization/hints in shell_exec.
	EnableScriptPathMitigation bool
	// MaxScriptMitigationRetries limits retry attempts after a script-path normalization.
	MaxScriptMitigationRetries int
	Confirm                    func(ctx context.Context, argv []string, cwd string) (bool, error)
}

func NewBuiltinShellInvoker(rootDir string, confirm func(context.Context, []string, string) (bool, error), vfsMount string) *BuiltinShellInvoker {
	return &BuiltinShellInvoker{
		RootDir:      rootDir,
		VFSMountName: strings.TrimSpace(vfsMount),
		MountRoots: map[string]string{
			strings.TrimSpace(vfsMount): rootDir,
		},
		Deny:                       DefaultShellDenylist(),
		MaxBytes:                   defaultShellMaxOutputBytes,
		EnableVFSPathTranslation:   true,
		EnableScriptPathMitigation: true,
		MaxScriptMitigationRetries: 1,
		Confirm:                    confirm,
	}
}

func DefaultShellDenylist() map[string]bool {
	return map[string]bool{
		"sudo": true, "su": true, "doas": true,
		"ssh": true, "scp": true, "sftp": true, "rsync": true,
		"nc": true, "ncat": true, "netcat": true, "socat": true, "telnet": true,
		"shutdown": true, "reboot": true, "halt": true, "poweroff": true, "dd": true, "mkfs": true, "mount": true, "umount": true,
		"systemctl": true, "launchctl": true,
		"rm": true,
	}
}

type shellExecInput struct {
	Argv  []string `json:"argv"`
	Cwd   string   `json:"cwd,omitempty"`
	Stdin string   `json:"stdin,omitempty"`
}

type shellExecOutput struct {
	ExitCode             int    `json:"exitCode"`
	Stdout               string `json:"stdout"`
	Stderr               string `json:"stderr"`
	Warning              string `json:"warning,omitempty"`
	VFSPathTranslated    bool   `json:"vfsPathTranslated,omitempty"`
	VFSPathMounts        string `json:"vfsPathMounts,omitempty"`
	ScriptPathNormalized bool   `json:"scriptPathNormalized,omitempty"`
	ScriptAntiPattern    string `json:"scriptAntiPattern,omitempty"`
}

func (s *BuiltinShellInvoker) Invoke(ctx context.Context, req pkgtools.ToolRequest) (pkgtools.ToolCallResult, error) {
	if s == nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "builtin.shell invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "exec" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: exec)", req.ActionID)}
	}

	root := strings.TrimSpace(s.RootDir)
	if root == "" || !filepath.IsAbs(root) {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "rootDir must be absolute"}
	}

	maxBytes := s.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultShellMaxOutputBytes
	}

	var in shellExecInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	if len(in.Argv) == 0 {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "argv is required"}
	}

	cmdName := in.Argv[0]
	if strings.Contains(cmdName, "/") || strings.Contains(cmdName, string(filepath.Separator)) {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "command must be a bare executable name (no path separators)"}
	}
	if s.Deny != nil && s.Deny[cmdName] {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("command %q is denied", cmdName)}
	}

	cwd := strings.TrimSpace(in.Cwd)
	absDir, logicalCwd, err := s.resolveCwd(root, cwd)
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if st, err := os.Stat(absDir); err != nil || !st.IsDir() {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("cwd %q is not a directory", logicalCwd)}
	}

	if s.Confirm != nil {
		ok, err := s.Confirm(ctx, in.Argv, logicalCwd)
		if err != nil {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
		}
		if !ok {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{
				Code:    types.CommandRejectedErrorCode,
				Message: types.CommandRejectedErrorMessage,
			}
		}
	}

	argv := append([]string(nil), in.Argv...)
	vfsTranslated := false
	vfsMounts := []string(nil)
	if s.EnableVFSPathTranslation {
		var changed bool
		argv, changed, vfsMounts = translateVFSArgs(argv, s.mountRoots())
		vfsTranslated = changed
	}
	normalized := false
	antiPattern := "none"
	warning := ""
	if s.EnableScriptPathMitigation {
		normArgv, changed, anti, note := normalizeScriptInvocationArgs(argv, logicalCwd)
		if changed {
			argv = normArgv
			normalized = true
			if anti != "" {
				antiPattern = anti
			}
			warning = note
		}
	}

	for i := 1; i < len(argv); i++ {
		if looksLikeAbsPathOrFlagValue(argv[i]) {
			if isAllowedAbsolutePathArg(argv[i], s.mountRoots()) {
				continue
			}
			msg := fmt.Sprintf("absolute paths are not allowed in argv (got %q); use relative paths or known VFS mounts (/project, /workspace, /skills, /plan, /memory)", argv[i])
			if hint, anti := scriptPathHint(logicalCwd, argv, msg); hint != "" {
				msg += "\n" + hint
				if antiPattern == "none" && anti != "" {
					antiPattern = anti
				}
			}
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: msg}
		}
	}
	if len(argv) >= 3 && argv[0] == "bash" && argv[1] == "-c" {
		if bad := firstDisallowedAbsPathInShell(argv[2], s.mountRoots()); bad != "" {
			msg := fmt.Sprintf("absolute path %q is not allowed in command; use known VFS mounts (/project, /workspace, /skills, /plan, /memory)", bad)
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: msg}
		}
	}

	stdoutText, stderrText, exitCode, runErr := runShellCommand(ctx, cmdName, argv[1:], absDir, in.Stdin)
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "timeout", Message: "command timed out", Retryable: true, Err: runErr}
		}
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: runErr.Error(), Err: runErr}
	}

	if exitCode != 0 && s.EnableScriptPathMitigation && !normalized && s.MaxScriptMitigationRetries > 0 {
		retryArgv, changed, retryAnti, retryNote := normalizeFromFailure(argv, logicalCwd, stderrText)
		if changed {
			retryStdout, retryStderr, retryCode, retryErr := runShellCommand(ctx, cmdName, retryArgv[1:], absDir, in.Stdin)
			if retryErr == nil {
				argv = retryArgv
				normalized = true
				if retryAnti != "" {
					antiPattern = retryAnti
				}
				warning = retryNote
				stdoutText = retryStdout
				stderrText = retryStderr
				exitCode = retryCode
			}
		}
	}

	stdoutText = s.translateOutputPaths(stdoutText)
	stderrText = s.translateOutputPaths(stderrText)
	if hint, anti := scriptPathHint(logicalCwd, argv, stderrText); hint != "" {
		if !strings.Contains(stderrText, hint) {
			if strings.TrimSpace(stderrText) != "" {
				stderrText += "\n"
			}
			stderrText += hint
		}
		if antiPattern == "none" && anti != "" {
			antiPattern = anti
		}
	}

	if antiPattern == "" {
		antiPattern = "none"
	}
	if normalized && strings.TrimSpace(warning) == "" {
		warning = fmt.Sprintf("Agen8 normalized script invocation (%s).", antiPattern)
	}

	out := shellExecOutput{
		ExitCode:             exitCode,
		Stdout:               truncateString(stdoutText, maxBytes),
		Stderr:               truncateString(stderrText, maxBytes),
		Warning:              warning,
		VFSPathTranslated:    vfsTranslated,
		VFSPathMounts:        strings.Join(vfsMounts, ","),
		ScriptPathNormalized: normalized,
		ScriptAntiPattern:    antiPattern,
	}

	outputJSON, err := json.Marshal(out)
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}

	return pkgtools.ToolCallResult{Output: outputJSON}, nil
}

func filterShellEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if isShellSensitiveEnvKey(k) {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

func isShellSensitiveEnvKey(k string) bool {
	upper := strings.ToUpper(k)
	sensitivePrefixes := []string{
		"AWS_",
		"SECRET",
		"TOKEN",
		"KEY",
		"PASS",
		"OPENROUTER_",
	}
	for _, p := range sensitivePrefixes {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}

func looksLikeAbsPath(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	if strings.HasPrefix(s, "/") {
		return true
	}
	if filepath.IsAbs(s) {
		return true
	}
	return false
}

func looksLikeAbsPathOrFlagValue(s string) bool {
	if looksLikeAbsPath(s) {
		return true
	}
	_, v, ok := strings.Cut(s, "=")
	if ok && looksLikeAbsPath(v) {
		return true
	}
	return false
}

func truncateString(s string, max int) string {
	if max <= 0 {
		return s
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (s *BuiltinShellInvoker) translateOutputPaths(text string) string {
	out := text
	for mount, root := range s.mountRoots() {
		mount = strings.TrimSpace(mount)
		root = strings.TrimSpace(root)
		if mount == "" || root == "" {
			continue
		}
		hostRoot := filepath.Clean(root)
		prefix := "/" + mount
		out = strings.ReplaceAll(out, hostRoot, prefix)
		if slashRoot := filepath.ToSlash(hostRoot); slashRoot != hostRoot {
			out = strings.ReplaceAll(out, slashRoot, prefix)
		}
	}
	return out
}

func (s *BuiltinShellInvoker) resolveCwd(projectRoot, cwd string) (string, string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = "."
	}
	if strings.HasPrefix(cwd, "/") {
		path := filepath.ToSlash(filepath.Clean(cwd))
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
			return "", cwd, fmt.Errorf("invalid cwd %q", cwd)
		}
		mount := strings.TrimSpace(parts[0])
		root, ok := s.mountRoots()[mount]
		if !ok || strings.TrimSpace(root) == "" {
			return "", cwd, fmt.Errorf("cwd mount %q is not available; use one of /%s", mount, strings.Join(sortedMountKeys(s.mountRoots()), ", /"))
		}
		sub := "."
		if len(parts) > 1 {
			sub = filepath.ToSlash(filepath.Join(parts[1:]...))
		}
		absDir, err := vfsutil.SafeJoinBaseDir(root, sub)
		if err != nil {
			return "", cwd, err
		}
		return absDir, path, nil
	}
	absDir, err := vfsutil.SafeJoinBaseDir(projectRoot, cwd)
	if err != nil {
		return "", cwd, err
	}
	return absDir, cwd, nil
}

func (s *BuiltinShellInvoker) mountRoots() map[string]string {
	out := make(map[string]string)
	for k, v := range s.MountRoots {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		mount := strings.TrimSpace(s.VFSMountName)
		root := strings.TrimSpace(s.RootDir)
		if mount != "" && root != "" {
			out[mount] = root
		}
	}
	return out
}

func sortedMountKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func translateVFSArgs(argv []string, mountRoots map[string]string) ([]string, bool, []string) {
	if len(argv) == 0 {
		return argv, false, nil
	}
	out := append([]string(nil), argv...)
	used := map[string]struct{}{}
	changed := false

	for i := 1; i < len(out); i++ {
		orig := out[i]
		next := orig
		if len(out) >= 3 && out[0] == "bash" && out[1] == "-c" && i == 2 {
			next = translateVFSShellString(next, mountRoots, used)
		} else {
			next = translateVFSToken(next, mountRoots, used)
		}
		if next != orig {
			out[i] = next
			changed = true
		}
	}
	if !changed {
		return argv, false, nil
	}
	mounts := make([]string, 0, len(used))
	for m := range used {
		mounts = append(mounts, m)
	}
	sort.Strings(mounts)
	return out, true, mounts
}

func translateVFSToken(token string, mountRoots map[string]string, used map[string]struct{}) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return token
	}
	if k, v, ok := strings.Cut(token, "="); ok && looksLikeAbsPath(v) {
		return k + "=" + translateVFSAbsPath(v, mountRoots, used)
	}
	if looksLikeAbsPath(token) {
		return translateVFSAbsPath(token, mountRoots, used)
	}
	return token
}

func translateVFSShellString(cmd string, mountRoots map[string]string, used map[string]struct{}) string {
	out := cmd
	for _, mount := range sortedMountKeys(mountRoots) {
		root := strings.TrimSpace(mountRoots[mount])
		if root == "" {
			continue
		}
		base := "/" + mount
		re := regexp.MustCompile(`(^|[\s"'` + "`" + `=|&;()<>])` + regexp.QuoteMeta(base) + `(?:/[^\s"'` + "`" + `|&;()<>]*)?`)
		out = re.ReplaceAllStringFunc(out, func(match string) string {
			prefix := ""
			path := match
			if len(match) > 0 {
				first := match[:1]
				if strings.Contains(" \t\r\n\"'`=|&;()<>", first) {
					prefix = first
					path = match[1:]
				}
			}
			repl := translateVFSAbsPath(path, mountRoots, used)
			return prefix + repl
		})
	}
	return out
}

func translateVFSAbsPath(path string, mountRoots map[string]string, used map[string]struct{}) string {
	p := filepath.ToSlash(strings.TrimSpace(path))
	for _, mount := range sortedMountKeys(mountRoots) {
		root := strings.TrimSpace(mountRoots[mount])
		if root == "" {
			continue
		}
		base := "/" + mount
		if p == base {
			used[mount] = struct{}{}
			return root
		}
		if strings.HasPrefix(p, base+"/") {
			used[mount] = struct{}{}
			rel := strings.TrimPrefix(p, base+"/")
			return filepath.Join(root, filepath.FromSlash(rel))
		}
	}
	return path
}

func isAllowedAbsolutePathArg(arg string, mountRoots map[string]string) bool {
	val := strings.TrimSpace(arg)
	if _, v, ok := strings.Cut(val, "="); ok && looksLikeAbsPath(v) {
		val = v
	}
	if !looksLikeAbsPath(val) {
		return true
	}
	clean := filepath.Clean(val)
	for _, root := range mountRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func firstDisallowedAbsPathInShell(cmd string, mountRoots map[string]string) string {
	re := regexp.MustCompile(`(^|[\s"'` + "`" + `=|&;()<>])(/[^ \t\r\n"'` + "`" + `|&;()<>]+)`)
	matches := re.FindAllStringSubmatch(cmd, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		candidate := strings.TrimSpace(m[2])
		if candidate == "" || !strings.HasPrefix(candidate, "/") {
			continue
		}
		if isAllowedAbsolutePathArg(candidate, mountRoots) {
			continue
		}
		if isAllowedSystemAbsolutePath(candidate) {
			continue
		}
		return candidate
	}
	return ""
}

func isAllowedSystemAbsolutePath(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || !strings.HasPrefix(path, "/") {
		return false
	}
	// Allow common device paths used in redirects.
	if path == "/dev/null" || strings.HasPrefix(path, "/dev/fd/") || path == "/dev/stdin" || path == "/dev/stdout" || path == "/dev/stderr" {
		return true
	}
	// Allow absolute executable invocations (e.g. /bin/bash, /usr/bin/env).
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Mode().Perm()&0o111 != 0
}

func runShellCommand(ctx context.Context, cmdName string, args []string, absDir, stdin string) (stdout string, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = absDir
	cmd.Env = filterShellEnv(os.Environ())
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode = 0
	if runErr := cmd.Run(); runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return "", "", 0, runErr
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), exitCode, nil
}

func normalizeScriptInvocationArgs(argv []string, logicalCwd string) ([]string, bool, string, string) {
	if len(argv) == 0 {
		return argv, false, "none", ""
	}
	if !isSkillScriptsCwd(logicalCwd) {
		return argv, false, "none", ""
	}
	out := append([]string(nil), argv...)
	changed := false
	anti := "none"

	for i := 1; i < len(out); i++ {
		v := out[i]
		nv, ch, a := normalizeArgToken(v, logicalCwd)
		if ch {
			out[i] = nv
			changed = true
			if anti == "none" && a != "" {
				anti = a
			}
		}
	}
	if len(out) >= 3 && out[0] == "bash" && out[1] == "-c" {
		nc, ch, a := normalizeShellCommand(out[2], logicalCwd)
		if ch {
			out[2] = nc
			changed = true
			if anti == "none" && a != "" {
				anti = a
			}
		}
	}
	if !changed {
		return argv, false, anti, ""
	}
	if anti == "none" {
		anti = "duplicate_scripts_prefix"
	}
	note := fmt.Sprintf("Agen8 normalized script invocation (%s): use cwd=\"/skills/<skill>/scripts\" and invoke scripts by basename.", anti)
	return out, true, anti, note
}

func normalizeArgToken(v, logicalCwd string) (string, bool, string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return v, false, ""
	}
	base := skillsScriptsBase(logicalCwd)
	if base != "" && strings.HasPrefix(v, base+"/") {
		return strings.TrimPrefix(v, base+"/"), true, "absolute_skills_path"
	}
	if strings.HasPrefix(v, "scripts/") {
		return strings.TrimPrefix(v, "scripts/"), true, "duplicate_scripts_prefix"
	}
	return v, false, ""
}

func normalizeShellCommand(cmd, logicalCwd string) (string, bool, string) {
	orig := cmd
	anti := ""

	base := regexp.QuoteMeta(skillsScriptsBase(logicalCwd) + "/")
	if skillsScriptsBase(logicalCwd) != "" {
		reAbs := regexp.MustCompile(`(^|[\s="'` + "`" + `])` + base + `([^\s"'` + "`" + `]+)`)
		cmd = reAbs.ReplaceAllString(cmd, `${1}${2}`)
		if cmd != orig {
			anti = "absolute_skills_path"
		}
	}

	before := cmd
	reDup := regexp.MustCompile(`(^|[\s="'` + "`" + `])scripts/([^\s"'` + "`" + `]+)`)
	cmd = reDup.ReplaceAllString(cmd, `${1}${2}`)
	if cmd != before && anti == "" {
		anti = "duplicate_scripts_prefix"
	}
	return cmd, cmd != orig, anti
}

func scriptPathHint(logicalCwd string, argv []string, stderr string) (string, string) {
	if !isSkillScriptsCwd(logicalCwd) {
		return "", ""
	}
	errText := strings.ToLower(strings.TrimSpace(stderr))
	if errText == "" {
		return "", ""
	}
	if !strings.Contains(errText, "no such file") && !strings.Contains(errText, "can't open file") && !strings.Contains(errText, "absolute paths are not allowed") {
		return "", ""
	}
	anti := "none"
	base := skillsScriptsBase(logicalCwd)
	for _, a := range argv {
		if strings.Contains(a, "/skills/") || (base != "" && strings.Contains(a, base+"/")) {
			anti = "absolute_skills_path"
			break
		}
		if strings.Contains(a, "scripts/") {
			anti = "duplicate_scripts_prefix"
		}
	}
	hint := "script_hint: when cwd is /skills/<skill>/scripts, invoke by basename (e.g., `python3 csv_validate.py --help`), not `/skills/...` or `scripts/...`."
	return hint, anti
}

func normalizeFromFailure(argv []string, logicalCwd, stderr string) ([]string, bool, string, string) {
	// First try the deterministic normalization pass.
	if out, changed, anti, note := normalizeScriptInvocationArgs(argv, logicalCwd); changed {
		return out, changed, anti, note
	}
	if len(argv) < 3 || argv[0] != "bash" || argv[1] != "-c" {
		return argv, false, "", ""
	}
	re := regexp.MustCompile(`(/[^\s"'` + "`" + `]+/scripts/([^\s"'` + "`" + `]+))`)
	m := re.FindStringSubmatch(stderr)
	if len(m) != 3 {
		return argv, false, "", ""
	}
	needle := strings.TrimSpace(m[1])
	base := strings.TrimSpace(m[2])
	if needle == "" || base == "" {
		return argv, false, "", ""
	}
	cmd := argv[2]
	ncmd := strings.ReplaceAll(cmd, needle, base)
	if ncmd == cmd {
		return argv, false, "", ""
	}
	out := append([]string(nil), argv...)
	out[2] = ncmd
	note := "Agen8 normalized script invocation from failure diagnostics (absolute_skills_path): use cwd=\"/skills/<skill>/scripts\" and invoke scripts by basename."
	return out, true, "absolute_skills_path", note
}

func isSkillScriptsCwd(logicalCwd string) bool {
	path := filepath.ToSlash(strings.TrimSpace(logicalCwd))
	if path == "" {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) == 3 && parts[0] == "skills" && parts[2] == "scripts" && strings.TrimSpace(parts[1]) != ""
}

func skillsScriptsBase(logicalCwd string) string {
	if !isSkillScriptsCwd(logicalCwd) {
		return ""
	}
	path := filepath.ToSlash(strings.TrimSpace(logicalCwd))
	return path
}
