package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/buildinfo"
)

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

	output := captureStdout(t, func() {
		if err := run([]string{"version"}); err != nil {
			t.Fatalf("run version: %v", err)
		}
	})

	for _, want := range []string{
		"agen8 v0.1.0",
		"commit: abc1234",
		"built: 2026-06-05T19:30:00Z",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q missing %q", output, want)
		}
	}
}

func TestRunHealthcheckAcceptsHealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	if err := run([]string{"healthcheck", "--url", server.URL}); err != nil {
		t.Fatalf("run healthcheck: %v", err)
	}
}

func TestRunHealthcheckRejectsUnhealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	err := run([]string{"healthcheck", "--url", server.URL})
	if err == nil {
		t.Fatal("run healthcheck unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error=%v, want status code", err)
	}
}

func TestRunSkillInstallWritesSkill(t *testing.T) {
	home := t.TempDir()

	output := captureStdout(t, func() {
		if err := run([]string{"skill", "install", "--harness", "codex", "--home", home}); err != nil {
			t.Fatalf("run skill install: %v", err)
		}
	})

	wantRoot := filepath.Join(home, ".codex", "skills")
	wantSkillPath := filepath.Join(wantRoot, "agen8", "SKILL.md")
	if _, err := os.Stat(wantSkillPath); err != nil {
		t.Fatalf("expected installed skill at %s: %v", wantSkillPath, err)
	}
	wantGraphSkillPath := filepath.Join(wantRoot, "agen8-graph", "SKILL.md")
	if _, err := os.Stat(wantGraphSkillPath); err != nil {
		t.Fatalf("expected installed graph skill at %s: %v", wantGraphSkillPath, err)
	}
	for _, want := range []string{
		"installed agen8 skills for codex",
		"root: " + wantRoot,
		"skills: agen8, agen8-graph",
		"rerun this command to refresh",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q missing %q", output, want)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = write
	defer func() {
		os.Stdout = old
	}()

	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, read); err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return buf.String()
}
