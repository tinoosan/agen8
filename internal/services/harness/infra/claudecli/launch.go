package claudecli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	defaultClaudeChannelServerRef = "server:agen8-channel"
	defaultClaudeChannelPluginRef = "plugin:agen8@skills-dir"
)

type LaunchOptions struct {
	ProjectRoot                     string
	SpaceID                         string
	ClaudeCommand                   string
	RemoteControlTitle              string
	ChannelRef                      string
	DevelopmentChannel              bool
	AllowDangerouslySkipPermissions bool
}

type LaunchResult struct {
	ProjectRoot                     string   `json:"projectRoot"`
	SpaceID                         string   `json:"spaceId,omitempty"`
	ClaudeCommand                   string   `json:"claudeCommand"`
	Args                            []string `json:"args"`
	CommandLine                     string   `json:"commandLine"`
	PID                             int      `json:"pid"`
	LogPath                         string   `json:"logPath"`
	RemoteControlTitle              string   `json:"remoteControlTitle"`
	ChannelRef                      string   `json:"channelRef"`
	DevelopmentChannel              bool     `json:"developmentChannel"`
	AllowDangerouslySkipPermissions bool     `json:"allowDangerouslySkipPermissions"`
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
		if opts.DevelopmentChannel {
			channelRef = defaultClaudeChannelServerRef
		} else {
			channelRef = defaultClaudeChannelPluginRef
		}
	}
	title := strings.TrimSpace(opts.RemoteControlTitle)
	if title == "" {
		title = "Agen8: " + filepath.Base(projectRoot)
	}
	spaceID := strings.TrimSpace(opts.SpaceID)
	if err := writeClaudeLaunchContext(projectRoot, claudeLaunchContext{SpaceID: spaceID}); err != nil {
		return LaunchResult{}, err
	}
	if err := removeClaudeMCPToolSessionStartHook(projectRoot); err != nil {
		return LaunchResult{}, err
	}
	args := []string{"--remote-control", title}
	if opts.DevelopmentChannel {
		args = append(args, "--dangerously-load-development-channels", channelRef)
	} else {
		args = append(args, "--channels", channelRef)
	}
	if opts.AllowDangerouslySkipPermissions {
		args = append(args, "--permission-mode", "bypassPermissions", "--allow-dangerously-skip-permissions")
	}
	logPath, err := nextClaudeLaunchLogPath(projectRoot)
	if err != nil {
		return LaunchResult{}, err
	}
	pid, err := startClaudeDetached(projectRoot, logPath, claudeCommand, args, opts.DevelopmentChannel, launchEnv(spaceID))
	if err != nil {
		return LaunchResult{}, err
	}
	return LaunchResult{
		ProjectRoot:                     projectRoot,
		SpaceID:                         spaceID,
		ClaudeCommand:                   claudeCommand,
		Args:                            append([]string(nil), args...),
		CommandLine:                     shellCommandLine(claudeCommand, args),
		PID:                             pid,
		LogPath:                         logPath,
		RemoteControlTitle:              title,
		ChannelRef:                      channelRef,
		DevelopmentChannel:              opts.DevelopmentChannel,
		AllowDangerouslySkipPermissions: opts.AllowDangerouslySkipPermissions,
	}, nil
}

type claudeLaunchContext struct {
	SpaceID   string `json:"spaceId,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

func writeClaudeLaunchContext(projectRoot string, context claudeLaunchContext) error {
	if strings.TrimSpace(context.SpaceID) == "" {
		return nil
	}
	context.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.MarshalIndent(context, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claude launch context: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(projectRoot, ".agen8", "claude-launch-context.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create claude launch context dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write claude launch context: %w", err)
	}
	return nil
}

func readClaudeLaunchContext(projectRoot string) (claudeLaunchContext, error) {
	raw, err := os.ReadFile(filepath.Join(projectRoot, ".agen8", "claude-launch-context.json"))
	if err != nil {
		return claudeLaunchContext{}, err
	}
	var context claudeLaunchContext
	if err := json.Unmarshal(raw, &context); err != nil {
		return claudeLaunchContext{}, fmt.Errorf("parse claude launch context: %w", err)
	}
	return context, nil
}

func launchEnv(spaceID string) []string {
	if spaceID = strings.TrimSpace(spaceID); spaceID == "" {
		return nil
	}
	return []string{EnvAgen8SpaceID + "=" + spaceID}
}

func startClaudeDetached(projectRoot string, logPath string, claudeCommand string, args []string, developmentChannel bool, env []string) (int, error) {
	if runtime.GOOS == "darwin" && strings.TrimSpace(os.Getenv("AGEN8_CLAUDE_LAUNCH_FORCE_PTY")) == "" {
		return startClaudeWithScript(projectRoot, logPath, claudeCommand, args, developmentChannel, env)
	}
	return startClaudeWithPTY(projectRoot, logPath, claudeCommand, args, developmentChannel, env)
}

func startClaudeWithScript(projectRoot string, logPath string, claudeCommand string, args []string, developmentChannel bool, env []string) (int, error) {
	outerLogPath := logPath + ".outer"
	outerLogFile, err := os.OpenFile(outerLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open claude launch wrapper log: %w", err)
	}
	scriptCommand := shellCommandLine("/usr/bin/script", append([]string{"-q", logPath, claudeCommand}, args...))
	command := "exec " + scriptCommand
	if developmentChannel {
		command = "(sleep 2; printf '\\r'; sleep 2147483647) | " + scriptCommand
	}
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = outerLogFile
	cmd.Stderr = outerLogFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = outerLogFile.Close()
		return 0, fmt.Errorf("launch claude remote-control: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		_ = outerLogFile.Close()
		return 0, fmt.Errorf("detach claude remote-control: %w", err)
	}
	_ = outerLogFile.Close()
	return pid, nil
}

func startClaudeWithPTY(projectRoot string, logPath string, claudeCommand string, args []string, developmentChannel bool, env []string) (int, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open claude launch log: %w", err)
	}
	cmd := exec.Command(claudeCommand, args...)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), env...)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		_ = logFile.Close()
		return 0, fmt.Errorf("launch claude remote-control: %w", err)
	}
	if developmentChannel {
		go func() {
			for i, delay := range []time.Duration{750, 1500, 2500, 3500, 4500} {
				time.Sleep(delay * time.Millisecond)
				if i == 0 {
					_, _ = ptmx.Write([]byte("1\r\n"))
					continue
				}
				_, _ = ptmx.Write([]byte("\r\n"))
			}
		}()
	}
	go func() {
		_, _ = io.Copy(logFile, ptmx)
		if err := cmd.Wait(); err != nil {
			_, _ = fmt.Fprintf(logFile, "\nagen8 claude launch exited: %v\n", err)
		}
		_ = ptmx.Close()
		_ = logFile.Close()
	}()
	return cmd.Process.Pid, nil
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
