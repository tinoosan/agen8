package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/pkg/buildinfo"
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
		"agen8-mcp v0.1.0",
		"commit: abc1234",
		"built: 2026-06-05T19:30:00Z",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q missing %q", output, want)
		}
	}
}

func TestRunSkillInstallWritesSkill(t *testing.T) {
	home := t.TempDir()

	output := captureStdout(t, func() {
		if err := run([]string{"skill", "install", "--harness", "codex", "--home", home}); err != nil {
			t.Fatalf("run skill install: %v", err)
		}
	})

	wantPath := filepath.Join(home, ".codex", "skills", "agen8", "SKILL.md")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected installed skill at %s: %v", wantPath, err)
	}
	for _, want := range []string{
		"installed agen8 skill for codex",
		"path: " + wantPath,
		"rerun this command to refresh the skill",
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
