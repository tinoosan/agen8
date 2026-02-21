package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	pkgobsidian "github.com/tinoosan/agen8/pkg/obsidian"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	agen8KeyringService = "agen8"
	openRouterProvider  = "openrouter"
	openRouterAPIKeyEnv = "OPENROUTER_API_KEY"
	openRouterModelEnv  = "OPENROUTER_MODEL"

	onboardingBanner = `
============================================
  Welcome to Agen8 — autonomous agent runtime
============================================

This guided setup will configure the daemon for its first run.
You will need an API key from your LLM provider (default: OpenRouter).
`
)

var (
	keyringGet       = keyring.Get
	keyringSet       = keyring.Set
	onboardingPrompt = promptLine
	onboardingSecret = promptSecret
)

func ensureRuntimeCredentials(dataDir string, interactive bool, in *os.File, out io.Writer) error {
	if strings.TrimSpace(os.Getenv(openRouterAPIKeyEnv)) != "" {
		return nil
	}
	if key, err := readAPIKeyFromKeyring(openRouterProvider); err == nil && key != "" {
		_ = os.Setenv(openRouterAPIKeyEnv, key)
		return nil
	}
	if !interactive {
		cfgPath := filepath.Join(strings.TrimSpace(dataDir), "config.toml")
		return fmt.Errorf("%s is required. Start in a TTY for guided setup, or set it manually with `export %s=...`.\nConfig path: %s", openRouterAPIKeyEnv, openRouterAPIKeyEnv, cfgPath)
	}
	return runInteractiveRuntimeOnboarding(dataDir, in, out)
}

func readAPIKeyFromKeyring(provider string) (string, error) {
	account := keyringAccountName(provider)
	key, err := keyringGet(agen8KeyringService, account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(key), nil
}

func runInteractiveRuntimeOnboarding(dataDir string, in *os.File, out io.Writer) error {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	reader := bufio.NewReader(in)

	if _, err := fmt.Fprint(out, onboardingBanner); err != nil {
		return err
	}
	provider, err := onboardingPrompt(reader, out, "Provider", openRouterProvider)
	if err != nil {
		return err
	}
	if provider == "" {
		provider = openRouterProvider
	}
	model, err := onboardingPrompt(reader, out, "Default model", runtimeDefaultModel)
	if err != nil {
		return err
	}
	if model == "" {
		model = runtimeDefaultModel
	}
	obsidianStatus := pkgobsidian.DetectInstall()
	obsidianVaultPath, err := configureObsidianOnboarding(reader, out)
	if err != nil {
		return err
	}
	key, err := onboardingSecret(in, out, "API key")
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("api key cannot be empty")
	}
	if err := keyringSet(agen8KeyringService, keyringAccountName(provider), key); err != nil {
		return fmt.Errorf("store API key in keychain: %w", err)
	}
	_ = os.Setenv(openRouterAPIKeyEnv, key)
	setEnvIfUnset(openRouterModelEnv, model)
	if err := upsertRuntimeConfigDefaultsModel(dataDir, model); err != nil {
		return err
	}
	printOnboardingSummary(out, provider, model, dataDir, obsidianStatus.Installed, obsidianVaultPath)
	return nil
}

func printOnboardingSummary(out io.Writer, provider, model, dataDir string, obsidianInstalled bool, obsidianVaultPath string) {
	cfgPath := filepath.Join(strings.TrimSpace(dataDir), "config.toml")
	obsidianLine := "not detected (you can still use Agen8; obsidian tool calls will fail until installed)"
	if obsidianInstalled {
		obsidianLine = "detected"
	}
	fmt.Fprintf(out, `
--- Setup complete ---

  Provider:   %s
  Model:      %s
  API key:    saved to OS keychain
  Obsidian:   %s
  Vault:      %s
  Config:     %s

--- Next steps ---

  Start the daemon:
    agen8 daemon

  Monitor with the TUI:
    agen8 monitor

  Attach in control mode:
    agen8

  Edit runtime config:
    %s
`, provider, model, obsidianLine, strings.TrimSpace(obsidianVaultPath), cfgPath, cfgPath)
}

func keyringAccountName(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = openRouterProvider
	}
	return provider + ".api_key"
}

func promptLine(reader *bufio.Reader, out io.Writer, label, def string) (string, error) {
	if _, err := fmt.Fprintf(out, "%s [%s]: ", strings.TrimSpace(label), strings.TrimSpace(def)); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = strings.TrimSpace(def)
	}
	return value, nil
}

func promptSecret(in *os.File, out io.Writer, label string) (string, error) {
	if _, err := fmt.Fprintf(out, "%s: ", strings.TrimSpace(label)); err != nil {
		return "", err
	}
	raw, err := term.ReadPassword(int(in.Fd()))
	if _, werr := fmt.Fprintln(out); werr != nil && err == nil {
		err = werr
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func upsertRuntimeConfigDefaultsModel(dataDir, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	path, err := ensureRuntimeConfigTemplate(dataDir)
	if err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw := runtimeConfigFile{}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	raw.Defaults.Model = model

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	if err := enc.Encode(raw); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}

func configureObsidianOnboarding(reader *bufio.Reader, out io.Writer) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	projectCtx, err := LoadProjectContext(cwd)
	if err != nil {
		return "", err
	}
	if !projectCtx.Exists {
		if _, err := InitProject(cwd, ProjectConfig{}); err != nil {
			return "", err
		}
		projectCtx, err = LoadProjectContext(cwd)
		if err != nil {
			return "", err
		}
	}
	defaultPath := strings.TrimSpace(projectCtx.Config.ObsidianVaultPath)
	if defaultPath == "" {
		defaultPath = "/project/obsidian-vault"
	}
	def, err := pkgobsidian.ResolveDefaultVaultPath(cwd, strings.TrimSpace(projectCtx.Config.ObsidianVaultPath))
	if err != nil {
		return "", err
	}
	vaultPath, err := onboardingPrompt(reader, out, "Obsidian vault path", defaultPath)
	if err != nil {
		return "", err
	}
	vaultPath = strings.TrimSpace(vaultPath)
	if vaultPath == "" {
		vaultPath = defaultPath
	}
	if pkgobsidian.IsWorkspacePath(vaultPath) {
		return "", fmt.Errorf("INVALID_VAULT_PATH: refusing run-scoped /workspace path: %s", vaultPath)
	}
	resolved, err := pkgobsidian.ResolveVaultPath(pkgobsidian.ResolveOptions{
		ExplicitPath:      vaultPath,
		ProjectRoot:       cwd,
		ProjectVaultPath:  strings.TrimSpace(projectCtx.Config.ObsidianVaultPath),
		KnowledgeRootHost: def.Host,
	})
	if err != nil {
		return "", err
	}
	cfg := projectCtx.Config
	cfg.ObsidianVaultPath = resolved.Logical
	cfg.ObsidianEnabled = true
	if _, err := SaveProjectConfig(projectCtx.RootDir, cfg); err != nil {
		return "", err
	}
	status := pkgobsidian.DetectInstall()
	if !status.Installed {
		if _, err := fmt.Fprintln(out, "Warning: Obsidian was not detected. Obsidian tool calls will fail until Obsidian is installed."); err != nil {
			return "", err
		}
	}
	return resolved.Logical, nil
}
