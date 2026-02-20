package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/events"
)

func startCodeExecConfigReloader(ctx context.Context, baseCfg config.Config, emit func(context.Context, events.Event)) {
	if strings.TrimSpace(baseCfg.DataDir) == "" {
		return
	}
	cfgPath := filepath.Join(strings.TrimSpace(baseCfg.DataDir), "config.toml")
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		defer ticker.Stop()
		var lastAppliedSig string
		var lastMtime time.Time
		reconcile := func(trigger string) {
			loaded, ok, err := decodeRuntimeConfigFile(cfgPath)
			if err != nil {
				if emit != nil {
					emit(ctx, events.Event{
						Type:    "config.reload.failed",
						Message: "config.toml reload failed",
						Data: map[string]string{
							"path":  cfgPath,
							"error": strings.TrimSpace(err.Error()),
						},
					})
				}
				return
			}
			if !ok {
				return
			}
			cfg := applyRuntimeConfigHostDefaults(baseCfg, loaded)
			required := resolveCodeExecRequiredImports(cfg.CodeExec.RequiredPackages)
			sig := strings.Join(required, ",") + "|" + resolveCodeExecVenvPath(cfg)
			if sig == lastAppliedSig {
				return
			}
			out, err := ensureCodeExecPythonEnv(ctx, cfg, "", required)
			if err != nil {
				if emit != nil {
					data := map[string]string{
						"path":            cfgPath,
						"trigger":         trigger,
						"error":           strings.TrimSpace(err.Error()),
						"requiredPackage": strings.Join(required, ","),
					}
					emit(ctx, events.Event{
						Type:    "code_exec.env.reconcile_failed",
						Message: "code_exec package reconcile failed",
						Data:    data,
					})
				}
				return
			}
			lastAppliedSig = sig
			if emit != nil {
				data := map[string]string{
					"path":             cfgPath,
					"trigger":          trigger,
					"venvPath":         strings.TrimSpace(out.VenvPath),
					"python":           strings.TrimSpace(out.PythonBin),
					"requiredPackages": strings.Join(required, ","),
				}
				if len(out.InstalledMods) > 0 {
					data["installedPackages"] = strings.Join(out.InstalledMods, ",")
				}
				emit(ctx, events.Event{
					Type:    "code_exec.env.reconciled",
					Message: "code_exec package reconcile complete",
					Data:    data,
				})
				emit(ctx, events.Event{
					Type:    "config.reload.success",
					Message: "config.toml reloaded",
					Data: map[string]string{
						"path": cfgPath,
					},
				})
			}
		}

		reconcile("startup")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				st, err := statFile(cfgPath)
				if err != nil {
					continue
				}
				mod := st.ModTime().UTC()
				if mod.After(lastMtime) {
					lastMtime = mod
					reconcile("file_change")
				}
			}
		}
	}()
}

func statFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
