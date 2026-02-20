package config

import "testing"

func TestDefault_CodeExecDefaults(t *testing.T) {
	cfg := Default()
	if cfg.CodeExec.VenvPath != "" {
		t.Fatalf("expected empty venv_path by default")
	}
	if len(cfg.CodeExec.RequiredPackages) != 0 {
		t.Fatalf("expected empty required_packages by default")
	}
}
