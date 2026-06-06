package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tinoosan/agen8-mcp-server/internal/claudehook"
	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/daemon"
	"github.com/tinoosan/agen8-mcp-server/pkg/buildinfo"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "agen8-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runDaemonStart(nil)
	}
	switch args[0] {
	case "version", "--version", "-v":
		info := buildinfo.Current()
		fmt.Printf("agen8-mcp %s\ncommit: %s\n", info.Version, info.Commit)
		if info.BuildDate != "" {
			fmt.Printf("built: %s\n", info.BuildDate)
		}
		return nil
	case "claude":
		return runClaude(args[1:])
	}
	if args[0] != "daemon" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	if len(args) < 2 || args[1] != "start" {
		return fmt.Errorf("usage: agen8-mcp daemon start [--data-dir DIR] [--listener http] [--http-addr ADDR]")
	}
	return runDaemonStart(args[2:])
}

// runClaude dispatches the Claude Code integration subcommands. Today the only
// one is `claude hook`, the PreToolUse entrypoint that stamps a conversation's
// session_id into agen8 tool calls so each Claude conversation resolves to its
// own member. It reads a hook payload on stdin and writes the hook response on
// stdout (see internal/claudehook).
func runClaude(args []string) error {
	if len(args) == 0 || args[0] != "hook" {
		return fmt.Errorf("usage: agen8-mcp claude hook")
	}
	return claudehook.Run(os.Stdin, os.Stdout, os.Stderr)
}

func runDaemonStart(args []string) error {
	fs := flag.NewFlagSet("daemon start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		dataDir  string
		listener string
		httpAddr string
	)
	fs.StringVar(&dataDir, "data-dir", "", "agen8 data directory")
	fs.StringVar(&listener, "listener", daemon.ListenerHTTP, "daemon listener")
	fs.StringVar(&httpAddr, "http-addr", daemon.DefaultHTTPAddr, "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	hostConfig := config.Default()
	resolvedDataDir, err := config.ResolveDataDir(dataDir, strings.TrimSpace(dataDir) != "")
	if err != nil {
		return err
	}
	hostConfig.DataDir = resolvedDataDir
	d, err := daemon.New(daemon.Config{
		AppConfig: hostConfig,
		Listener:  listener,
		HTTPAddr:  httpAddr,
		Out:       os.Stdout,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = daemon.HTTPStrategy{}.Run(ctx, d)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
