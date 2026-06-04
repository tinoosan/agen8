package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/daemon"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra/claudecli"
)

func TestDaemonStartDefaultsToLocalListener(t *testing.T) {
	var captured daemon.Config
	original := runDaemon
	runDaemon = func(_ context.Context, cfg daemon.Config) error {
		captured = cfg
		return nil
	}
	t.Cleanup(func() { runDaemon = original })

	cmd := newRootCommand()
	cmd.SetArgs([]string{"daemon", "start", "--data-dir", t.TempDir()})
	require.NoError(t, cmd.Execute())
	require.Equal(t, daemon.ListenerLocal, captured.Listener)
	require.NotEmpty(t, captured.Endpoint)
}

func TestDaemonStartSelectsHTTPListener(t *testing.T) {
	var captured daemon.Config
	original := runDaemon
	runDaemon = func(_ context.Context, cfg daemon.Config) error {
		captured = cfg
		return nil
	}
	t.Cleanup(func() { runDaemon = original })

	cmd := newRootCommand()
	cmd.SetArgs([]string{"daemon", "start", "--data-dir", t.TempDir(), "--listener", "http", "--http-addr", "127.0.0.1:0"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, daemon.ListenerHTTP, captured.Listener)
	require.Equal(t, "127.0.0.1:0", captured.HTTPAddr)
}

func TestDaemonStartRejectsInvalidListener(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"daemon", "start", "--data-dir", t.TempDir(), "--listener", "bogus"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown daemon listener")
}

func TestBridgeServePassesHTTPAddr(t *testing.T) {
	var captured string
	original := runBridge
	runBridge = func(_ context.Context, httpAddr string) error {
		captured = httpAddr
		return nil
	}
	t.Cleanup(func() { runBridge = original })

	cmd := newRootCommand()
	cmd.SetArgs([]string{"bridge", "serve", "--http-addr", "127.0.0.1:0"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "127.0.0.1:0", captured)
}

func TestMCPStdioRunsStdioTransport(t *testing.T) {
	called := false
	original := runMCPStdio
	runMCPStdio = func(context.Context) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runMCPStdio = original })

	cmd := newRootCommand()
	cmd.SetArgs([]string{"mcp", "stdio"})
	require.NoError(t, cmd.Execute())
	require.True(t, called)
}

func TestClaudeSetupPassesOptions(t *testing.T) {
	var captured claudecli.SetupOptions
	original := runClaudeSetup
	runClaudeSetup = func(_ context.Context, opts claudecli.SetupOptions) (claudecli.SetupResult, error) {
		captured = opts
		return claudecli.SetupResult{
			ProjectRoot:   opts.ProjectRoot,
			MCPConfigPath: opts.ProjectRoot + "/.mcp.json",
			SettingsPath:  opts.ProjectRoot + "/.claude/settings.local.json",
			MCPURL:        opts.MCPURL,
			HookCommand:   opts.HookCommand,
			HookArgs:      opts.HookArgs,
			ChannelStatus: "not ready",
		}, nil
	}
	t.Cleanup(func() { runClaudeSetup = original })

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"claude", "setup",
		"--project-root", "/repo",
		"--mcp-url", "http://127.0.0.1:7777/mcp?token=abc",
		"--hook-command", "/bin/agen8",
		"--hook-arg", "claude",
		"--hook-arg", "hook",
		"--channel-command", "/bin/agen8-channel",
		"--channel-arg", "claude",
		"--channel-arg", "channel",
	})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "/repo", captured.ProjectRoot)
	require.Equal(t, "http://127.0.0.1:7777/mcp?token=abc", captured.MCPURL)
	require.Equal(t, "/bin/agen8", captured.HookCommand)
	require.Equal(t, []string{"claude", "hook"}, captured.HookArgs)
	require.Equal(t, "/bin/agen8-channel", captured.ChannelCommand)
	require.Equal(t, []string{"claude", "channel"}, captured.ChannelArgs)
	require.Contains(t, out.String(), `"settingsPath"`)
}

func TestClaudeChannelRunsAdapter(t *testing.T) {
	var captured string
	original := runClaudeChannel
	runClaudeChannel = func(_ context.Context, listenAddr string) error {
		captured = listenAddr
		return nil
	}
	t.Cleanup(func() { runClaudeChannel = original })

	cmd := newRootCommand()
	cmd.SetArgs([]string{"claude", "channel", "--listen", "127.0.0.1:9010"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "127.0.0.1:9010", captured)
}
