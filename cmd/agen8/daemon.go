package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/tinoosan/agen8/internal/config"
	"github.com/tinoosan/agen8/internal/daemon"
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the agen8 daemon",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newDaemonStartCmd())
	return cmd
}

func newDaemonStartCmd() *cobra.Command {
	var (
		dataDir  string
		listener string
		httpAddr string
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon (HTTP listener + MCP server)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
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
		},
	}
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "agen8 data directory")
	cmd.Flags().StringVar(&listener, "listener", daemon.ListenerHTTP, "daemon listener")
	cmd.Flags().StringVar(&httpAddr, "http-addr", daemon.DefaultHTTPAddr, "HTTP listen address")
	return cmd
}
