package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/internal/web"
)

var (
	webPort int
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the agen8 web UI server",
	Long: `Start an HTTP server that serves the agen8 web UI and bridges
browser JSON-RPC calls to the daemon.

The daemon must already be running (agen8 daemon start) before opening the UI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		rpc := resolvedRPCEndpoint()
		addr := fmt.Sprintf(":%d", webPort)

		// Resolve the project root from the current working directory so
		// the web server can inject it into forwarded RPC requests.
		var projectRoot string
		projCtx, err := app.LoadProjectContext("")
		if err == nil && projCtx.Exists {
			projectRoot = projCtx.RootDir
			slog.Info("web: resolved project root", "root", projectRoot)
		}

		srv := &web.Server{
			Addr:        addr,
			RPCEndpoint: rpc,
			ProjectRoot: projectRoot,
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return srv.Run(ctx)
	},
}

func init() {
	webCmd.Flags().IntVar(&webPort, "port", 8080, "HTTP listen port for the web UI")
}
