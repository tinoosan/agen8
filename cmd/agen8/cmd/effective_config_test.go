package cmd

import (
	"os"
	"path/filepath"
	"testing"

	authpkg "github.com/tinoosan/agen8/pkg/auth"
)

func TestEffectiveConfig_AppliesRuntimeConfigEnvDefaults(t *testing.T) {
	prevDataDir := dataDir
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir = prevDataDir })

	t.Setenv("AGEN8_DATA_DIR", "")
	prevAuth, hadAuth := os.LookupEnv(authpkg.EnvAuthProvider)
	_ = os.Unsetenv(authpkg.EnvAuthProvider)
	t.Cleanup(func() {
		if hadAuth {
			_ = os.Setenv(authpkg.EnvAuthProvider, prevAuth)
		} else {
			_ = os.Unsetenv(authpkg.EnvAuthProvider)
		}
	})
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(`
[auth]
provider = "chatgpt_account"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := effectiveConfig(nil); err != nil {
		t.Fatalf("effectiveConfig: %v", err)
	}
	if got := os.Getenv(authpkg.EnvAuthProvider); got != authpkg.ProviderChatGPTAccount {
		t.Fatalf("%s=%q", authpkg.EnvAuthProvider, got)
	}
}
