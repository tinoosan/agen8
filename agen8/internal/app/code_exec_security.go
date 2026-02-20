package app

import (
	"context"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/events"
)

func emitCodeExecProvisioningSecurityWarning(ctx context.Context, cfg config.Config, emit func(context.Context, events.Event)) {
	_ = ctx
	_ = cfg
	_ = emit
}

func boolToString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func emitCodeExecConfigWarning(ctx context.Context, cfg config.Config, emit func(context.Context, events.Event)) {
	if emit == nil {
		return
	}
	if len(cfg.CodeExec.RequiredPackages) == 0 {
		return
	}
	emit(ctx, events.Event{
		Type:    "daemon.info",
		Message: "code_exec required packages loaded from config.toml",
		Data: map[string]string{
			"setting":  "code_exec.required_packages",
			"venvPath": resolveCodeExecVenvPath(cfg),
		},
	})
}
