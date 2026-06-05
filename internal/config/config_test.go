package config

import "testing"

func TestDefault_DataDir(t *testing.T) {
	cfg := Default()
	if cfg.DataDir != "" {
		t.Fatalf("DataDir=%q want empty unresolved default", cfg.DataDir)
	}
}

func TestConfig_Validate_PostgresRequiresDatabaseURL(t *testing.T) {
	cfg := Default()
	cfg.DataDir = t.TempDir()
	cfg.DBDriver = "postgres"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected postgres config without database url to fail")
	}

	cfg.DatabaseURL = "postgres://user:pass@localhost:5432/agen8"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("postgres config with database url should validate: %v", err)
	}
}
