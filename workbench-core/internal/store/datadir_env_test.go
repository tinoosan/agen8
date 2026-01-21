package store

import (
	"os"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
)

func TestDataDirFromEnv_WritesUnderResolvedDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(config.EnvDataDir, tmp)

	dataDir, err := config.ResolveDataDir("", false)
	if err != nil {
		t.Fatalf("ResolveDataDir failed: %v", err)
	}
	cfg := config.Config{DataDir: dataDir}

	_, run, err := CreateSession(cfg, "env data dir test", 256)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	runPath := fsutil.GetRunFilePath(cfg.DataDir, run.RunId)
	if _, err := os.Stat(runPath); err != nil {
		t.Fatalf("expected run.json to exist at %s: %v", runPath, err)
	}
}
