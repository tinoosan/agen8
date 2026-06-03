package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/daemon"
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
