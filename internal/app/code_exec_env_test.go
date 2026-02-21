package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
)

func TestResolveCodeExecVenvPath_DefaultAndRelativeOverride(t *testing.T) {
	cfg := config.Config{DataDir: "/tmp/agen8"}
	got := resolveCodeExecVenvPath(cfg)
	want := filepath.Join("/tmp/agen8", "exec", ".venv")
	if got != want {
		t.Fatalf("default venv path=%q want=%q", got, want)
	}
	cfg.CodeExec.VenvPath = "custom-venv"
	got = resolveCodeExecVenvPath(cfg)
	want = filepath.Join("/tmp/agen8", "custom-venv")
	if got != want {
		t.Fatalf("relative venv path=%q want=%q", got, want)
	}
}

func TestEnsureCodeExecPythonEnv_CreatesVenv(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	out, err := ensureCodeExecPythonEnv(context.Background(), cfg, "python3", nil)
	if err != nil {
		t.Fatalf("ensureCodeExecPythonEnv: %v", err)
	}
	if !out.VenvCreated {
		t.Fatalf("expected venv creation")
	}
}

func TestEnsureCodeExecPythonEnv_InstallFailsForMissingPackage(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	_, err := ensureCodeExecPythonEnv(context.Background(), cfg, "python3", []string{"module_that_should_not_exist_code_exec"})
	if err == nil {
		t.Fatalf("expected install failure")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pip_install") {
		t.Fatalf("expected pip_install error stage, got %v", err)
	}
}
