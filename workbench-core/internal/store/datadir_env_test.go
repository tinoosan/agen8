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

	_, _, err = CreateSession(cfg, "env data dir test", 256)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	dbPath := fsutil.GetSQLitePath(cfg.DataDir)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected sqlite db to exist at %s: %v", dbPath, err)
	}
}
