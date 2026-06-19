package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/buildinfo"
)

// runCLI executes the full command tree with args, capturing combined stdout so
// tests assert against what a user sees. Cobra writes command output to the
// command's OutOrStdout, which Execute inherits from the root's SetOut.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestRunVersionPrintsBuildInfo(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate
	t.Cleanup(func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildDate = oldBuildDate
	})
	buildinfo.Version = "v0.1.0"
	buildinfo.Commit = "abc1234"
	buildinfo.BuildDate = "2026-06-05T19:30:00Z"

	output, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("run version: %v", err)
	}
	for _, want := range []string{"agen8 v0.1.0", "commit: abc1234", "built: 2026-06-05T19:30:00Z"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q missing %q", output, want)
		}
	}
}

func TestHelpListsHumanFacingCommands(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {}} {
		output, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("run %v: unexpected error %v", args, err)
		}
		for _, want := range []string{"daemon", "skill", "hooks", "healthcheck", "version"} {
			if !strings.Contains(output, want) {
				t.Fatalf("run %v output missing %q:\n%s", args, want, output)
			}
		}
		// claude hook is internal and must not appear in the synopsis.
		if strings.Contains(output, "claude") {
			t.Fatalf("run %v leaked the internal claude entrypoint into help:\n%s", args, output)
		}
	}
}

func TestUnknownCommandErrors(t *testing.T) {
	_, err := runCLI(t, "bogus")
	if err == nil {
		t.Fatal("unknown command unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error %q should name the unknown command", err)
	}
}

func TestClaudeHookStillRunsButIsHidden(t *testing.T) {
	// Hidden from help…
	output, err := runCLI(t, "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	if strings.Contains(output, "claude") {
		t.Fatalf("claude entrypoint should be hidden, help was:\n%s", output)
	}
	// …but still dispatchable: a PreToolUse payload for a non-agen8 tool passes
	// through unchanged (claudehook handles the decode and echoes a response).
	root := newRootCmd()
	root.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{}}`))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"claude", "hook"})
	if err := root.Execute(); err != nil {
		t.Fatalf("claude hook should still run: %v", err)
	}
}

func TestRunHealthcheckAcceptsHealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	if _, err := runCLI(t, "healthcheck", "--url", server.URL); err != nil {
		t.Fatalf("run healthcheck: %v", err)
	}
}

func TestRunHealthcheckRejectsUnhealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	_, err := runCLI(t, "healthcheck", "--url", server.URL)
	if err == nil {
		t.Fatal("run healthcheck unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error=%v, want status code", err)
	}
}

func TestRunSkillInstallWritesSkill(t *testing.T) {
	home := t.TempDir()

	output, err := runCLI(t, "skill", "install", "--harness", "codex", "--home", home)
	if err != nil {
		t.Fatalf("run skill install: %v", err)
	}

	wantRoot := filepath.Join(home, ".codex", "skills")
	if _, err := os.Stat(filepath.Join(wantRoot, "agen8", "SKILL.md")); err != nil {
		t.Fatalf("expected installed skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantRoot, "agen8-graph", "SKILL.md")); err != nil {
		t.Fatalf("expected installed graph skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantRoot, "agen8-coordination", "SKILL.md")); err != nil {
		t.Fatalf("expected installed coordination skill: %v", err)
	}
	for _, want := range []string{
		"installed agen8 skills for codex",
		"root: " + wantRoot,
		"skills: agen8, agen8-coordination, agen8-graph",
		"rerun this command to refresh",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q missing %q", output, want)
		}
	}
}
