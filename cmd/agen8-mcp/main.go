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

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/daemon"
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
	if args[0] != "daemon" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	if len(args) < 2 || args[1] != "start" {
		return fmt.Errorf("usage: agen8-mcp daemon start [--data-dir DIR] [--listener http] [--http-addr ADDR]")
	}
	return runDaemonStart(args[2:])
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
	if strings.TrimSpace(dataDir) != "" {
		hostConfig.DataDir = strings.TrimSpace(dataDir)
	}
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
