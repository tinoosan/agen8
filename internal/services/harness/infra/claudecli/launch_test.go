package claudecli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLaunchRemoteControlStartsClaudeWithDevelopmentChannel(t *testing.T) {
	t.Setenv("AGEN8_CLAUDE_LAUNCH_FORCE_PTY", "1")
	root := t.TempDir()
	argsPath := filepath.Join(root, "args.txt")
	fakeClaude := filepath.Join(root, "claude")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args.txt\nsleep 1\n"
	if err := os.WriteFile(fakeClaude, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	result, err := LaunchRemoteControl(context.Background(), LaunchOptions{
		ProjectRoot:        root,
		ClaudeCommand:      fakeClaude,
		RemoteControlTitle: "Agen8 test",
		DevelopmentChannel: true,
	})
	if err != nil {
		t.Fatalf("LaunchRemoteControl: %v", err)
	}
	if result.PID == 0 {
		t.Fatal("missing pid")
	}
	if result.ChannelRef != "server:agen8-channel" {
		t.Fatalf("channel ref=%q", result.ChannelRef)
	}
	if !strings.Contains(result.CommandLine, "--dangerously-load-development-channels") {
		t.Fatalf("command line=%q", result.CommandLine)
	}
	if _, err := os.Stat(result.LogPath); err != nil {
		t.Fatalf("launch log missing: %v", err)
	}

	var raw []byte
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		raw, err = os.ReadFile(argsPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{"--remote-control", "Agen8 test", "--dangerously-load-development-channels", "server:agen8-channel"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args=%#v want %#v", args, want)
	}
}

func TestLaunchRemoteControlSupportsApprovedChannelFlag(t *testing.T) {
	t.Setenv("AGEN8_CLAUDE_LAUNCH_FORCE_PTY", "1")
	root := t.TempDir()
	argsPath := filepath.Join(root, "args.txt")
	fakeClaude := filepath.Join(root, "claude")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args.txt\nsleep 1\n"
	if err := os.WriteFile(fakeClaude, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	result, err := LaunchRemoteControl(context.Background(), LaunchOptions{
		ProjectRoot:   root,
		ClaudeCommand: fakeClaude,
	})
	if err != nil {
		t.Fatalf("LaunchRemoteControl: %v", err)
	}
	if result.DevelopmentChannel {
		t.Fatal("expected approved-channel mode")
	}

	var raw []byte
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		raw, err = os.ReadFile(argsPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if result.ChannelRef != "plugin:agen8@skills-dir" {
		t.Fatalf("channel ref=%q", result.ChannelRef)
	}
	want := []string{"--remote-control", "Agen8: " + filepath.Base(root), "--channels", "plugin:agen8@skills-dir"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args=%#v want %#v", args, want)
	}
}
