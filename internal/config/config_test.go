package config

import "testing"

func TestDefault_DataDir(t *testing.T) {
	cfg := Default()
	if cfg.DataDir != "" {
		t.Fatalf("DataDir=%q want empty unresolved default", cfg.DataDir)
	}
}
