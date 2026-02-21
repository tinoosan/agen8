package obsidian

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultVaultLogicalPath = "/project/obsidian-vault"
	projectConfigDirName    = ".agen8"
	legacyProjectConfigDir  = ".agent8"
	projectConfigName       = "config.toml"
	vaultConfigName         = "vault.conf"
)

type ResolvedPath struct {
	Logical string
	Host    string
}

type ResolveOptions struct {
	ExplicitPath      string
	ProjectRoot       string
	ProjectVaultPath  string
	KnowledgeRootHost string
}

func ResolveDefaultVaultPath(projectRoot string, projectVaultPath string) (ResolvedPath, error) {
	return resolvePath(ResolveOptions{
		ProjectRoot:      projectRoot,
		ProjectVaultPath: projectVaultPath,
	})
}

func ResolveVaultPath(opts ResolveOptions) (ResolvedPath, error) {
	return resolvePath(opts)
}

func IsWorkspacePath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return path == "/workspace" || strings.HasPrefix(path, "/workspace/")
}

func ResolveProjectVaultPath(projectRoot string) string {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return ""
	}
	path := filepath.Join(projectRoot, projectConfigDirName, projectConfigName)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			legacyPath := filepath.Join(projectRoot, legacyProjectConfigDir, projectConfigName)
			b, err = os.ReadFile(legacyPath)
		}
	}
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	inProjectSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.TrimSpace(strings.Trim(line, "[]")))
			inProjectSection = section == "project"
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if strings.HasPrefix(key, "project.") {
			key = strings.TrimSpace(strings.TrimPrefix(key, "project."))
		}
		if key != "obsidian_vault_path" {
			continue
		}
		if !inProjectSection && key == "obsidian_vault_path" {
			// legacy flat format is allowed as fallback.
		}
		if !inProjectSection && strings.Contains(strings.TrimSpace(parts[0]), ".") {
			continue
		}
		raw := strings.TrimSpace(parts[1])
		if raw == "" {
			return ""
		}
		if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
			if unquoted, err := strconv.Unquote(raw); err == nil {
				return strings.TrimSpace(unquoted)
			}
		}
		return strings.TrimSpace(raw)
	}
	return ""
}

func resolvePath(opts ResolveOptions) (ResolvedPath, error) {
	projectRoot, err := resolveProjectRoot(opts.ProjectRoot)
	if err != nil {
		return ResolvedPath{}, err
	}
	knowledgeRootHost := strings.TrimSpace(opts.KnowledgeRootHost)
	if knowledgeRootHost == "" {
		knowledgeRootHost = filepath.Join(projectRoot, "obsidian-vault")
	}

	candidate := strings.TrimSpace(opts.ExplicitPath)
	if candidate == "" {
		if env := strings.TrimSpace(os.Getenv("OBSIDIAN_VAULT_PATH")); env != "" {
			candidate = env
		}
	}
	if candidate == "" {
		if conf := readVaultConf(); conf != "" {
			candidate = conf
		}
	}
	if candidate == "" {
		if projectCfg := strings.TrimSpace(opts.ProjectVaultPath); projectCfg != "" {
			candidate = projectCfg
		}
	}
	if candidate == "" {
		candidate = defaultVaultLogicalPath
	}

	logical := normalizeLogical(candidate)
	host, err := logicalToHost(logical, projectRoot, knowledgeRootHost)
	if err != nil {
		return ResolvedPath{}, err
	}
	if err := rejectWorkspace(logical, host); err != nil {
		return ResolvedPath{}, err
	}
	return ResolvedPath{Logical: logical, Host: host}, nil
}

func readVaultConf() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".agents", vaultConfigName)
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

func resolveProjectRoot(projectRoot string) (string, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve cwd: %w", err)
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	return filepath.Clean(abs), nil
}

func normalizeLogical(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return defaultVaultLogicalPath
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return filepath.ToSlash(filepath.Clean(filepath.Join("/project", path)))
}

func logicalToHost(logical string, projectRoot string, knowledgeRoot string) (string, error) {
	switch {
	case logical == "/project":
		return projectRoot, nil
	case strings.HasPrefix(logical, "/project/"):
		rel := strings.TrimPrefix(logical, "/project/")
		host := filepath.Join(projectRoot, filepath.FromSlash(rel))
		abs, err := filepath.Abs(host)
		if err != nil {
			return "", err
		}
		return filepath.Clean(abs), nil
	case logical == "/knowledge":
		if knowledgeRoot == "" {
			return "", fmt.Errorf("knowledge root is not configured")
		}
		return filepath.Clean(knowledgeRoot), nil
	case strings.HasPrefix(logical, "/knowledge/"):
		if knowledgeRoot == "" {
			return "", fmt.Errorf("knowledge root is not configured")
		}
		rel := strings.TrimPrefix(logical, "/knowledge/")
		host := filepath.Join(knowledgeRoot, filepath.FromSlash(rel))
		abs, err := filepath.Abs(host)
		if err != nil {
			return "", err
		}
		return filepath.Clean(abs), nil
	default:
		expanded := os.ExpandEnv(logical)
		if strings.HasPrefix(expanded, "~") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", err
		}
		return filepath.Clean(abs), nil
	}
}

func rejectWorkspace(logical string, host string) error {
	if IsWorkspacePath(logical) {
		return fmt.Errorf("INVALID_VAULT_PATH: refusing run-scoped /workspace path: %s", logical)
	}
	hostSlash := filepath.ToSlash(filepath.Clean(strings.TrimSpace(host)))
	if IsWorkspacePath(hostSlash) {
		return fmt.Errorf("INVALID_VAULT_PATH: refusing run-scoped /workspace path: %s", hostSlash)
	}
	return nil
}
