package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/events"
)

// Security configuration constants
const (
	minReloadInterval     = 5 * time.Second                // Minimum time between config reloads
	maxPackageNameLength  = 100                            // Maximum length of a package name
	maxPackagesPerReload  = 50                             // Maximum number of packages per reload
	allowedPackagePattern = `^[a-zA-Z0-9][a-zA-Z0-9._-]*$` // Valid package name pattern
	maxPathLength         = 4096                           // Maximum path length
	maxDataDirDepth       = 10                             // Maximum directory depth for DataDir
)

// Rate limiter for config reloads
type reloadRateLimiter struct {
	mu          sync.Mutex
	lastReload  time.Time
	minInterval time.Duration
}

func newReloadRateLimiter(minInterval time.Duration) *reloadRateLimiter {
	return &reloadRateLimiter{
		minInterval: minInterval,
	}
}

func (r *reloadRateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.lastReload) < r.minInterval {
		return false
	}
	r.lastReload = now
	return true
}

// Config file state tracking for integrity
type configFileState struct {
	mu       sync.RWMutex
	checksum string
	modTime  time.Time
}

func (s *configFileState) hasChanged(checksum string, modTime time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.checksum != checksum || !s.modTime.Equal(modTime)
}

func (s *configFileState) update(checksum string, modTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checksum = checksum
	s.modTime = modTime
}

// Package name validation
var validPackageNameRegex = regexp.MustCompile(allowedPackagePattern)

func containsControlCharacter(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func hasPathTraversal(path string) bool {
	return strings.Contains(path, ".."+string(filepath.Separator)) ||
		strings.Contains(path, string(filepath.Separator)+"..") ||
		strings.Contains(path, "../") ||
		strings.Contains(path, `..\`) ||
		path == ".."
}

// SecurityError represents a security policy violation
type SecurityError struct {
	Stage   string
	Message string
	Details string
}

func (e *SecurityError) Error() string {
	if e == nil {
		return ""
	}
	if e.Details != "" {
		return fmt.Sprintf("security error [%s]: %s (%s)", e.Stage, e.Message, e.Details)
	}
	return fmt.Sprintf("security error [%s]: %s", e.Stage, e.Message)
}

// validatePackageName validates a Python package name according to PEP 508
func validatePackageName(name string) error {
	if containsControlCharacter(name) {
		return &SecurityError{
			Stage:   "package_validation",
			Message: "package name contains control characters",
			Details: fmt.Sprintf("name=%q", name),
		}
	}

	name = strings.TrimSpace(name)

	if name == "" {
		return &SecurityError{Stage: "package_validation", Message: "package name is empty"}
	}

	if len(name) > maxPackageNameLength {
		return &SecurityError{
			Stage:   "package_validation",
			Message: "package name exceeds maximum length",
			Details: fmt.Sprintf("max=%d, got=%d", maxPackageNameLength, len(name)),
		}
	}

	// Check against allowlist pattern
	if !validPackageNameRegex.MatchString(name) {
		return &SecurityError{
			Stage:   "package_validation",
			Message: "package name contains invalid characters",
			Details: fmt.Sprintf("name=%q, allowed_pattern=%s", name, allowedPackagePattern),
		}
	}

	// Block dangerous patterns
	dangerousPatterns := []string{
		"..", "//", "~", "$", "`", "|", ";", "&",
	}
	lowerName := strings.ToLower(name)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerName, pattern) {
			return &SecurityError{
				Stage:   "package_validation",
				Message: "package name contains dangerous pattern",
				Details: fmt.Sprintf("name=%q, pattern=%q", name, pattern),
			}
		}
	}

	return nil
}

// validatePackageList validates a list of package names
func validatePackageList(packages []string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}

	if len(packages) > maxPackagesPerReload {
		return nil, &SecurityError{
			Stage:   "package_list_validation",
			Message: "too many packages requested",
			Details: fmt.Sprintf("max=%d, got=%d", maxPackagesPerReload, len(packages)),
		}
	}

	seen := make(map[string]struct{}, len(packages))
	validated := make([]string, 0, len(packages))

	for _, pkg := range packages {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}

		// Normalize for deduplication
		normalized := strings.ToLower(pkg)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}

		if err := validatePackageName(pkg); err != nil {
			return nil, err
		}

		validated = append(validated, pkg)
	}

	return validated, nil
}

// validateDataDir validates the DataDir configuration
func validateDataDir(dataDir string) error {
	if containsControlCharacter(dataDir) {
		return &SecurityError{
			Stage:   "datadir_validation",
			Message: "DataDir contains control characters",
			Details: fmt.Sprintf("path=%q", dataDir),
		}
	}

	dataDir = strings.TrimSpace(dataDir)

	if dataDir == "" {
		return &SecurityError{Stage: "datadir_validation", Message: "DataDir is empty"}
	}

	if len(dataDir) > maxPathLength {
		return &SecurityError{
			Stage:   "datadir_validation",
			Message: "DataDir path exceeds maximum length",
			Details: fmt.Sprintf("max=%d, got=%d", maxPathLength, len(dataDir)),
		}
	}

	// Check for path traversal attempts
	cleaned := filepath.Clean(dataDir)
	if hasPathTraversal(dataDir) || cleaned == ".." {
		return &SecurityError{
			Stage:   "datadir_validation",
			Message: "DataDir contains path traversal sequence",
			Details: fmt.Sprintf("path=%q", dataDir),
		}
	}

	// Check directory depth
	depth := 0
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part != "" && part != "." {
			depth++
		}
	}
	if depth > maxDataDirDepth {
		return &SecurityError{
			Stage:   "datadir_validation",
			Message: "DataDir exceeds maximum directory depth",
			Details: fmt.Sprintf("max=%d, got=%d", maxDataDirDepth, depth),
		}
	}

	return nil
}

// validateVenvPath validates the virtual environment path
func validateVenvPath(venvPath string, dataDir string) error {
	if containsControlCharacter(venvPath) {
		return &SecurityError{
			Stage:   "venv_validation",
			Message: "VenvPath contains control characters",
			Details: fmt.Sprintf("path=%q", venvPath),
		}
	}

	venvPath = strings.TrimSpace(venvPath)

	if venvPath == "" {
		// Empty venvPath is valid - will use default
		return nil
	}

	if len(venvPath) > maxPathLength {
		return &SecurityError{
			Stage:   "venv_validation",
			Message: "VenvPath exceeds maximum length",
			Details: fmt.Sprintf("max=%d, got=%d", maxPathLength, len(venvPath)),
		}
	}

	// Must be relative path or within dataDir
	if filepath.IsAbs(venvPath) {
		// Absolute paths must be under dataDir
		absDataDir, err := filepath.Abs(dataDir)
		if err != nil {
			return &SecurityError{
				Stage:   "venv_validation",
				Message: "failed to resolve DataDir",
				Details: err.Error(),
			}
		}

		absVenv, err := filepath.Abs(venvPath)
		if err != nil {
			return &SecurityError{
				Stage:   "venv_validation",
				Message: "failed to resolve VenvPath",
				Details: err.Error(),
			}
		}

		if !strings.HasPrefix(absVenv, absDataDir+string(filepath.Separator)) {
			return &SecurityError{
				Stage:   "venv_validation",
				Message: "VenvPath must be within DataDir",
				Details: fmt.Sprintf("venv=%q, datadir=%q", venvPath, dataDir),
			}
		}
	}

	// Check for path traversal
	cleaned := filepath.Clean(venvPath)
	if hasPathTraversal(venvPath) || cleaned == ".." {
		return &SecurityError{
			Stage:   "venv_validation",
			Message: "VenvPath contains path traversal sequence",
			Details: fmt.Sprintf("path=%q", venvPath),
		}
	}

	return nil
}

// sanitizeEventData removes sensitive information from event data
func sanitizeEventData(data map[string]string) map[string]string {
	if data == nil {
		return nil
	}

	sanitized := make(map[string]string, len(data))
	sensitiveKeys := map[string]bool{
		"password":   true,
		"secret":     true,
		"token":      true,
		"key":        true,
		"auth":       true,
		"credential": true,
	}

	for k, v := range data {
		lowerKey := strings.ToLower(k)
		isSensitive := false
		for sensitive := range sensitiveKeys {
			if strings.Contains(lowerKey, sensitive) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			sanitized[k] = "[REDACTED]"
		} else {
			// Truncate long values
			if len(v) > 1000 {
				sanitized[k] = v[:997] + "..."
			} else {
				sanitized[k] = v
			}
		}
	}

	return sanitized
}

// computeFileChecksum calculates SHA256 checksum of file contents
func computeFileChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// statFile returns file info (extracted for testing)
func statFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// startCodeExecConfigReloader starts the configuration reloader with security hardening
func startCodeExecConfigReloader(ctx context.Context, baseCfg config.Config, emit func(context.Context, events.Event)) {
	// Validate DataDir before proceeding
	if err := validateDataDir(baseCfg.DataDir); err != nil {
		if emit != nil {
			emit(ctx, events.Event{
				Type:    "config.reload.failed",
				Message: "DataDir validation failed - reloader not started",
				Data: map[string]string{
					"error": err.Error(),
				},
			})
		}
		return
	}

	dataDir := strings.TrimSpace(baseCfg.DataDir)
	if dataDir == "" {
		return
	}

	// Validate VenvPath if set
	if err := validateVenvPath(baseCfg.CodeExec.VenvPath, dataDir); err != nil {
		if emit != nil {
			emit(ctx, events.Event{
				Type:    "config.reload.failed",
				Message: "VenvPath validation failed - reloader not started",
				Data: map[string]string{
					"error": err.Error(),
				},
			})
		}
		return
	}

	cfgPath := filepath.Join(dataDir, "config.toml")

	// Use longer ticker interval for rate limiting
	ticker := time.NewTicker(minReloadInterval)
	rateLimiter := newReloadRateLimiter(minReloadInterval)
	fileState := &configFileState{}

	go func() {
		defer ticker.Stop()

		var lastAppliedSig string
		var lastMtime time.Time

		reconcile := func(trigger string) {
			// Rate limiting check
			if !rateLimiter.allow() {
				if emit != nil {
					emit(ctx, events.Event{
						Type:    "config.reload.rate_limited",
						Message: "Config reload rate limited",
						Data: map[string]string{
							"path":    cfgPath,
							"trigger": trigger,
						},
					})
				}
				return
			}

			// Load and decode config with error handling
			loaded, ok, err := decodeRuntimeConfigFile(cfgPath)
			if err != nil {
				if emit != nil {
					emit(ctx, events.Event{
						Type:    "config.reload.failed",
						Message: "config.toml reload failed",
						Data: sanitizeEventData(map[string]string{
							"path":  cfgPath,
							"error": strings.TrimSpace(err.Error()),
						}),
					})
				}
				return
			}
			if !ok {
				return
			}

			// Apply host defaults
			cfg := applyRuntimeConfigHostDefaults(baseCfg, loaded)

			// Validate required packages before proceeding
			resolved, err := validatePackageList(cfg.CodeExec.RequiredPackages)
			if err != nil {
				if emit != nil {
					emit(ctx, events.Event{
						Type:    "config.reload.failed",
						Message: "Package list validation failed",
						Data: sanitizeEventData(map[string]string{
							"path":  cfgPath,
							"error": err.Error(),
						}),
					})
				}
				return
			}

			// Re-resolve imports using validated packages
			required := resolveCodeExecRequiredImports(resolved)
			sig := strings.Join(required, ",") + "|" + resolveCodeExecVenvPath(cfg)

			if sig == lastAppliedSig {
				return
			}

			out, err := ensureCodeExecPythonEnv(ctx, cfg, "", required)
			if err != nil {
				if emit != nil {
					data := sanitizeEventData(map[string]string{
						"path":            cfgPath,
						"trigger":         trigger,
						"error":           strings.TrimSpace(err.Error()),
						"requiredPackage": strings.Join(required, ","),
					})
					emit(ctx, events.Event{
						Type:    "code_exec.env.reconcile_failed",
						Message: "code_exec package reconcile failed",
						Data:    data,
					})
				}
				return
			}

			lastAppliedSig = sig

			if emit != nil {
				data := sanitizeEventData(map[string]string{
					"path":             cfgPath,
					"trigger":          trigger,
					"venvPath":         strings.TrimSpace(out.VenvPath),
					"python":           strings.TrimSpace(out.PythonBin),
					"requiredPackages": strings.Join(required, ","),
				})

				if len(out.InstalledMods) > 0 {
					data["installedPackages"] = strings.Join(out.InstalledMods, ",")
				}

				emit(ctx, events.Event{
					Type:    "code_exec.env.reconciled",
					Message: "code_exec package reconcile complete",
					Data:    data,
				})

				emit(ctx, events.Event{
					Type:    "config.reload.success",
					Message: "config.toml reloaded",
					Data: sanitizeEventData(map[string]string{
						"path": cfgPath,
					}),
				})
			}
		}

		reconcile("startup")

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				st, err := statFile(cfgPath)
				if err != nil {
					continue
				}

				mod := st.ModTime().UTC()

				// Check for actual file content changes using checksum
				if mod.After(lastMtime) {
					checksum, err := computeFileChecksum(cfgPath)
					if err != nil {
						if emit != nil {
							emit(ctx, events.Event{
								Type:    "config.reload.checksum_failed",
								Message: "Failed to compute config file checksum",
								Data: sanitizeEventData(map[string]string{
									"path":  cfgPath,
									"error": err.Error(),
								}),
							})
						}
						continue
					}

					if fileState.hasChanged(checksum, mod) {
						fileState.update(checksum, mod)
						lastMtime = mod
						reconcile("file_change")
					}
				}
			}
		}
	}()
}
