package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewTextLoggerHonorsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewTextLogger(&buf, Config{Level: "warn"})
	if err != nil {
		t.Fatalf("NewTextLogger: %v", err)
	}

	logger.Info("hidden")
	logger.Warn("shown", "service", "task")

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Fatalf("info log was written at warn level: %s", out)
	}
	if !strings.Contains(out, "shown") {
		t.Fatalf("warn log missing: %s", out)
	}
	if !strings.Contains(out, "service=task") {
		t.Fatalf("service field missing: %s", out)
	}
}

func TestNewTextLoggerRejectsUnknownLevel(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewTextLogger(&buf, Config{Level: "verbose"})
	if err == nil {
		t.Fatal("NewTextLogger returned nil error for unknown level")
	}
}

func TestNewLoggerRejectsTraversalPath(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatalf("mkdir inside: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	if err := os.Chdir(inside); err != nil {
		t.Fatalf("chdir inside: %v", err)
	}

	_, err = NewLogger(Config{File: "../outside/probe.log"})
	if err == nil {
		t.Fatal("NewLogger returned nil error for traversal path")
	}

	if _, err := os.Stat(filepath.Join(outside, "probe.log")); err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("stat outside log: %v", err)
		}
	} else {
		t.Fatal("NewLogger created log file outside working directory")
	}
}

func TestNewLoggerRejectsEnvTraversalPath(t *testing.T) {
	t.Setenv(EnvLogFile, "../outside/env.log")

	_, err := NewLogger(Config{})
	if err == nil {
		t.Fatal("NewLogger returned nil error for env traversal path")
	}
}

func TestNewLoggerRejectsAbsolutePath(t *testing.T) {
	_, err := NewLogger(Config{File: filepath.Join(t.TempDir(), "agen8.log")})
	if err == nil {
		t.Fatal("NewLogger returned nil error for absolute path")
	}
}

func TestNewLoggerWritesLegitimateRelativePath(t *testing.T) {
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	logger, err := NewLogger(Config{File: "tmp/daemon.log"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Info("probe")

	out, err := os.ReadFile(filepath.Join(root, "tmp", "daemon.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(out), "probe") {
		t.Fatalf("log file missing message: %s", out)
	}
}

func TestNewLoggerUsesSecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission mode assertions are Unix-specific")
	}

	// cleanLogFilePath only accepts relative paths, so create the log under a
	// temp working directory and assert the modes on what NewLogger created.
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	logger, err := NewLogger(Config{File: "logs/app.log", Level: "warn"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if logger == nil {
		t.Fatalf("NewLogger returned nil logger")
	}

	logger.Info("secure mode check")

	if got := dirMode(t, filepath.Join(root, "logs")).Perm(); got != 0o700 {
		t.Fatalf("log dir mode = %04o, want %04o", got, 0o700)
	}

	if got := fileMode(t, filepath.Join(root, "logs", "app.log")).Perm(); got != 0o600 {
		t.Fatalf("log file mode = %04o, want %04o", got, 0o600)
	}
}

func dirMode(t *testing.T, path string) os.FileMode {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	return info.Mode()
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	return info.Mode()
}
