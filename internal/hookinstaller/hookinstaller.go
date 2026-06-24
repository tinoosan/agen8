// Package hookinstaller provisions the harness attention hooks: the curl
// one-liners that tell agen8 when an agent is waiting on the human (see
// internal/services/attention). One explicit `agen8 hooks install` run per
// machine/harness — never installed silently.
//
// The installed hook is deliberately a bare curl pipe: the daemon normalizes
// the harness's raw payload server-side (POST /hooks/attention?harness=&kind=),
// so the hook has zero local dependencies and works identically against a
// local or hosted agen8.
package hookinstaller

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Harness selects which harness's hook config to write.
type Harness string

const (
	HarnessClaude Harness = "claude"
	HarnessCodex  Harness = "codex"
)

// hookMarker identifies agen8's own entries inside a shared hooks file, so
// re-installs replace them and never touch hooks the user wrote themselves.
const hookMarker = "/hooks/attention"

// Options configures Install.
type Options struct {
	Harness Harness
	// BaseURL of the agen8 daemon, e.g. http://127.0.0.1:7777 or a hosted URL.
	BaseURL string
	// Token is the bearer credential the hook authenticates with (an MCP API
	// key ak_... or equivalent).
	Token string
	// ProjectDir is where Claude Code's project-scoped settings live; defaults
	// to the working directory. Claude config goes to .claude/settings.local.json
	// specifically so the embedded token stays out of version control.
	ProjectDir string
	// HomeDir overrides the home directory for Codex's user-level config
	// (~/.codex/hooks.json). Defaults to the OS home dir.
	HomeDir string
}

// Result reports what Install did.
type Result struct {
	Harness Harness
	// Path of the hooks file written.
	Path string
}

// claudeHookEvents maps Claude Code hook events to attention kinds. Which hook
// fired tells us everything — the payload itself is passed through raw.
//
//   - Notification(idle_prompt): the agent finished and is waiting at the prompt.
//   - Notification(permission_prompt): blocked on a tool approval.
//   - Stop: turn ended — same "waiting for the human" state, fires immediately.
//   - Pre/PostToolUse(AskUserQuestion): the agent posed an interactive question
//     and the human answered it. Mapped payload-free (kind only, no question
//     text) so Claude Code and Codex stay at parity.
//   - UserPromptSubmit / SessionEnd: the human engaged or the session is gone.
var claudeHookEvents = []hookSpec{
	{Event: "Notification", Matcher: "idle_prompt", Kind: "waiting"},
	{Event: "Notification", Matcher: "permission_prompt", Kind: "needs_approval"},
	{Event: "PreToolUse", Matcher: "AskUserQuestion", Kind: "asking"},
	{Event: "PostToolUse", Matcher: "AskUserQuestion", Kind: "cleared"},
	// Heartbeat: a completed Bash call proves the agent is running, clearing a
	// stale needs_approval (nothing fires when the human approves a tool) or
	// waiting. Tight timeout — this fires often, so a down agen8 may cost at
	// most ~1s per Bash call, never more.
	{Event: "PostToolUse", Matcher: "Bash", Kind: "cleared", TimeoutSecs: 1},
	{Event: "Stop", Matcher: "", Kind: "waiting"},
	{Event: "UserPromptSubmit", Matcher: "", Kind: "cleared"},
	{Event: "SessionEnd", Matcher: "", Kind: "cleared"},
}

// codexHookEvents maps Codex lifecycle hooks to attention kinds. Codex has no
// Notification or SessionEnd events (verified against codex-rs source): Stop is
// the end-of-turn "now waiting" signal, and SessionStart doubles as a clear —
// a fresh/resumed session means any previous wait is over.
var codexHookEvents = []hookSpec{
	{Event: "Stop", Matcher: "", Kind: "waiting"},
	{Event: "PermissionRequest", Matcher: "", Kind: "needs_approval"},
	// Heartbeat — see the claude PostToolUse(Bash) entry.
	{Event: "PostToolUse", Matcher: "Bash", Kind: "cleared", TimeoutSecs: 1},
	{Event: "UserPromptSubmit", Matcher: "", Kind: "cleared"},
	{Event: "SessionStart", Matcher: "", Kind: "cleared"},
}

type hookSpec struct {
	Event   string
	Matcher string
	Kind    string
	// TimeoutSecs overrides the curl -m timeout (default 3). Use a tight value
	// for high-frequency hooks so a down agen8 cannot meaningfully slow agents.
	TimeoutSecs int
}

// Install provisions the attention hooks for one harness.
func Install(opts Options) (Result, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		return Result{}, fmt.Errorf("hooks install: --url is required (e.g. http://127.0.0.1:7777)")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return Result{}, fmt.Errorf("hooks install: --token is required (an agen8 API key, ak_...)")
	}
	switch opts.Harness {
	case HarnessClaude:
		return installClaude(opts, baseURL)
	case HarnessCodex:
		return installCodex(opts, baseURL)
	default:
		return Result{}, fmt.Errorf("hooks install: unknown harness %q (must be claude or codex)", opts.Harness)
	}
}

// hookCommand renders the curl one-liner for one harness+kind. Short timeout,
// silent, and `|| true` so a slow or down agen8 can never block or fail the
// agent's hook chain.
func hookCommand(baseURL, token, harness string, spec hookSpec) string {
	timeout := spec.TimeoutSecs
	if timeout <= 0 {
		timeout = 3
	}
	return fmt.Sprintf(
		"curl -m %d -s -o /dev/null -X POST '%s/hooks/attention?harness=%s&kind=%s' -H 'Authorization: Bearer %s' --data-binary @- || true",
		timeout, baseURL, harness, spec.Kind, token,
	)
}

// installClaude merges agen8's hook entries into <project>/.claude/settings.local.json.
// settings.local.json (not settings.json) keeps the embedded token out of git.
// The merge is surgical: within each event, any existing group containing an
// agen8 attention command is replaced; everything else is preserved verbatim.
func installClaude(opts Options, baseURL string) (Result, error) {
	projectDir := strings.TrimSpace(opts.ProjectDir)
	if projectDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Result{}, fmt.Errorf("hooks install: resolve working directory: %w", err)
		}
		projectDir = wd
	}
	path := filepath.Join(projectDir, ".claude", "settings.local.json")

	settings := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return Result{}, fmt.Errorf("hooks install: %s exists but is not valid JSON: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("hooks install: read %s: %w", path, err)
	}

	mergeHookEvents(settings, claudeHookEvents, "claude-code", baseURL, opts.Token)

	if err := writeJSONFile(path, settings); err != nil {
		return Result{}, err
	}
	return Result{Harness: HarnessClaude, Path: path}, nil
}

// installCodex merges agen8's hook entries into ~/.codex/hooks.json — the
// user-level file (never a repo-level .codex/) so the embedded token stays out
// of version control. The file shape mirrors Claude Code's: {"hooks": {Event:
// [matcher groups]}} (verified against codex-rs config/hook_config.rs).
func installCodex(opts Options, baseURL string) (Result, error) {
	homeDir := strings.TrimSpace(opts.HomeDir)
	if homeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Result{}, fmt.Errorf("hooks install: resolve home directory: %w", err)
		}
		homeDir = home
	}
	path := filepath.Join(homeDir, ".codex", "hooks.json")

	config := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return Result{}, fmt.Errorf("hooks install: %s exists but is not valid JSON: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("hooks install: read %s: %w", path, err)
	}

	mergeHookEvents(config, codexHookEvents, "codex", baseURL, opts.Token)

	if err := writeJSONFile(path, config); err != nil {
		return Result{}, err
	}
	return Result{Harness: HarnessCodex, Path: path}, nil
}

// mergeHookEvents upserts agen8's hook groups into the container's "hooks"
// map. Both harnesses share the same matcher-group shape, so one merge serves
// Claude Code's settings.local.json and Codex's hooks.json.
func mergeHookEvents(container map[string]any, specs []hookSpec, harness, baseURL, token string) {
	hooks, _ := container["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	cleanedGroups := map[string][]any{}
	for _, spec := range specs {
		groups, ok := cleanedGroups[spec.Event]
		if !ok {
			groups = withoutAgen8Groups(hooks[spec.Event])
		}
		group := map[string]any{
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": hookCommand(baseURL, token, harness, spec),
			}},
		}
		if spec.Matcher != "" {
			group["matcher"] = spec.Matcher
		}
		cleanedGroups[spec.Event] = append(groups, group)
		hooks[spec.Event] = cleanedGroups[spec.Event]
	}
	container["hooks"] = hooks
}

func writeJSONFile(path string, content map[string]any) error {
	out, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return fmt.Errorf("hooks install: encode %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("hooks install: create %s: %w", filepath.Dir(path), err)
	}
	// 0600: the file embeds the bearer token.
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		return fmt.Errorf("hooks install: write %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("hooks install: secure permissions for %s: %w", path, err)
	}
	return nil
}

// withoutAgen8Groups filters an event's existing matcher-groups, dropping the
// ones agen8 installed previously (identified by the attention endpoint in any
// of their commands) and keeping user-authored groups untouched.
func withoutAgen8Groups(existing any) []any {
	groups, _ := existing.([]any)
	kept := make([]any, 0, len(groups))
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			kept = append(kept, g)
			continue
		}
		if groupHasAgen8Hook(group) {
			continue
		}
		kept = append(kept, g)
	}
	return kept
}

func groupHasAgen8Hook(group map[string]any) bool {
	hooks, _ := group["hooks"].([]any)
	for _, h := range hooks {
		hook, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, _ := hook["command"].(string); strings.Contains(cmd, hookMarker) {
			return true
		}
	}
	return false
}
