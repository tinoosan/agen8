package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestHandlerReadyz(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(NewServer().Handler())
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/readyz")
	if err != nil {
		t.Fatalf("get readyz: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read readyz body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", string(body))
	}
	if resp.Header.Get("X-Agen8-Bridge-Version") != Version {
		t.Fatalf("version header = %q", resp.Header.Get("X-Agen8-Bridge-Version"))
	}
}

func TestParseClaudeCommandSpecAcceptsMultilineArgs(t *testing.T) {
	payload, err := json.Marshal(claudeCommandSpec{
		Type:    claudeBridgeStartMessageType,
		Command: "claude",
		Args:    []string{"--mcp-config", "{\n  \"mcpServers\": {}\n}"},
		Workdir: "/srv/project",
		Env:     []string{"AGEN8=1"},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	spec, err := parseClaudeCommandSpec(payload)
	if err != nil {
		t.Fatalf("parseClaudeCommandSpec: %v", err)
	}
	if spec.Command != "claude" || spec.Workdir != "/srv/project" {
		t.Fatalf("spec=%+v", spec)
	}
	if len(spec.Args) != 2 || spec.Args[1] != "{\n  \"mcpServers\": {}\n}" {
		t.Fatalf("args=%q", spec.Args)
	}
}

func TestParseClaudeCommandSpecRequiresStartMessage(t *testing.T) {
	_, err := parseClaudeCommandSpec([]byte(`{"command":"claude","workdir":"/srv/project"}`))
	if err == nil {
		t.Fatalf("expected missing start message error")
	}
}

func TestServeRejectsMissingHTTPAddress(t *testing.T) {
	t.Parallel()

	err := NewServer().Serve(context.Background(), " ")
	if err == nil {
		t.Fatalf("expected missing address error")
	}
}

func TestCodexCandidatePathsIncludesCommonHomePath(t *testing.T) {
	home := t.TempDir()
	paths := map[string]bool{}
	for _, got := range codexCandidatePaths(home) {
		paths[got] = true
	}
	for _, path := range []string{
		filepath.Join(home, ".local", "bin", "codex"),
		filepath.Join(home, ".linuxbrew", "bin", "codex"),
		"/home/linuxbrew/.linuxbrew/bin/codex",
	} {
		if !paths[path] {
			t.Fatalf("candidate paths do not include %q", path)
		}
	}
}

func TestResolveCodexExecutableUsesExplicitBridgeEnv(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("SHELL", "")
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write codex: %v", err)
	}
	t.Setenv("AGEN8_CODEX_BIN", path)

	got, err := resolveCodexExecutable()
	if err != nil {
		t.Fatalf("resolveCodexExecutable: %v", err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}

func TestCodexBackendReportsProcessExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start command: %v", err)
	}
	backend := newCodexBackend(cmd, "", nil, nil, nil)
	deadline := time.After(2 * time.Second)
	for {
		err, exited := backend.exitStatus()
		if exited {
			if err == nil {
				t.Fatalf("exit err=nil want non-nil")
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for backend exit")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestCodexAppServerArgsIncludesMCPConfigOverrides(t *testing.T) {
	args := codexAppServerArgs(true, "ws://127.0.0.1:9000", []string{
		`model="gpt-5.5"`,
		`model_reasoning_effort="medium"`,
		`mcp_servers.agen8.url="http://127.0.0.1:38123/mcp?token=abc"`,
		" ",
	})
	want := []string{
		"app-server",
		"--listen",
		"ws://127.0.0.1:9000",
		"--config",
		`model="gpt-5.5"`,
		"--config",
		`model_reasoning_effort="medium"`,
		"--config",
		`mcp_servers.agen8.url="http://127.0.0.1:38123/mcp?token=abc"`,
	}
	if len(args) != len(want) {
		t.Fatalf("args=%v want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args=%v want %v", args, want)
		}
	}
}

func TestResolveCodexExecutablePrefersUserInstallOverBundledEditorBinary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	userCodex := filepath.Join(home, ".local", "bin", "codex")
	bundledDir := filepath.Join(home, ".cursor-server", "extensions", "openai.chatgpt", "bin")
	bundledCodex := filepath.Join(bundledDir, "codex")
	for _, path := range []string{userCodex, bundledCodex} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bundledDir)
	t.Setenv("SHELL", "")
	t.Setenv("AGEN8_CODEX_BIN", "")

	got, err := resolveCodexExecutable()
	if err != nil {
		t.Fatalf("resolveCodexExecutable: %v", err)
	}
	if got != userCodex {
		t.Fatalf("path = %q, want %q", got, userCodex)
	}
}

func TestResolveClaudeExecutableFindsUserInstallOutsidePATH(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	userClaude := filepath.Join(home, ".local", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(userClaude), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(userClaude), err)
	}
	if err := os.WriteFile(userClaude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write claude: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	t.Setenv("SHELL", "")
	t.Setenv("AGEN8_CLAUDE_BIN", "")

	got, err := resolveClaudeExecutable()
	if err != nil {
		t.Fatalf("resolveClaudeExecutable: %v", err)
	}
	if got != userClaude {
		t.Fatalf("path = %q, want %q", got, userClaude)
	}
}

func TestResolveNPMExecutableFindsUserNodeInstallOutsidePATH(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	userNPM := filepath.Join(home, ".nvm", "versions", "node", "v22.0.0", "bin", "npm")
	if err := os.MkdirAll(filepath.Dir(userNPM), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(userNPM), err)
	}
	if err := os.WriteFile(userNPM, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write npm: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	t.Setenv("SHELL", "")

	got, err := resolveNPMExecutable()
	if err != nil {
		t.Fatalf("resolveNPMExecutable: %v", err)
	}
	if got != userNPM {
		t.Fatalf("path = %q, want %q", got, userNPM)
	}
}
