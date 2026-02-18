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
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	workbenchKeyringService = "workbench"
	openRouterProvider      = "openrouter"
	openRouterAPIKeyEnv     = "OPENROUTER_API_KEY"
	openRouterModelEnv      = "OPENROUTER_MODEL"
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
	key, err := keyringGet(workbenchKeyringService, account)
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

	if _, err := fmt.Fprintln(out, "Workbench first-run setup"); err != nil {
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
	key, err := onboardingSecret(in, out, "API key")
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("api key cannot be empty")
	}
	if err := keyringSet(workbenchKeyringService, keyringAccountName(provider), key); err != nil {
		return fmt.Errorf("store API key in keychain: %w", err)
	}
	_ = os.Setenv(openRouterAPIKeyEnv, key)
	setEnvIfUnset(openRouterModelEnv, model)
	if err := upsertRuntimeConfigDefaultsModel(dataDir, model); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Saved API key to OS keychain and updated runtime defaults."); err != nil {
		return err
	}
	return nil
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
