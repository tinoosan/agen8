package claudecli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/creack/pty"
)

const defaultClaudeChannelServerRef = "server:agen8-channel"

type LaunchOptions struct {
	ProjectRoot        string
	ClaudeCommand      string
	RemoteControlTitle string
	ChannelRef         string
	DevelopmentChannel bool
}

type LaunchResult struct {
	ProjectRoot        string   `json:"projectRoot"`
	ClaudeCommand      string   `json:"claudeCommand"`
	Args               []string `json:"args"`
	CommandLine        string   `json:"commandLine"`
	PID                int      `json:"pid"`
	LogPath            string   `json:"logPath"`
	RemoteControlTitle string   `json:"remoteControlTitle"`
	ChannelRef         string   `json:"channelRef"`
	DevelopmentChannel bool     `json:"developmentChannel"`
}

func LaunchRemoteControl(ctx context.Context, opts LaunchOptions) (LaunchResult, error) {
	if err := ctx.Err(); err != nil {
		return LaunchResult{}, err
	}
	projectRoot, err := resolveProjectRoot(opts.ProjectRoot)
	if err != nil {
		return LaunchResult{}, err
	}
	claudeCommand := strings.TrimSpace(opts.ClaudeCommand)
	if claudeCommand == "" {
		resolved, err := exec.LookPath("claude")
		if err != nil {
			return LaunchResult{}, fmt.Errorf("find claude executable: %w", err)
		}
		claudeCommand = resolved
	}
	channelRef := strings.TrimSpace(opts.ChannelRef)
	if channelRef == "" {
		channelRef = defaultClaudeChannelServerRef
	}
	title := strings.TrimSpace(opts.RemoteControlTitle)
	if title == "" {
		title = "Agen8: " + filepath.Base(projectRoot)
	}
	args := []string{"--remote-control", title}
	if opts.DevelopmentChannel {
		args = append(args, "--dangerously-load-development-channels", channelRef)
	} else {
		args = append(args, "--channels", channelRef)
	}
	logPath, err := nextClaudeLaunchLogPath(projectRoot)
	if err != nil {
		return LaunchResult{}, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return LaunchResult{}, fmt.Errorf("open claude launch log: %w", err)
	}
	cmd := exec.Command(claudeCommand, args...)
	cmd.Dir = projectRoot
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		_ = logFile.Close()
		return LaunchResult{}, fmt.Errorf("launch claude remote-control: %w", err)
	}
	if opts.DevelopmentChannel {
		go func() {
			for _, delay := range []time.Duration{1500, 2500, 3500, 4500} {
				time.Sleep(delay * time.Millisecond)
				_, _ = ptmx.Write([]byte("\r\n"))
			}
		}()
	}
	go func() {
		_, _ = io.Copy(logFile, ptmx)
		_ = cmd.Wait()
		_ = ptmx.Close()
		_ = logFile.Close()
	}()
	return LaunchResult{
		ProjectRoot:        projectRoot,
		ClaudeCommand:      claudeCommand,
		Args:               append([]string(nil), args...),
		CommandLine:        shellCommandLine(claudeCommand, args),
		PID:                cmd.Process.Pid,
		LogPath:            logPath,
		RemoteControlTitle: title,
		ChannelRef:         channelRef,
		DevelopmentChannel: opts.DevelopmentChannel,
	}, nil
}

func nextClaudeLaunchLogPath(projectRoot string) (string, error) {
	dir := filepath.Join(projectRoot, ".agen8", "claude-launches")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create claude launch log dir: %w", err)
	}
	name := "launch-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".log"
	return filepath.Join(dir, name), nil
}

func shellCommandLine(command string, args []string) string {
	parts := []string{shellQuote(command)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '=' || r == '+' || r == '@' || r == ',' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
