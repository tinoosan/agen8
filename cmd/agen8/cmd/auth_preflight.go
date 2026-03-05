package cmd

import (
	"context"

	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/pkg/config"
)

func requireRuntimeAuthReady(ctx context.Context, cfg config.Config) error {
	return app.EnsureRuntimeAuthReady(ctx, cfg.DataDir, "")
}
