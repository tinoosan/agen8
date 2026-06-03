package infra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	locationapp "github.com/tinoosan/agen8-mcp-server/internal/services/location/app"
)

func TestShellCommandResolvesCodexWithProbePathRules(t *testing.T) {
	cmd := shellCommand(locationapp.CommandSpec{
		Command: "codex",
		Args:    []string{"app-server", "--listen", "ws://127.0.0.1:5555"},
		Workdir: "/srv/project",
	})

	if !strings.Contains(cmd, "resolve_codex_path") {
		t.Fatalf("codex command does not resolve codex path: %s", cmd)
	}
	if !strings.Contains(cmd, `"$codexPath" 'app-server'`) {
		t.Fatalf("codex command does not execute resolved codex path: %s", cmd)
	}
	if !strings.Contains(cmd, "cd '/srv/project'") {
		t.Fatalf("codex command does not cd into workdir: %s", cmd)
	}
}

func TestNormalizeBridgePlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kernel  string
		machine string
		want    bridgePlatform
	}{
		{name: "linux amd64", kernel: "Linux", machine: "x86_64", want: bridgePlatform{GOOS: "linux", GOARCH: "amd64"}},
		{name: "linux arm64", kernel: "linux", machine: "aarch64", want: bridgePlatform{GOOS: "linux", GOARCH: "arm64"}},
		{name: "darwin arm64", kernel: "Darwin", machine: "arm64", want: bridgePlatform{GOOS: "darwin", GOARCH: "arm64"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeBridgePlatform(tt.kernel, tt.machine)
			if err != nil {
				t.Fatalf("normalizeBridgePlatform: %v", err)
			}
			if got != tt.want {
				t.Fatalf("platform = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizeBridgePlatformRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	if _, err := normalizeBridgePlatform("FreeBSD", "x86_64"); err == nil {
		t.Fatalf("expected unsupported os error")
	}
	if _, err := normalizeBridgePlatform("Linux", "riscv64"); err == nil {
		t.Fatalf("expected unsupported arch error")
	}
}

func TestBridgeBinaryIsStaleWhenSourceIsNewer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module test\n", time.Now().Add(-time.Hour))
	writeTestFile(t, filepath.Join(root, "go.sum"), "", time.Now().Add(-time.Hour))
	writeTestFile(t, filepath.Join(root, "cmd", "agen8-bridge", "main.go"), "package main\n", time.Now().Add(time.Hour))
	writeTestFile(t, filepath.Join(root, "internal", "bridge", "server.go"), "package bridge\n", time.Now().Add(-time.Hour))

	stale, err := bridgeBinaryIsStale(root, time.Now())
	if err != nil {
		t.Fatalf("bridgeBinaryIsStale: %v", err)
	}
	if !stale {
		t.Fatalf("expected stale binary")
	}
}

func TestBridgeBinaryIsStaleAcceptsFreshBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceTime := time.Now().Add(-time.Hour)
	writeTestFile(t, filepath.Join(root, "go.mod"), "module test\n", sourceTime)
	writeTestFile(t, filepath.Join(root, "go.sum"), "", sourceTime)
	writeTestFile(t, filepath.Join(root, "cmd", "agen8-bridge", "main.go"), "package main\n", sourceTime)
	writeTestFile(t, filepath.Join(root, "internal", "bridge", "server.go"), "package bridge\n", sourceTime)

	stale, err := bridgeBinaryIsStale(root, time.Now())
	if err != nil {
		t.Fatalf("bridgeBinaryIsStale: %v", err)
	}
	if stale {
		t.Fatalf("expected fresh binary")
	}
}

func TestSSHBridgeLaunchCommandReusesRunningProcess(t *testing.T) {
	t.Parallel()

	cmd := sshBridgeLaunchCommand("loc_ssh", "/home/dev/.agen8/bin/agen8-bridge", 49152)
	if strings.Contains(cmd, "& &&") {
		t.Fatalf("launcher command contains invalid background/and sequence: %s", cmd)
	}
	if strings.Contains(cmd, "X-Agen8-Bridge-Version") {
		t.Fatalf("launcher command should not probe HTTP in shell: %s", cmd)
	}
	if !strings.Contains(cmd, `kill -0 "$PID"`) || !strings.Contains(cmd, `printf 'port=%s\n' "$(cat "$PORT_FILE")"; exit 0`) {
		t.Fatalf("launcher command does not reuse a running bridge with a saved port: %s", cmd)
	}
	if !strings.Contains(cmd, `BINARY_FILE="$STATE_DIR/bridge.binary"`) {
		t.Fatalf("launcher command does not track the running bridge binary: %s", cmd)
	}
	if !strings.Contains(cmd, `RUNNING_BINARY=$(cat "$BINARY_FILE" 2>/dev/null || true)`) || !strings.Contains(cmd, `[ "$RUNNING_BINARY" = "$EXPECTED_BINARY" ]`) {
		t.Fatalf("launcher command does not gate reuse on the uploaded bridge binary: %s", cmd)
	}
	if !strings.Contains(cmd, `kill "$PID"`) {
		t.Fatalf("launcher command does not kill a stale bridge pid: %s", cmd)
	}
	if !strings.Contains(cmd, `rm -f "$PID_FILE" "$PORT_FILE" "$BINARY_FILE"`) {
		t.Fatalf("launcher command does not clear stale launch files: %s", cmd)
	}
	if !strings.Contains(cmd, `> "$PORT_FILE"`) {
		t.Fatalf("launcher command does not persist bridge port: %s", cmd)
	}
	if !strings.Contains(cmd, `> "$BINARY_FILE"`) {
		t.Fatalf("launcher command does not persist bridge binary identity: %s", cmd)
	}
	if !strings.Contains(cmd, "AGEN8_CODEX_BIN=") {
		t.Fatalf("launcher command does not pass discovered codex path into bridge: %s", cmd)
	}
	if strings.Index(cmd, "$HOME/.local/bin/codex") > strings.Index(cmd, "${SHELL}") {
		t.Fatalf("launcher command should prefer user-installed codex paths before shell PATH: %s", cmd)
	}
	if !strings.Contains(cmd, ".cursor-server/extensions") {
		t.Fatalf("launcher command does not reject bundled editor codex paths: %s", cmd)
	}
	if !strings.Contains(cmd, `& echo $! > "$PID_FILE"`) {
		t.Fatalf("launcher command does not write background pid in the launch step: %s", cmd)
	}
	if !strings.Contains(cmd, "'/home/dev/.agen8/bin/agen8-bridge' serve --http-addr 127.0.0.1:49152") {
		t.Fatalf("launcher command does not start bridge binary: %s", cmd)
	}
}

func TestSSHBridgeDiagnosticsCommandReadsLaunchState(t *testing.T) {
	t.Parallel()

	cmd := sshBridgeDiagnosticsCommand("loc_ssh")
	for _, want := range []string{
		"$HOME/.agen8/ssh-launch/'loc_ssh'",
		"bridge.pid",
		"bridge.port",
		"bridge.log",
		"tail -n 80",
		"process=running",
		"port=",
		"bridge log missing",
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("diagnostics command missing %q: %s", want, cmd)
		}
	}
}

func TestParseSSHBridgeLaunchPort(t *testing.T) {
	t.Parallel()

	port, err := parseSSHBridgeLaunchPort("port=49152\n")
	if err != nil {
		t.Fatalf("parseSSHBridgeLaunchPort: %v", err)
	}
	if port != 49152 {
		t.Fatalf("port=%d want 49152", port)
	}
}

func TestFileSHA256Prefix(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("bridge"), 0o755); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got, err := fileSHA256Prefix(path, 12)
	if err != nil {
		t.Fatalf("fileSHA256Prefix: %v", err)
	}
	if got != "17f29b073143" {
		t.Fatalf("hash prefix = %q", got)
	}
}

func writeTestFile(t *testing.T, path, contents string, modTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}
