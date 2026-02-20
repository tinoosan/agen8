package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
)

type codeExecEnvProvisionResult struct {
	VenvPath      string
	PythonBin     string
	VenvCreated   bool
	InstalledMods []string
	MissingMods   []string
}

type codeExecEnvError struct {
	Stage       string
	Err         error
	MissingMods []string
}

func (e *codeExecEnvError) Error() string {
	if e == nil {
		return ""
	}
	msg := strings.TrimSpace(e.Stage)
	if msg == "" {
		msg = "unknown"
	}
	if len(e.MissingMods) > 0 {
		return fmt.Sprintf("code_exec env %s: %v (missing: %s)", msg, e.Err, strings.Join(e.MissingMods, ","))
	}
	return fmt.Sprintf("code_exec env %s: %v", msg, e.Err)
}

func (e *codeExecEnvError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ensureCodeExecPythonEnv(ctx context.Context, cfg config.Config, preferredPythonBin string, requiredPackages []string) (codeExecEnvProvisionResult, error) {
	venvPath := resolveCodeExecVenvPath(cfg)
	venvPython := codeExecVenvPythonBin(venvPath)
	result := codeExecEnvProvisionResult{
		VenvPath:  venvPath,
		PythonBin: venvPython,
	}

	if _, err := os.Stat(venvPython); err != nil {
		if os.IsNotExist(err) {
			result.VenvCreated = true
			if err := os.MkdirAll(filepath.Dir(venvPath), 0o755); err != nil {
				return result, &codeExecEnvError{Stage: "venv_create", Err: err}
			}
			basePython := strings.TrimSpace(preferredPythonBin)
			if basePython == "" {
				basePython = "python3"
			}
			if _, err := exec.LookPath(basePython); err != nil {
				return result, &codeExecEnvError{Stage: "venv_create", Err: err}
			}
			if err := runCodeExecEnvCmd(ctx, "", basePython, "-m", "venv", venvPath); err != nil {
				return result, &codeExecEnvError{Stage: "venv_create", Err: err}
			}
		} else {
			return result, &codeExecEnvError{Stage: "venv_create", Err: err}
		}
	}

	requiredPackages = normalizeCodeExecImportList(requiredPackages)
	if len(requiredPackages) == 0 {
		return result, nil
	}
	missing, err := probeMissingPythonPackages(ctx, venvPython, requiredPackages)
	if err != nil {
		return result, &codeExecEnvError{Stage: "package_probe", Err: err}
	}
	result.MissingMods = append([]string(nil), missing...)
	if len(missing) == 0 {
		return result, nil
	}
	if err := runCodeExecEnvCmd(ctx, "", venvPython, "-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--upgrade", "pip"); err != nil {
		return result, &codeExecEnvError{Stage: "pip_bootstrap", Err: err}
	}
	installArgs := []string{"-m", "pip", "install", "--disable-pip-version-check", "--no-input"}
	installArgs = append(installArgs, missing...)
	if err := runCodeExecEnvCmd(ctx, "", venvPython, installArgs...); err != nil {
		return result, &codeExecEnvError{Stage: "pip_install", Err: err, MissingMods: missing}
	}
	result.InstalledMods = append([]string(nil), missing...)

	missingAfter, err := probeMissingPythonPackages(ctx, venvPython, requiredPackages)
	if err != nil {
		return result, &codeExecEnvError{Stage: "package_probe", Err: err}
	}
	result.MissingMods = append([]string(nil), missingAfter...)
	if len(missingAfter) > 0 {
		return result, &codeExecEnvError{
			Stage:       "package_probe",
			Err:         fmt.Errorf("package(s) still missing after install"),
			MissingMods: missingAfter,
		}
	}
	return result, nil
}

func resolveCodeExecVenvPath(cfg config.Config) string {
	venvPath := strings.TrimSpace(cfg.CodeExec.VenvPath)
	if venvPath == "" {
		return filepath.Join(strings.TrimSpace(cfg.DataDir), "exec", ".venv")
	}
	if filepath.IsAbs(venvPath) {
		return filepath.Clean(venvPath)
	}
	return filepath.Join(strings.TrimSpace(cfg.DataDir), venvPath)
}

func codeExecVenvPythonBin(venvPath string) string {
	return filepath.Join(strings.TrimSpace(venvPath), "bin", "python")
}

func probeMissingPythonPackages(ctx context.Context, pythonBin string, packages []string) ([]string, error) {
	packages = normalizeCodeExecImportList(packages)
	if len(packages) == 0 {
		return nil, nil
	}
	missing := make([]string, 0, len(packages))
	for _, pkg := range packages {
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		cmd := exec.CommandContext(probeCtx, pythonBin, "-m", "pip", "show", pkg)
		out, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(string(out)))
		if strings.Contains(text, "package(s) not found") || strings.Contains(text, "not found") {
			missing = append(missing, pkg)
			continue
		}
		return nil, fmt.Errorf("package probe failed for %q: %w (%s)", pkg, err, strings.TrimSpace(string(out)))
	}
	return missing, nil
}

func runCodeExecEnvCmd(ctx context.Context, workdir string, name string, args ...string) error {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(runCtx, name, args...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("%w (%s)", err, trimmed)
	}
	return nil
}

func normalizeCodeExecImportList(modules []string) []string {
	if len(modules) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(modules))
	for _, mod := range modules {
		mod = strings.TrimSpace(mod)
		mod = strings.Trim(mod, ",")
		if mod == "" {
			continue
		}
		if _, ok := seen[mod]; ok {
			continue
		}
		seen[mod] = struct{}{}
		out = append(out, mod)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}
