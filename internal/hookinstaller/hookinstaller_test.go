package hookinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return out
}

func eventGroups(t *testing.T, config map[string]any, event string) []any {
	t.Helper()
	hooks, _ := config["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatalf("no hooks map in config")
	}
	groups, _ := hooks[event].([]any)
	return groups
}

func TestInstallClaudeWritesSettingsLocal(t *testing.T) {
	dir := t.TempDir()
	result, err := Install(Options{
		Harness:    HarnessClaude,
		BaseURL:    "http://127.0.0.1:7777/",
		Token:      "ak_test",
		ProjectDir: dir,
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := filepath.Join(dir, ".claude", "settings.local.json")
	if result.Path != want {
		t.Fatalf("path = %s, want %s", result.Path, want)
	}
	config := readJSON(t, want)

	// Notification carries two matcher groups: idle -> waiting, permission -> needs_approval.
	notif := eventGroups(t, config, "Notification")
	if len(notif) != 2 {
		t.Fatalf("Notification groups = %d, want 2", len(notif))
	}
	first := notif[0].(map[string]any)
	if first["matcher"] != "idle_prompt" {
		t.Fatalf("first Notification matcher = %v", first["matcher"])
	}
	cmd := first["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if !strings.Contains(cmd, "harness=claude-code&kind=waiting") || !strings.Contains(cmd, "Bearer ak_test") {
		t.Fatalf("unexpected command: %s", cmd)
	}
	// Trailing slash on the base URL must not produce a double slash.
	if strings.Contains(cmd, "7777//") {
		t.Fatalf("base URL not normalized: %s", cmd)
	}
	for _, event := range []string{"Stop", "UserPromptSubmit", "SessionEnd"} {
		if len(eventGroups(t, config, event)) != 1 {
			t.Fatalf("expected one group for %s", event)
		}
	}
	// AskUserQuestion is matched by tool name: posed -> asking, answered -> cleared.
	pre := eventGroups(t, config, "PreToolUse")[0].(map[string]any)
	if pre["matcher"] != "AskUserQuestion" {
		t.Fatalf("PreToolUse matcher = %v", pre["matcher"])
	}
	preCmd := pre["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if !strings.Contains(preCmd, "kind=asking") {
		t.Fatalf("PreToolUse should map to asking: %s", preCmd)
	}
	// PostToolUse carries two groups: question answered + the Bash heartbeat.
	postGroups := eventGroups(t, config, "PostToolUse")
	if len(postGroups) != 2 {
		t.Fatalf("PostToolUse groups = %d, want 2", len(postGroups))
	}
	post := postGroups[0].(map[string]any)
	postCmd := post["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if post["matcher"] != "AskUserQuestion" || !strings.Contains(postCmd, "kind=cleared") {
		t.Fatalf("PostToolUse should clear on AskUserQuestion: %v / %s", post["matcher"], postCmd)
	}
	heartbeat := postGroups[1].(map[string]any)
	heartbeatCmd := heartbeat["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if heartbeat["matcher"] != "Bash" || !strings.Contains(heartbeatCmd, "kind=cleared") {
		t.Fatalf("expected Bash heartbeat clearing: %v / %s", heartbeat["matcher"], heartbeatCmd)
	}
	// High-frequency hook: must use the tight 1s timeout, not the default 3s.
	if !strings.Contains(heartbeatCmd, "curl -m 1 ") {
		t.Fatalf("Bash heartbeat should use -m 1: %s", heartbeatCmd)
	}

	// The file embeds a token: must not be group/world readable.
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
	}
}

func TestInstallClaudeIsIdempotentAndPreservesUserHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
		"permissions": {"allow": ["Bash(go test:*)"]},
		"hooks": {
			"Stop": [{"hooks": [{"type": "command", "command": "say done"}]}]
		}
	}`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := Options{Harness: HarnessClaude, BaseURL: "http://x", Token: "ak_1", ProjectDir: dir}
	if _, err := Install(opts); err != nil {
		t.Fatalf("first install: %v", err)
	}
	// Re-run with a rotated token: agen8's entries are replaced, not duplicated.
	opts.Token = "ak_2"
	if _, err := Install(opts); err != nil {
		t.Fatalf("second install: %v", err)
	}

	config := readJSON(t, path)
	if _, ok := config["permissions"]; !ok {
		t.Fatal("unrelated settings were dropped")
	}
	stop := eventGroups(t, config, "Stop")
	if len(stop) != 2 {
		t.Fatalf("Stop groups = %d, want 2 (user's + agen8's)", len(stop))
	}
	userCmd := stop[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if userCmd != "say done" {
		t.Fatalf("user hook not preserved: %s", userCmd)
	}
	agen8Cmd := stop[1].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if !strings.Contains(agen8Cmd, "Bearer ak_2") || strings.Contains(agen8Cmd, "ak_1") {
		t.Fatalf("token not rotated: %s", agen8Cmd)
	}
}

func TestInstallClaudeMCPUsesPrivateLocalScopeAndReplacesOnlyAgen8(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	run := func(_ context.Context, gotDir string, args ...string) ([]byte, error) {
		if gotDir != dir {
			t.Fatalf("project dir=%q want %q", gotDir, dir)
		}
		calls = append(calls, append([]string(nil), args...))
		if len(args) > 1 && args[1] == "remove" {
			return []byte("No project-local MCP server found with name: agen8"), errors.New("exit status 1")
		}
		return nil, nil
	}

	result, err := InstallClaudeMCP(MCPOptions{
		BaseURL:    "http://127.0.0.1:7777/",
		Token:      "ak_new",
		ProjectDir: dir,
		runCommand: run,
	})
	if err != nil {
		t.Fatalf("InstallClaudeMCP: %v", err)
	}
	if !strings.HasSuffix(result.Path, ".claude.json") || result.ServerName != "agen8" || result.URL != "http://127.0.0.1:7777/mcp" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(calls) != 2 {
		t.Fatalf("command calls=%d want 2", len(calls))
	}
	if got := strings.Join(calls[0], " "); got != "mcp remove --scope local agen8" {
		t.Fatalf("remove args=%q", got)
	}
	if got := strings.Join(calls[1][:5], " "); got != "mcp add-json --scope local agen8" {
		t.Fatalf("add args=%q", got)
	}
	var server map[string]any
	if err := json.Unmarshal([]byte(calls[1][5]), &server); err != nil {
		t.Fatalf("parse server JSON: %v", err)
	}
	if server["type"] != "http" || server["url"] != "http://127.0.0.1:7777/mcp" {
		t.Fatalf("server=%+v", server)
	}
	headers := server["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer ak_new" {
		t.Fatalf("Authorization header = %v", headers["Authorization"])
	}
}

func TestInstallClaudeMCPIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return nil, nil
	}
	for _, token := range []string{"ak_first", "ak_rotated"} {
		if _, err := InstallClaudeMCP(MCPOptions{BaseURL: "http://127.0.0.1:7777", Token: token, ProjectDir: dir, runCommand: run}); err != nil {
			t.Fatalf("InstallClaudeMCP(%s): %v", token, err)
		}
	}
	if len(calls) != 4 || calls[0][1] != "remove" || calls[1][1] != "add-json" || calls[2][1] != "remove" || calls[3][1] != "add-json" {
		t.Fatalf("unexpected command sequence: %+v", calls)
	}
	if strings.Contains(calls[1][5], "ak_rotated") || !strings.Contains(calls[3][5], "ak_rotated") {
		t.Fatalf("token was not rotated across idempotent install")
	}
}

func TestInstallClaudeMCPRejectsNonLocalRoot(t *testing.T) {
	_, err := InstallClaudeMCP(MCPOptions{
		BaseURL:    "http://127.0.0.1:7777",
		Token:      "ak_test",
		ProjectDir: "relative/path",
	})
	if err == nil || !strings.Contains(err.Error(), "absolute local path") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestInstallClaudeMCPReportsCLIErrorWithoutEchoingToken(t *testing.T) {
	dir := t.TempDir()
	_, err := InstallClaudeMCP(MCPOptions{
		BaseURL:    "http://127.0.0.1:7777",
		Token:      "ak_must_not_leak",
		ProjectDir: dir,
		runCommand: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if args[1] == "remove" {
				return nil, nil
			}
			return []byte("invalid config containing ak_must_not_leak"), errors.New("exit status 1")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "add server") {
		t.Fatalf("unexpected err=%v", err)
	}
	if strings.Contains(err.Error(), "ak_must_not_leak") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestInstallCodexWritesUserLevelHooks(t *testing.T) {
	home := t.TempDir()
	result, err := Install(Options{
		Harness: HarnessCodex,
		BaseURL: "http://127.0.0.1:7777",
		Token:   "ak_test",
		HomeDir: home,
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := filepath.Join(home, ".codex", "hooks.json")
	if result.Path != want {
		t.Fatalf("path = %s, want %s", result.Path, want)
	}
	config := readJSON(t, want)
	for event, kind := range map[string]string{
		"Stop":              "kind=waiting",
		"PermissionRequest": "kind=needs_approval",
		"PostToolUse":       "kind=cleared", // Bash heartbeat
		"UserPromptSubmit":  "kind=cleared",
		"SessionStart":      "kind=cleared",
	} {
		groups := eventGroups(t, config, event)
		if len(groups) != 1 {
			t.Fatalf("%s groups = %d, want 1", event, len(groups))
		}
		cmd := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
		if !strings.Contains(cmd, "harness=codex&"+kind) {
			t.Fatalf("%s command wrong: %s", event, cmd)
		}
		// Codex marks a Stop hook Failed on non-empty non-JSON stdout, and an
		// empty response abstains on PermissionRequest — the command must be
		// fully silent on stdout.
		if !strings.Contains(cmd, "-o /dev/null") || !strings.Contains(cmd, "-s") {
			t.Fatalf("%s command is not stdout-silent: %s", event, cmd)
		}
	}
}

func TestInstallSecuresExistingHookFileMode(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".claude", "settings.local.json")
		// Seed a valid hooks file first, then force the mode to a permissive value.
		if _, err := Install(Options{
			Harness:    HarnessClaude,
			BaseURL:    "http://127.0.0.1:7777",
			Token:      "ak_seed",
			ProjectDir: dir,
		}); err != nil {
			t.Fatalf("seed install: %v", err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Install(Options{
			Harness:    HarnessClaude,
			BaseURL:    "http://127.0.0.1:7777",
			Token:      "ak_test",
			ProjectDir: dir,
		}); err != nil {
			t.Fatalf("reinstall: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("codex", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, ".codex", "hooks.json")
		if _, err := Install(Options{
			Harness: HarnessCodex,
			BaseURL: "http://127.0.0.1:7777",
			Token:   "ak_seed",
			HomeDir: home,
		}); err != nil {
			t.Fatalf("seed install: %v", err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Install(Options{
			Harness: HarnessCodex,
			BaseURL: "http://127.0.0.1:7777",
			Token:   "ak_test",
			HomeDir: home,
		}); err != nil {
			t.Fatalf("reinstall: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
		}
	})
}

func TestInstallValidation(t *testing.T) {
	if _, err := Install(Options{Harness: HarnessClaude, Token: "t"}); err == nil {
		t.Fatal("expected error for missing url")
	}
	if _, err := Install(Options{Harness: HarnessClaude, BaseURL: "http://x"}); err == nil {
		t.Fatal("expected error for missing token")
	}
	if _, err := Install(Options{Harness: "vscode", BaseURL: "http://x", Token: "t"}); err == nil {
		t.Fatal("expected error for unknown harness")
	}
}

func TestInstallRejectsSymlinkedHookConfigFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.json")
	if err := os.WriteFile(target, []byte(`{"hooks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(link, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedPath := filepath.Join(link, "settings.local.json")
	if err := os.Remove(linkedPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(target, linkedPath); err != nil {
		t.Fatal(err)
	}

	_, err := Install(Options{
		Harness:    HarnessClaude,
		BaseURL:    "http://127.0.0.1:7777",
		Token:      "ak_test",
		ProjectDir: dir,
	})
	if err == nil {
		t.Fatal("expected install to reject symlinked config file")
	}
}

func TestInstallRejectsSymlinkedHookConfigParentDir(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".codex-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}

	codexDir := filepath.Join(home, ".codex")
	if err := os.Symlink(target, codexDir); err != nil {
		t.Fatal(err)
	}

	_, err := Install(Options{
		Harness: HarnessCodex,
		BaseURL: "http://127.0.0.1:7777",
		Token:   "ak_test",
		HomeDir: home,
	})
	if err == nil {
		t.Fatal("expected install to reject symlinked parent directory")
	}
}
