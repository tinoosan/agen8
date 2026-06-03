package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8-mcp-server/internal/bridge"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var httpAddr string
	root := &cobra.Command{
		Use:           "agen8-bridge",
		Short:         "agen8 remote bridge",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the bridge",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return bridge.NewServer().Serve(ctx, httpAddr)
		},
	}
	serveCmd.Flags().StringVar(&httpAddr, "http-addr", "127.0.0.1:0", "bridge HTTP address")
	root.AddCommand(serveCmd)
	return root
}
