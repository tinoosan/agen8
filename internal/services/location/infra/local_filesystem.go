package infra

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	credentialapp "github.com/tinoosan/agen8-mcp-server/internal/services/credential/app"
	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
	filedomain "github.com/tinoosan/agen8-mcp-server/internal/services/file/domain/file"
	locationapp "github.com/tinoosan/agen8-mcp-server/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type CredentialResolver interface {
	ResolveCredential(ctx context.Context, input credentialapp.ResolveCredentialInput) (credentialdomain.ResolvedCredential, error)
}

type Transport struct {
	credentials     CredentialResolver
	dialTimeout     time.Duration
	localDaemonAddr string
	logger          *slog.Logger
}

type TransportConfig struct {
	Credentials     CredentialResolver
	DialTimeout     time.Duration
	LocalDaemonAddr string
	Logger          *slog.Logger
}

func NewTransport(config ...TransportConfig) Transport {
	cfg := TransportConfig{}
	if len(config) > 0 {
		cfg = config[0]
	}
	timeout := cfg.DialTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default().With("service", "location.transport")
	}
	return Transport{
		credentials:     cfg.Credentials,
		dialTimeout:     timeout,
		localDaemonAddr: strings.TrimSpace(cfg.LocalDaemonAddr),
		logger:          logger,
	}
}

func (t Transport) Probe(ctx context.Context, location locationdomain.Location) (locationapp.ProbeResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		codex := true
		claude := true
		message := ""
		if _, err := exec.LookPath("codex"); err != nil {
			codex = false
			message = "codex binary was not found on this machine"
		}
		if _, err := exec.LookPath("claude"); err != nil {
			claude = false
		}
		return locationapp.ProbeResult{
			Reachable:    true,
			FileBrowsing: true,
			Exec:         true,
			Codex:        codex,
			Claude:       claude,
			Status:       localProbeStatus(codex),
			FailureCode:  localProbeFailure(codex),
			Message:      message,
			ProbedAt:     time.Now().UTC(),
		}, nil
	case locationdomain.KindSSH:
		return t.probeSSH(ctx, location)
	default:
		return locationapp.ProbeResult{}, fmt.Errorf("location transport for %q is not implemented", location.Kind())
	}
}

func (t Transport) ListDir(ctx context.Context, location locationdomain.Location, path string) ([]locationapp.DirEntry, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return listLocalDir(path)
	case locationdomain.KindSSH:
		return t.listSSHDir(ctx, location, path)
	default:
		return nil, fmt.Errorf("location transport for %q is not implemented", location.Kind())
	}
}

func (t Transport) StatFile(ctx context.Context, location locationdomain.Location, path string) (filedomain.Info, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		info, err := os.Stat(path)
		if err != nil {
			return filedomain.Info{}, err
		}
		return fileInfoFromOS(info), nil
	case locationdomain.KindSSH:
		return t.statSSHFile(ctx, location, path)
	default:
		return filedomain.Info{}, fmt.Errorf("file stat for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) ListFiles(ctx context.Context, location locationdomain.Location, path string) ([]filedomain.Entry, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return listLocalFiles(path)
	case locationdomain.KindSSH:
		return t.listSSHFiles(ctx, location, path)
	default:
		return nil, fmt.Errorf("file listing for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) ReadFile(ctx context.Context, location locationdomain.Location, path string, maxBytes int64) (filedomain.Content, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return readLocalFile(path, maxBytes)
	case locationdomain.KindSSH:
		return t.readSSHFile(ctx, location, path, maxBytes)
	default:
		return filedomain.Content{}, fmt.Errorf("file read for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) CreateDir(ctx context.Context, location locationdomain.Location, path string) error {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return os.MkdirAll(path, 0o755)
	case locationdomain.KindSSH:
		return t.createSSHDir(ctx, location, path)
	default:
		return fmt.Errorf("directory creation for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) CreateFile(ctx context.Context, location locationdomain.Location, path string) error {
	switch location.Kind() {
	case locationdomain.KindLocal:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		return file.Close()
	case locationdomain.KindSSH:
		return t.createSSHFile(ctx, location, path)
	default:
		return fmt.Errorf("file creation for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) MoveFile(ctx context.Context, location locationdomain.Location, source string, destination string) error {
	switch location.Kind() {
	case locationdomain.KindLocal:
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.Rename(source, destination)
	case locationdomain.KindSSH:
		return t.moveSSHFile(ctx, location, source, destination)
	default:
		return fmt.Errorf("file move for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) CopyFile(ctx context.Context, location locationdomain.Location, source string, destination string) error {
	switch location.Kind() {
	case locationdomain.KindLocal:
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		return copyLocalPath(source, destination, info)
	case locationdomain.KindSSH:
		return t.copySSHFile(ctx, location, source, destination)
	default:
		return fmt.Errorf("file copy for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) DeleteFile(ctx context.Context, location locationdomain.Location, path string) error {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return os.RemoveAll(path)
	case locationdomain.KindSSH:
		return t.deleteSSHFile(ctx, location, path)
	default:
		return fmt.Errorf("file deletion for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) WriteFile(ctx context.Context, location locationdomain.Location, path string, contents []byte) error {
	switch location.Kind() {
	case locationdomain.KindLocal:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, contents, 0o644)
	case locationdomain.KindSSH:
		return t.writeSSHFile(ctx, location, path, contents)
	default:
		return fmt.Errorf("file write for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) InstallCodex(ctx context.Context, location locationdomain.Location) (locationapp.InstallResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return installCodexLocal(ctx)
	case locationdomain.KindSSH:
		return t.installCodexSSH(ctx, location)
	default:
		return locationapp.InstallResult{}, fmt.Errorf("codex installation for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) CodexAuthStatus(ctx context.Context, location locationdomain.Location) (locationapp.CodexAuthStatusResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return codexAuthStatusLocal(ctx)
	case locationdomain.KindSSH:
		return t.codexAuthStatusSSH(ctx, location)
	default:
		return locationapp.CodexAuthStatusResult{}, fmt.Errorf("Codex auth status for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) BeginCodexLogin(ctx context.Context, location locationdomain.Location) (locationapp.CodexLoginResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return beginCodexLoginLocal(ctx)
	case locationdomain.KindSSH:
		return t.beginCodexLoginSSH(ctx, location)
	default:
		return locationapp.CodexLoginResult{}, fmt.Errorf("Codex login for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) InstallClaude(ctx context.Context, location locationdomain.Location) (locationapp.InstallResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return installClaudeLocal(ctx)
	case locationdomain.KindSSH:
		return t.installClaudeSSH(ctx, location)
	default:
		return locationapp.InstallResult{}, fmt.Errorf("Claude Code installation for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) ClaudeAuthStatus(ctx context.Context, location locationdomain.Location) (locationapp.ClaudeAuthStatusResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return claudeAuthStatusLocal(ctx)
	case locationdomain.KindSSH:
		return t.claudeAuthStatusSSH(ctx, location)
	default:
		return locationapp.ClaudeAuthStatusResult{}, fmt.Errorf("Claude Code auth status for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) BeginClaudeLogin(ctx context.Context, location locationdomain.Location) (locationapp.ClaudeLoginResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return beginClaudeLoginLocal(ctx)
	case locationdomain.KindSSH:
		return t.beginClaudeLoginSSH(ctx, location)
	default:
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("Claude Code login for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) CompleteClaudeLogin(ctx context.Context, location locationdomain.Location, code string) (locationapp.ClaudeLoginResult, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return completeClaudeLoginLocal(ctx, code)
	case locationdomain.KindSSH:
		return t.completeClaudeLoginSSH(ctx, location, code)
	default:
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("Claude Code login completion for %q locations is not implemented", location.Kind())
	}
}

func (t Transport) StartCommand(ctx context.Context, location locationdomain.Location, spec locationapp.CommandSpec) (locationapp.CommandProcess, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return startLocalCommand(ctx, spec)
	case locationdomain.KindSSH:
		return t.startSSHCommand(ctx, location, spec)
	default:
		return nil, fmt.Errorf("location transport for %q is not implemented", location.Kind())
	}
}

func (t Transport) EnsureBridge(ctx context.Context, location locationdomain.Location) (locationapp.Bridge, error) {
	switch location.Kind() {
	case locationdomain.KindLocal:
		return locationapp.Bridge{}, fmt.Errorf("local bridge is managed by the harness runtime")
	case locationdomain.KindSSH:
		return t.ensureSSHBridge(ctx, location)
	default:
		return locationapp.Bridge{}, fmt.Errorf("location bridge for %q is not implemented", location.Kind())
	}
}

func (t Transport) probeSSH(ctx context.Context, location locationdomain.Location) (locationapp.ProbeResult, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.ProbeResult{
			Status:      locationdomain.ProbeStatusFailed,
			FailureCode: classifySSHFailure(err),
			Message:     err.Error(),
			ProbedAt:    time.Now().UTC(),
		}, err
	}
	defer client.Close()

	result := locationapp.ProbeResult{
		Reachable: true,
		ProbedAt:  time.Now().UTC(),
		Status:    locationdomain.ProbeStatusUnknown,
	}

	if _, err := runSSHCommand(ctx, client, "echo agen8-ok"); err != nil {
		result.Message = fmt.Sprintf("ssh exec probe failed: %v", err)
		result.FailureCode = locationdomain.FailureCodeExecutionMissing
		return result, nil
	}
	result.Exec = true

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		result.Message = fmt.Sprintf("ssh sftp probe failed: %v", err)
		result.FailureCode = locationdomain.FailureCodePermissionDenied
		return result, nil
	}
	if _, err := sftpClient.ReadDir("."); err != nil {
		_ = sftpClient.Close()
		result.Message = fmt.Sprintf("ssh file browsing probe failed: %v", err)
		result.FailureCode = locationdomain.FailureCodePermissionDenied
		return result, nil
	}
	_ = sftpClient.Close()
	result.FileBrowsing = true

	if _, err := runSSHCommand(ctx, client, codexDetectCommand()); err != nil {
		result.Message = codexMissingMessage(ctx, client)
		result.FailureCode = locationdomain.FailureCodeCodexMissing
		return result, nil
	}
	result.Codex = true
	if _, err := runSSHCommand(ctx, client, claudeDetectCommand()); err == nil {
		result.Claude = true
	}
	result.Status = locationdomain.ProbeStatusPassed
	return result, nil
}

func (t Transport) listSSHDir(ctx context.Context, location locationdomain.Location, path string) ([]locationapp.DirEntry, error) {
	cleanPath := cleanPath(path)
	if cleanPath == "" {
		return nil, fmt.Errorf("path is required")
	}
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("open ssh file browser: %w", err)
	}
	defer sftpClient.Close()
	if cleanPath == "~" {
		home, err := runSSHCommand(ctx, client, "printf %s \"$HOME\"")
		if err != nil {
			return nil, fmt.Errorf("resolve remote home dir: %w", err)
		}
		cleanPath = strings.TrimSpace(home)
	}
	entries, err := sftpClient.ReadDir(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("list remote directory %s: %w", cleanPath, err)
	}
	out := make([]locationapp.DirEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, locationapp.DirEntry{
			Name: entry.Name(),
			Path: remoteJoin(cleanPath, entry.Name()),
			Type: dirEntryType(entry),
			Size: entry.Size(),
		})
	}
	sortEntries(out)
	return out, nil
}

func (t Transport) statSSHFile(ctx context.Context, location locationdomain.Location, path string) (filedomain.Info, error) {
	sftpClient, closeFn, cleanPath, err := t.openSFTPPath(ctx, location, path)
	if err != nil {
		return filedomain.Info{}, err
	}
	defer closeFn()
	info, err := sftpClient.Stat(cleanPath)
	if err != nil {
		return filedomain.Info{}, err
	}
	return fileInfoFromOS(info), nil
}

func (t Transport) listSSHFiles(ctx context.Context, location locationdomain.Location, path string) ([]filedomain.Entry, error) {
	sftpClient, closeFn, cleanPath, err := t.openSFTPPath(ctx, location, path)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	entries, err := sftpClient.ReadDir(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("list remote directory %s: %w", cleanPath, err)
	}
	out := make([]filedomain.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, filedomain.Entry{Name: entry.Name(), Info: fileInfoFromOS(entry)})
	}
	sortFileEntries(out)
	return out, nil
}

func (t Transport) readSSHFile(ctx context.Context, location locationdomain.Location, path string, maxBytes int64) (filedomain.Content, error) {
	if maxBytes <= 0 {
		return filedomain.Content{}, fmt.Errorf("maxBytes is required")
	}
	sftpClient, closeFn, cleanPath, err := t.openSFTPPath(ctx, location, path)
	if err != nil {
		return filedomain.Content{}, err
	}
	defer closeFn()
	info, err := sftpClient.Stat(cleanPath)
	if err != nil {
		return filedomain.Content{}, err
	}
	file, err := sftpClient.Open(cleanPath)
	if err != nil {
		return filedomain.Content{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return filedomain.Content{}, err
	}
	truncated := int64(len(raw)) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}
	return filedomain.Content{Bytes: raw, Truncated: truncated, FileSize: info.Size()}, nil
}

func (t Transport) createSSHDir(ctx context.Context, location locationdomain.Location, path string) error {
	sftpClient, closeFn, cleanPath, err := t.openSFTPPath(ctx, location, path)
	if err != nil {
		return err
	}
	defer closeFn()
	return sftpClient.MkdirAll(cleanPath)
}

func (t Transport) createSSHFile(ctx context.Context, location locationdomain.Location, path string) error {
	sftpClient, closeFn, cleanPath, err := t.openSFTPPath(ctx, location, path)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := sftpClient.MkdirAll(pathpkg.Dir(cleanPath)); err != nil {
		return err
	}
	file, err := sftpClient.OpenFile(cleanPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	return file.Close()
}

func (t Transport) moveSSHFile(ctx context.Context, location locationdomain.Location, source string, destination string) error {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return err
	}
	defer client.Close()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("open ssh file mover: %w", err)
	}
	defer sftpClient.Close()
	cleanSource, err := resolveSSHPath(ctx, client, source)
	if err != nil {
		return err
	}
	cleanDestination, err := resolveSSHPath(ctx, client, destination)
	if err != nil {
		return err
	}
	if err := sftpClient.MkdirAll(pathpkg.Dir(cleanDestination)); err != nil {
		return err
	}
	return sftpClient.Rename(cleanSource, cleanDestination)
}

func (t Transport) copySSHFile(ctx context.Context, location locationdomain.Location, source string, destination string) error {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return err
	}
	defer client.Close()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("open ssh file copier: %w", err)
	}
	defer sftpClient.Close()
	cleanSource, err := resolveSSHPath(ctx, client, source)
	if err != nil {
		return err
	}
	cleanDestination, err := resolveSSHPath(ctx, client, destination)
	if err != nil {
		return err
	}
	info, err := sftpClient.Stat(cleanSource)
	if err != nil {
		return err
	}
	return copySFTPPath(sftpClient, cleanSource, cleanDestination, info)
}

func (t Transport) deleteSSHFile(ctx context.Context, location locationdomain.Location, path string) error {
	sftpClient, closeFn, cleanPath, err := t.openSFTPPath(ctx, location, path)
	if err != nil {
		return err
	}
	defer closeFn()
	info, err := sftpClient.Stat(cleanPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return sftpClient.Remove(cleanPath)
	}
	var paths []string
	walker := sftpClient.Walk(cleanPath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}
		paths = append(paths, walker.Path())
	}
	for i := len(paths) - 1; i >= 0; i-- {
		info, err := sftpClient.Stat(paths[i])
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := sftpClient.RemoveDirectory(paths[i]); err != nil {
				return err
			}
			continue
		}
		if err := sftpClient.Remove(paths[i]); err != nil {
			return err
		}
	}
	return nil
}

func (t Transport) writeSSHFile(ctx context.Context, location locationdomain.Location, path string, contents []byte) error {
	sftpClient, closeFn, cleanPath, err := t.openSFTPPath(ctx, location, path)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := sftpClient.MkdirAll(pathpkg.Dir(cleanPath)); err != nil {
		return err
	}
	file, err := sftpClient.OpenFile(cleanPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (t Transport) installCodexSSH(ctx context.Context, location locationdomain.Location) (locationapp.InstallResult, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.InstallResult{}, err
	}
	defer client.Close()

	command := strings.Join([]string{
		"set -e",
		"if " + codexDetectCommand() + "; then codexPath=$(resolve_codex_path); \"$codexPath\" --version; exit 0; fi",
		"if ! " + npmDetectCommand() + "; then echo 'npm is required to install Codex CLI' >&2; exit 127; fi",
		"npmPath=$(resolve_npm_path)",
		"prefix=${HOME}/.local",
		"mkdir -p \"$prefix\"",
		"\"$npmPath\" install -g @openai/codex --prefix \"$prefix\"",
		"if ! command -v \"$prefix/bin/codex\" >/dev/null 2>&1; then echo 'codex install completed but binary was not found' >&2; exit 1; fi",
		"\"$prefix/bin/codex\" --version",
	}, " && ")
	output, err := runSSHCommand(ctx, client, command)
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("install codex over ssh: %w: %s", err, strings.TrimSpace(output))
	}
	return locationapp.InstallResult{Output: strings.TrimSpace(output)}, nil
}

func installCodexLocal(ctx context.Context) (locationapp.InstallResult, error) {
	if codexPath, err := exec.LookPath("codex"); err == nil {
		output, runErr := runLocalCommand(ctx, codexPath, "--version")
		if runErr != nil {
			return locationapp.InstallResult{}, fmt.Errorf("check local codex: %w: %s", runErr, strings.TrimSpace(output))
		}
		return locationapp.InstallResult{Output: strings.TrimSpace(output)}, nil
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("npm is required to install Codex CLI locally")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("resolve home dir: %w", err)
	}
	prefix := filepath.Join(home, ".local")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("create local npm prefix: %w", err)
	}
	output, err := runLocalCommand(ctx, npmPath, "install", "-g", "@openai/codex", "--prefix", prefix)
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("install Codex CLI locally: %w: %s", err, strings.TrimSpace(output))
	}
	codexPath := filepath.Join(prefix, "bin", "codex")
	version, err := runLocalCommand(ctx, codexPath, "--version")
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("verify local Codex CLI install: %w: %s", err, strings.TrimSpace(version))
	}
	return locationapp.InstallResult{Output: strings.TrimSpace(output + "\n" + version)}, nil
}

func (t Transport) codexAuthStatusSSH(ctx context.Context, location locationdomain.Location) (locationapp.CodexAuthStatusResult, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.CodexAuthStatusResult{}, err
	}
	defer client.Close()
	command := strings.Join([]string{
		"set -e",
		codexResolveFunctionCommand(),
		"codexPath=$(resolve_codex_path)",
		"\"$codexPath\" login status",
	}, " && ")
	output, err := runSSHCommandOutput(ctx, client, command)
	if err != nil && !codexStatusLoggedOut(output) {
		return locationapp.CodexAuthStatusResult{}, fmt.Errorf("check Codex auth over ssh: %w: %s", err, strings.TrimSpace(output))
	}
	return parseCodexAuthStatus(output), nil
}

func codexAuthStatusLocal(ctx context.Context) (locationapp.CodexAuthStatusResult, error) {
	codexPath, err := exec.LookPath("codex")
	if err != nil {
		return locationapp.CodexAuthStatusResult{}, fmt.Errorf("codex binary was not found on this machine")
	}
	output, err := runLocalCommandOutput(ctx, codexPath, "login", "status")
	if err != nil && !codexStatusLoggedOut(output) {
		return locationapp.CodexAuthStatusResult{}, fmt.Errorf("check local Codex auth: %w: %s", err, strings.TrimSpace(output))
	}
	return parseCodexAuthStatus(output), nil
}

func (t Transport) beginCodexLoginSSH(ctx context.Context, location locationdomain.Location) (locationapp.CodexLoginResult, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.CodexLoginResult{}, err
	}
	defer client.Close()
	command := strings.Join([]string{
		"set -e",
		codexResolveFunctionCommand(),
		"codexPath=$(resolve_codex_path)",
		"stateDir=\"$HOME/.agen8/codex-login\"",
		"mkdir -p \"$stateDir\"",
		"logFile=\"$stateDir/login.log\"",
		"pidFile=\"$stateDir/login.pid\"",
		": > \"$logFile\"",
		"nohup \"$codexPath\" login --device-auth >\"$logFile\" 2>&1 < /dev/null & echo $! > \"$pidFile\"",
		"sleep 2",
		"printf 'pid=%s\\nlog=%s\\n' \"$(cat \"$pidFile\" 2>/dev/null || true)\" \"$logFile\"",
		"cat \"$logFile\" 2>/dev/null || true",
	}, " && ")
	output, err := runSSHCommand(ctx, client, command)
	if err != nil {
		return locationapp.CodexLoginResult{}, fmt.Errorf("begin Codex login over ssh: %w: %s", err, strings.TrimSpace(output))
	}
	return parseCodexLoginOutput(output), nil
}

func beginCodexLoginLocal(_ context.Context) (locationapp.CodexLoginResult, error) {
	codexPath, err := exec.LookPath("codex")
	if err != nil {
		return locationapp.CodexLoginResult{}, fmt.Errorf("codex binary was not found on this machine")
	}
	stateDir, err := localStateDir("codex-login")
	if err != nil {
		return locationapp.CodexLoginResult{}, err
	}
	logFile := filepath.Join(stateDir, "login.log")
	pidFile := filepath.Join(stateDir, "login.pid")
	if err := os.WriteFile(logFile, nil, 0o600); err != nil {
		return locationapp.CodexLoginResult{}, fmt.Errorf("prepare Codex login log: %w", err)
	}
	logHandle, err := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return locationapp.CodexLoginResult{}, fmt.Errorf("open Codex login log: %w", err)
	}
	cmd := exec.Command(codexPath, "login", "--device-auth")
	cmd.Stdin = nil
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	if err := cmd.Start(); err != nil {
		_ = logHandle.Close()
		return locationapp.CodexLoginResult{}, fmt.Errorf("begin local Codex login: %w", err)
	}
	_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o600)
	go func() {
		_ = cmd.Wait()
		_ = logHandle.Close()
	}()
	time.Sleep(2 * time.Second)
	output := localLoginOutput(pidFile, logFile)
	return parseCodexLoginOutput(output), nil
}

func (t Transport) installClaudeSSH(ctx context.Context, location locationdomain.Location) (locationapp.InstallResult, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.InstallResult{}, err
	}
	defer client.Close()

	command := strings.Join([]string{
		"set -e",
		"if " + claudeDetectCommand() + "; then claudePath=$(resolve_claude_path); \"$claudePath\" --version; exit 0; fi",
		"if ! " + npmDetectCommand() + "; then echo 'npm is required to install Claude Code' >&2; exit 127; fi",
		"npmPath=$(resolve_npm_path)",
		"prefix=${HOME}/.local",
		"mkdir -p \"$prefix\"",
		"\"$npmPath\" install -g @anthropic-ai/claude-code --prefix \"$prefix\"",
		"if [ ! -x \"$prefix/bin/claude\" ]; then echo 'Claude Code install completed but binary was not found' >&2; exit 1; fi",
		"\"$prefix/bin/claude\" --version",
	}, " && ")
	output, err := runSSHCommand(ctx, client, command)
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("install Claude Code over ssh: %w: %s", err, strings.TrimSpace(output))
	}
	return locationapp.InstallResult{Output: strings.TrimSpace(output)}, nil
}

func installClaudeLocal(ctx context.Context) (locationapp.InstallResult, error) {
	if claudePath, err := exec.LookPath("claude"); err == nil {
		output, runErr := runLocalCommand(ctx, claudePath, "--version")
		if runErr != nil {
			return locationapp.InstallResult{}, fmt.Errorf("check local Claude Code: %w: %s", runErr, strings.TrimSpace(output))
		}
		return locationapp.InstallResult{Output: strings.TrimSpace(output)}, nil
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("npm is required to install Claude Code locally")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("resolve home dir: %w", err)
	}
	prefix := filepath.Join(home, ".local")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("create local npm prefix: %w", err)
	}
	output, err := runLocalCommand(ctx, npmPath, "install", "-g", "@anthropic-ai/claude-code", "--prefix", prefix)
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("install Claude Code locally: %w: %s", err, strings.TrimSpace(output))
	}
	claudePath := filepath.Join(prefix, "bin", "claude")
	version, err := runLocalCommand(ctx, claudePath, "--version")
	if err != nil {
		return locationapp.InstallResult{}, fmt.Errorf("verify local Claude Code install: %w: %s", err, strings.TrimSpace(version))
	}
	return locationapp.InstallResult{Output: strings.TrimSpace(output + "\n" + version)}, nil
}

func (t Transport) claudeAuthStatusSSH(ctx context.Context, location locationdomain.Location) (locationapp.ClaudeAuthStatusResult, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.ClaudeAuthStatusResult{}, err
	}
	defer client.Close()
	command := strings.Join([]string{
		"set -e",
		claudeResolveFunctionCommand(),
		"claudePath=$(resolve_claude_path)",
		"\"$claudePath\" auth status --json",
	}, " && ")
	output, err := runSSHCommandOutput(ctx, client, command)
	if status, parseErr := parseClaudeAuthStatus(output); parseErr == nil {
		return status, nil
	}
	if err != nil {
		return locationapp.ClaudeAuthStatusResult{}, fmt.Errorf("check Claude Code auth over ssh: %w: %s", err, strings.TrimSpace(output))
	}
	return parseClaudeAuthStatus(output)
}

func claudeAuthStatusLocal(ctx context.Context) (locationapp.ClaudeAuthStatusResult, error) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return locationapp.ClaudeAuthStatusResult{}, fmt.Errorf("claude binary was not found on this machine")
	}
	output, err := runLocalCommandOutput(ctx, claudePath, "auth", "status", "--json")
	if status, parseErr := parseClaudeAuthStatus(output); parseErr == nil {
		return status, nil
	}
	if err != nil {
		return locationapp.ClaudeAuthStatusResult{}, fmt.Errorf("check local Claude Code auth: %w: %s", err, strings.TrimSpace(output))
	}
	return parseClaudeAuthStatus(output)
}

func (t Transport) beginClaudeLoginSSH(ctx context.Context, location locationdomain.Location) (locationapp.ClaudeLoginResult, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.ClaudeLoginResult{}, err
	}
	defer client.Close()
	command := strings.Join([]string{
		"set -e",
		claudeResolveFunctionCommand(),
		"claudePath=$(resolve_claude_path)",
		"stateDir=\"$HOME/.agen8/claude-login\"",
		"mkdir -p \"$stateDir\"",
		"logFile=\"$stateDir/login.log\"",
		"pidFile=\"$stateDir/login.pid\"",
		"inputFile=\"$stateDir/login.in\"",
		"if [ -s \"$pidFile\" ] && [ -f \"$inputFile\" ] && kill -0 \"$(cat \"$pidFile\")\" 2>/dev/null; then :; else : > \"$inputFile\"; : > \"$logFile\"; nohup sh -c 'tail -n +1 -f \"$2\" | \"$1\" auth login --claudeai > \"$3\" 2>&1' sh \"$claudePath\" \"$inputFile\" \"$logFile\" >/dev/null 2>&1 & echo $! > \"$pidFile\"; fi",
		"sleep 2",
		"printf 'pid=%s\\nlog=%s\\n' \"$(cat \"$pidFile\" 2>/dev/null || true)\" \"$logFile\"",
		"cat \"$logFile\" 2>/dev/null || true",
	}, " && ")
	output, err := runSSHCommand(ctx, client, command)
	if err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("begin Claude Code login over ssh: %w: %s", err, strings.TrimSpace(output))
	}
	return parseClaudeLoginOutput(output), nil
}

func beginClaudeLoginLocal(_ context.Context) (locationapp.ClaudeLoginResult, error) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("claude binary was not found on this machine")
	}
	stateDir, err := localStateDir("claude-login")
	if err != nil {
		return locationapp.ClaudeLoginResult{}, err
	}
	logFile := filepath.Join(stateDir, "login.log")
	pidFile := filepath.Join(stateDir, "login.pid")
	inputFile := filepath.Join(stateDir, "login.in")
	if err := os.WriteFile(inputFile, nil, 0o600); err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("prepare Claude login input: %w", err)
	}
	if err := os.WriteFile(logFile, nil, 0o600); err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("prepare Claude login log: %w", err)
	}
	shellScript := fmt.Sprintf("tail -n +1 -f %s | %s auth login --claudeai > %s 2>&1", shellQuote(inputFile), shellQuote(claudePath), shellQuote(logFile))
	cmd := exec.Command("sh", "-c", shellScript)
	if err := cmd.Start(); err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("begin local Claude Code login: %w", err)
	}
	_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o600)
	go func() { _ = cmd.Wait() }()
	time.Sleep(2 * time.Second)
	return parseClaudeLoginOutput(localLoginOutput(pidFile, logFile)), nil
}

func (t Transport) completeClaudeLoginSSH(ctx context.Context, location locationdomain.Location, code string) (locationapp.ClaudeLoginResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("authorization code is required")
	}
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return locationapp.ClaudeLoginResult{}, err
	}
	defer client.Close()
	command := strings.Join([]string{
		"set -e",
		"stateDir=\"$HOME/.agen8/claude-login\"",
		"logFile=\"$stateDir/login.log\"",
		"pidFile=\"$stateDir/login.pid\"",
		"inputFile=\"$stateDir/login.in\"",
		"if [ ! -s \"$pidFile\" ] || ! kill -0 \"$(cat \"$pidFile\")\" 2>/dev/null; then echo 'Claude login is not running; start login again.' >&2; exit 1; fi",
		"if [ ! -f \"$inputFile\" ]; then echo 'Claude login input file is missing; start login again.' >&2; exit 1; fi",
		"cat >> \"$inputFile\"",
		"sleep 3",
		"printf 'pid=%s\\nlog=%s\\n' \"$(cat \"$pidFile\" 2>/dev/null || true)\" \"$logFile\"",
		"cat \"$logFile\" 2>/dev/null || true",
	}, " && ")
	output, err := runSSHCommandWithInput(ctx, client, command, code+"\n")
	if err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("complete Claude Code login over ssh: %w: %s", err, strings.TrimSpace(output))
	}
	return parseClaudeLoginOutput(output), nil
}

func completeClaudeLoginLocal(_ context.Context, code string) (locationapp.ClaudeLoginResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("authorization code is required")
	}
	stateDir, err := localStateDir("claude-login")
	if err != nil {
		return locationapp.ClaudeLoginResult{}, err
	}
	inputFile := filepath.Join(stateDir, "login.in")
	pidFile := filepath.Join(stateDir, "login.pid")
	logFile := filepath.Join(stateDir, "login.log")
	if _, err := os.Stat(inputFile); err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("Claude login input file is missing; start login again")
	}
	f, err := os.OpenFile(inputFile, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("open Claude login input: %w", err)
	}
	if _, err := io.WriteString(f, code+"\n"); err != nil {
		_ = f.Close()
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("write Claude login code: %w", err)
	}
	if err := f.Close(); err != nil {
		return locationapp.ClaudeLoginResult{}, fmt.Errorf("close Claude login input: %w", err)
	}
	time.Sleep(3 * time.Second)
	return parseClaudeLoginOutput(localLoginOutput(pidFile, logFile)), nil
}

func claudeDetectCommand() string {
	return claudeResolveFunctionCommand() + "; resolve_claude_path >/dev/null"
}

func claudeResolveFunctionCommand() string {
	return "setopt NULL_GLOB 2>/dev/null || true; resolve_claude_path() { " +
		"for p in \"$HOME/.local/bin/claude\" \"$HOME/.npm-global/bin/claude\" \"$HOME/.yarn/bin/claude\" \"$HOME/.bun/bin/claude\" \"$HOME/.volta/bin/claude\" \"$HOME/.asdf/shims/claude\" \"$HOME/.local/share/mise/shims/claude\" \"$HOME/.linuxbrew/bin/claude\" \"$HOME/.homebrew/bin/claude\" /home/linuxbrew/.linuxbrew/bin/claude /usr/local/bin/claude /opt/homebrew/bin/claude /snap/bin/claude \"$HOME/.nvm/versions/node\"/*/bin/claude \"$HOME/.asdf/installs/nodejs\"/*/bin/claude \"$HOME/.local/share/mise/installs/node\"/*/bin/claude \"$HOME/.local/share/fnm/node-versions\"/*/installation/bin/claude; do [ -x \"$p\" ] && printf '%s\\n' \"$p\" && return 0; done; " +
		"case \"${SHELL:-}\" in */bash|*/zsh|*/fish) p=$(\"${SHELL}\" -lc 'command -v claude' 2>/dev/null || true); [ -n \"$p\" ] && printf '%s\\n' \"$p\" && return 0 ;; esac; " +
		"if command -v claude >/dev/null 2>&1; then command -v claude; return 0; fi; " +
		"found=$(find \"$HOME\" -maxdepth 8 -name claude -print 2>/dev/null | while IFS= read -r p; do [ -x \"$p\" ] && printf '%s\\n' \"$p\" && break; done); [ -n \"$found\" ] && printf '%s\\n' \"$found\" && return 0; " +
		"echo 'claude binary was not found on the ssh location' >&2; return 1; " +
		"}"
}

func parseClaudeAuthStatus(output string) (locationapp.ClaudeAuthStatusResult, error) {
	raw := strings.TrimSpace(output)
	var payload struct {
		LoggedIn    bool   `json:"loggedIn"`
		AuthMethod  string `json:"authMethod"`
		APIProvider string `json:"apiProvider"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return locationapp.ClaudeAuthStatusResult{}, fmt.Errorf("parse Claude Code auth status: %w: %s", err, raw)
	}
	return locationapp.ClaudeAuthStatusResult{
		LoggedIn:   payload.LoggedIn,
		AuthMethod: strings.TrimSpace(payload.AuthMethod),
		Provider:   strings.TrimSpace(payload.APIProvider),
		RawJSON:    raw,
	}, nil
}

func parseClaudeLoginOutput(output string) locationapp.ClaudeLoginResult {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	result := locationapp.ClaudeLoginResult{Output: strings.TrimSpace(output)}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid=") {
			result.PID = strings.TrimSpace(strings.TrimPrefix(line, "pid="))
		}
		if strings.HasPrefix(line, "log=") {
			result.LogPath = strings.TrimSpace(strings.TrimPrefix(line, "log="))
		}
		if idx := strings.Index(line, "https://"); idx >= 0 {
			result.LoginURL = strings.TrimSpace(line[idx:])
		}
	}
	return result
}

func parseCodexAuthStatus(output string) locationapp.CodexAuthStatusResult {
	raw := strings.TrimSpace(output)
	lower := strings.ToLower(raw)
	result := locationapp.CodexAuthStatusResult{Output: raw}
	if strings.Contains(lower, "logged in") {
		result.LoggedIn = true
	}
	if strings.Contains(lower, "chatgpt") {
		result.Method = "account"
	}
	if strings.Contains(lower, "api key") {
		result.Method = "api_key"
	}
	return result
}

func codexStatusLoggedOut(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not logged in") || strings.Contains(lower, "login required")
}

func parseCodexLoginOutput(output string) locationapp.CodexLoginResult {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	result := locationapp.CodexLoginResult{Output: strings.TrimSpace(output)}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid=") {
			result.PID = strings.TrimSpace(strings.TrimPrefix(line, "pid="))
		}
		if strings.HasPrefix(line, "log=") {
			result.LogPath = strings.TrimSpace(strings.TrimPrefix(line, "log="))
		}
		if idx := strings.Index(line, "https://"); idx >= 0 {
			result.LoginURL = strings.TrimSpace(line[idx:])
		}
	}
	return result
}

func codexDetectCommand() string {
	return codexResolveFunctionCommand() + "; resolve_codex_path >/dev/null"
}

func codexResolveFunctionCommand() string {
	return "setopt NULL_GLOB 2>/dev/null || true; resolve_codex_path() { " +
		"is_bundled_editor_codex() { case \"$1\" in *'/.cursor-server/extensions/'*|*'/.vscode-server/extensions/'*) return 0 ;; *) return 1 ;; esac; }; " +
		"for p in \"$HOME/.local/bin/codex\" \"$HOME/.npm-global/bin/codex\" \"$HOME/.yarn/bin/codex\" \"$HOME/.bun/bin/codex\" \"$HOME/.volta/bin/codex\" \"$HOME/.asdf/shims/codex\" \"$HOME/.local/share/mise/shims/codex\" \"$HOME/.linuxbrew/bin/codex\" \"$HOME/.homebrew/bin/codex\" /home/linuxbrew/.linuxbrew/bin/codex /usr/local/bin/codex /opt/homebrew/bin/codex /snap/bin/codex \"$HOME/.nvm/versions/node\"/*/bin/codex \"$HOME/.asdf/installs/nodejs\"/*/bin/codex \"$HOME/.local/share/mise/installs/node\"/*/bin/codex \"$HOME/.local/share/fnm/node-versions\"/*/installation/bin/codex; do [ -x \"$p\" ] && printf '%s\\n' \"$p\" && return 0; done; " +
		"case \"${SHELL:-}\" in */bash|*/zsh|*/fish) p=$(\"${SHELL}\" -lc 'command -v codex' 2>/dev/null || true); [ -n \"$p\" ] && ! is_bundled_editor_codex \"$p\" && printf '%s\\n' \"$p\" && return 0 ;; esac; " +
		"if command -v codex >/dev/null 2>&1; then p=$(command -v codex); ! is_bundled_editor_codex \"$p\" && printf '%s\\n' \"$p\" && return 0; fi; " +
		"found=$(find \"$HOME\" -maxdepth 8 -name codex -print 2>/dev/null | while IFS= read -r p; do [ -x \"$p\" ] || continue; is_bundled_editor_codex \"$p\" && continue; printf '%s\\n' \"$p\"; break; done); [ -n \"$found\" ] && printf '%s\\n' \"$found\" && return 0; " +
		"return 1; " +
		"}"
}

func npmDetectCommand() string {
	return "setopt NULL_GLOB 2>/dev/null || true; resolve_npm_path() { " +
		"if command -v npm >/dev/null 2>&1; then command -v npm; return 0; fi; " +
		"for p in \"$HOME/.local/bin/npm\" \"$HOME/.npm-global/bin/npm\" \"$HOME/.volta/bin/npm\" \"$HOME/.asdf/shims/npm\" \"$HOME/.local/share/mise/shims/npm\" /usr/local/bin/npm /opt/homebrew/bin/npm \"$HOME/.nvm/versions/node\"/*/bin/npm \"$HOME/.asdf/installs/nodejs\"/*/bin/npm \"$HOME/.local/share/mise/installs/node\"/*/bin/npm \"$HOME/.local/share/fnm/node-versions\"/*/installation/bin/npm; do [ -x \"$p\" ] && printf '%s\\n' \"$p\" && return 0; done; " +
		"case \"${SHELL:-}\" in */bash|*/zsh|*/fish) \"${SHELL}\" -lc 'command -v npm' 2>/dev/null && return 0 ;; esac; " +
		"found=$(find \"$HOME\" -maxdepth 8 -name npm -print 2>/dev/null | while IFS= read -r p; do [ -x \"$p\" ] && printf '%s\\n' \"$p\" && break; done); [ -n \"$found\" ] && printf '%s\\n' \"$found\" && return 0; " +
		"return 1; " +
		"}; resolve_npm_path >/dev/null"
}

func codexMissingMessage(ctx context.Context, client *ssh.Client) string {
	diagnostic := strings.Join([]string{
		"printf 'PATH=%s\\nSHELL=%s\\nHOME=%s\\n' \"$PATH\" \"${SHELL:-}\" \"$HOME\"",
		"printf 'command_v='; command -v codex || true",
		"if [ -n \"${SHELL:-}\" ]; then printf 'login_command_v='; \"${SHELL}\" -lc 'command -v codex' 2>/dev/null || true; fi",
		"printf 'found_candidates=\\n'",
		"find \"$HOME\" -maxdepth 8 -name codex -print 2>/dev/null | head -20",
	}, "; ")
	output, err := runSSHCommand(ctx, client, diagnostic)
	if err != nil {
		return "codex binary was not found on the ssh location"
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return "codex binary was not found on the ssh location"
	}
	if len(output) > 1200 {
		output = output[:1200] + "..."
	}
	return "codex binary was not found on the ssh location; remote diagnostic: " + output
}

func listLocalDir(path string) ([]locationapp.DirEntry, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, fmt.Errorf("path is required")
	}
	if cleanPath == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		cleanPath = home
	}
	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("list directory %s: %w", cleanPath, err)
	}
	out := make([]locationapp.DirEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat directory entry %s: %w", entry.Name(), err)
		}
		out = append(out, locationapp.DirEntry{
			Name: entry.Name(),
			Path: filepath.Join(cleanPath, entry.Name()),
			Type: dirEntryType(info),
			Size: info.Size(),
		})
	}
	sortEntries(out)
	return out, nil
}

func listLocalFiles(path string) ([]filedomain.Entry, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, fmt.Errorf("path is required")
	}
	items, err := os.ReadDir(cleanPath)
	if err != nil {
		return nil, err
	}
	entries := make([]filedomain.Entry, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			return nil, fmt.Errorf("stat directory entry %s: %w", item.Name(), err)
		}
		entries = append(entries, filedomain.Entry{Name: item.Name(), Info: fileInfoFromOS(info)})
	}
	sortFileEntries(entries)
	return entries, nil
}

func readLocalFile(path string, maxBytes int64) (filedomain.Content, error) {
	if maxBytes <= 0 {
		return filedomain.Content{}, fmt.Errorf("maxBytes is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return filedomain.Content{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return filedomain.Content{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return filedomain.Content{}, err
	}
	truncated := int64(len(raw)) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}
	return filedomain.Content{Bytes: raw, Truncated: truncated, FileSize: info.Size()}, nil
}

func copyLocalPath(src string, dst string, info os.FileInfo) error {
	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			entryInfo, err := entry.Info()
			if err != nil {
				return err
			}
			return copyLocalFile(path, target, entryInfo.Mode())
		})
	}
	return copyLocalFile(src, dst, info.Mode())
}

func copyLocalFile(src string, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func copySFTPPath(client *sftp.Client, src string, dst string, info os.FileInfo) error {
	if info.IsDir() {
		walker := client.Walk(src)
		for walker.Step() {
			if err := walker.Err(); err != nil {
				return err
			}
			rel := "."
			if walker.Path() != src {
				rel = strings.TrimPrefix(walker.Path(), strings.TrimRight(src, "/")+"/")
			}
			target := pathpkg.Join(dst, rel)
			childInfo, err := client.Stat(walker.Path())
			if err != nil {
				return err
			}
			if childInfo.IsDir() {
				if err := client.MkdirAll(target); err != nil {
					return err
				}
				continue
			}
			if err := copySFTPFile(client, walker.Path(), target); err != nil {
				return err
			}
		}
		return nil
	}
	return copySFTPFile(client, src, dst)
}

func copySFTPFile(client *sftp.Client, src string, dst string) error {
	if err := client.MkdirAll(pathpkg.Dir(dst)); err != nil {
		return err
	}
	in, err := client.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := client.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (t Transport) dialSSH(ctx context.Context, location locationdomain.Location) (*ssh.Client, error) {
	address := location.Address()
	if strings.TrimSpace(address.Host) == "" {
		return nil, fmt.Errorf("ssh host is required")
	}
	if strings.TrimSpace(address.Username) == "" {
		return nil, fmt.Errorf("ssh username is required")
	}
	if address.Port <= 0 {
		return nil, fmt.Errorf("ssh port is required")
	}
	authMethods, err := t.sshAuthMethods(ctx, location)
	if err != nil {
		return nil, err
	}
	hostKeyCallback, err := knownHostsCallback()
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            strings.TrimSpace(address.Username),
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         t.dialTimeout,
	}
	endpoint := net.JoinHostPort(strings.TrimSpace(address.Host), fmt.Sprintf("%d", address.Port))
	dialer := net.Dialer{Timeout: t.dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return nil, fmt.Errorf("connect ssh %s: %w", endpoint, err)
	}
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, endpoint, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("authenticate ssh %s: %w", endpoint, err)
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
}

func (t Transport) sshAuthMethods(ctx context.Context, location locationdomain.Location) ([]ssh.AuthMethod, error) {
	record := location.Record()
	credentialID := strings.TrimSpace(record.CredentialRef)
	if credentialID == "" {
		return sshAgentAuth()
	}
	if t.credentials == nil {
		return nil, fmt.Errorf("location credential resolver is required")
	}
	resolved, err := t.credentials.ResolveCredential(ctx, credentialapp.ResolveCredentialInput{
		CredentialID: credentialdomain.ID(credentialID),
		Purpose:      credentialdomain.PurposeLocationSSH,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve location ssh credential %s: %w", credentialID, err)
	}
	switch resolved.Kind {
	case credentialdomain.KindSSHAgent:
		return sshAgentAuth()
	case credentialdomain.KindSSHKey:
		privateKey := strings.TrimSpace(resolved.Values["privateKey"])
		if privateKey == "" {
			return nil, fmt.Errorf("ssh_key credential %s is missing privateKey", credentialID)
		}
		passphrase := strings.TrimSpace(resolved.Values["passphrase"])
		var signer ssh.Signer
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(privateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key %s: %w", credentialID, err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	case credentialdomain.KindSSHPassword:
		password := strings.TrimSpace(resolved.Values["password"])
		if password == "" {
			return nil, fmt.Errorf("ssh_password credential %s is missing password", credentialID)
		}
		return []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = password
				}
				return answers, nil
			}),
		}, nil
	default:
		return nil, fmt.Errorf("credential kind %q cannot be used for ssh", resolved.Kind)
	}
}

func sshAgentAuth() ([]ssh.AuthMethod, error) {
	socket := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if socket == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK is required for ssh agent auth")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("connect ssh agent: %w", err)
	}
	agentClient := agent.NewClient(conn)
	signers, err := agentClient.Signers()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("list ssh agent signers: %w", err)
	}
	if len(signers) == 0 {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh agent has no loaded identities")
	}
	return []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)}, nil
}

func knownHostsCallback() (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir for known_hosts: %w", err)
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("ssh known_hosts file %s is required", path)
		}
		return nil, fmt.Errorf("stat ssh known_hosts %s: %w", path, err)
	}
	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("load ssh known_hosts %s: %w", path, err)
	}
	return callback, nil
}

func runSSHCommand(ctx context.Context, client *ssh.Client, command string) (string, error) {
	output, err := runSSHCommandOutput(ctx, client, command)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(output))
	}
	return output, nil
}

func runSSHCommandOutput(ctx context.Context, client *ssh.Client, command string) (string, error) {
	return runSSHCommandWithInput(ctx, client, command, "")
}

func runSSHCommandWithInput(ctx context.Context, client *ssh.Client, command string, input string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	if input != "" {
		session.Stdin = strings.NewReader(input)
	}
	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := session.CombinedOutput(command)
		done <- result{output: output, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return "", ctx.Err()
	case result := <-done:
		return string(result.output), result.err
	}
}

func runLocalCommand(ctx context.Context, command string, args ...string) (string, error) {
	output, err := runLocalCommandOutput(ctx, command, args...)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(output))
	}
	return output, nil
}

func runLocalCommandOutput(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func localStateDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".agen8", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create setup state dir: %w", err)
	}
	return dir, nil
}

func localLoginOutput(pidFile string, logFile string) string {
	pid, _ := os.ReadFile(pidFile)
	log, _ := os.ReadFile(logFile)
	return fmt.Sprintf("pid=%s\nlog=%s\n%s", strings.TrimSpace(string(pid)), logFile, string(log))
}

func localProbeStatus(codex bool) locationdomain.ProbeStatus {
	if codex {
		return locationdomain.ProbeStatusPassed
	}
	return locationdomain.ProbeStatusFailed
}

func localProbeFailure(codex bool) locationdomain.FailureCode {
	if codex {
		return ""
	}
	return locationdomain.FailureCodeCodexMissing
}

func startLocalCommand(ctx context.Context, spec locationapp.CommandSpec) (locationapp.CommandProcess, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	if dir := strings.TrimSpace(spec.Workdir); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(cmd.Environ(), spec.Env...)
	return &localCommandProcess{cmd: cmd, stderr: &bytes.Buffer{}}, nil
}

type localCommandProcess struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (p *localCommandProcess) StdinPipe() (io.WriteCloser, error) { return p.cmd.StdinPipe() }
func (p *localCommandProcess) StdoutPipe() (io.ReadCloser, error) { return p.cmd.StdoutPipe() }
func (p *localCommandProcess) StderrText() string                 { return p.stderr.String() }
func (p *localCommandProcess) Start() error {
	p.cmd.Stderr = p.stderr
	return p.cmd.Start()
}
func (p *localCommandProcess) Wait() error { return p.cmd.Wait() }
func (p *localCommandProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func (t Transport) startSSHCommand(ctx context.Context, location locationdomain.Location, spec locationapp.CommandSpec) (locationapp.CommandProcess, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &sshCommandProcess{
		ctx:     ctx,
		client:  client,
		session: session,
		command: shellCommand(spec),
		stderr:  &bytes.Buffer{},
	}, nil
}

type sshCommandProcess struct {
	ctx     context.Context
	client  *ssh.Client
	session *ssh.Session
	command string
	stderr  *bytes.Buffer
	done    chan error
}

func (p *sshCommandProcess) StdinPipe() (io.WriteCloser, error) {
	return p.session.StdinPipe()
}

func (p *sshCommandProcess) StdoutPipe() (io.ReadCloser, error) {
	stdout, err := p.session.StdoutPipe()
	if err != nil {
		return nil, err
	}
	return io.NopCloser(stdout), nil
}

func (p *sshCommandProcess) StderrText() string {
	if p == nil || p.stderr == nil {
		return ""
	}
	return p.stderr.String()
}

func (p *sshCommandProcess) Start() error {
	p.session.Stderr = p.stderr
	if err := p.session.Start(p.command); err != nil {
		_ = p.session.Close()
		_ = p.client.Close()
		return err
	}
	p.done = make(chan error, 1)
	go func() {
		p.done <- p.session.Wait()
	}()
	return nil
}

func (p *sshCommandProcess) Wait() error {
	if p.done == nil {
		return fmt.Errorf("ssh command was not started")
	}
	select {
	case <-p.ctx.Done():
		_ = p.Kill()
		return p.ctx.Err()
	case err := <-p.done:
		_ = p.session.Close()
		_ = p.client.Close()
		return err
	}
}

func (p *sshCommandProcess) Kill() error {
	if p == nil {
		return nil
	}
	if p.session != nil {
		_ = p.session.Signal(ssh.SIGKILL)
		_ = p.session.Close()
	}
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

func shellCommand(spec locationapp.CommandSpec) string {
	parts := []string{"set -e"}
	if dir := strings.TrimSpace(spec.Workdir); dir != "" {
		parts = append(parts, "cd "+shellQuote(dir))
	}
	for _, env := range spec.Env {
		key, value, ok := strings.Cut(env, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		parts = append(parts, "export "+key+"="+shellQuote(value))
	}
	command := strings.TrimSpace(spec.Command)
	cmd := shellQuote(command)
	if strings.TrimSpace(filepath.Base(command)) == "codex" {
		parts = append(parts, codexDetectCommand(), "codexPath=$(resolve_codex_path)")
		cmd = "\"$codexPath\""
	}
	for _, arg := range spec.Args {
		cmd += " " + shellQuote(arg)
	}
	parts = append(parts, "exec "+cmd)
	return strings.Join(parts, " && ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type tcpForwardSpec struct {
	RemoteHost string
	RemotePort int
}

func (t Transport) forwardSSHTCP(ctx context.Context, location locationdomain.Location, spec tcpForwardSpec) (locationapp.TCPForward, error) {
	remoteHost := strings.TrimSpace(spec.RemoteHost)
	if remoteHost == "" {
		return nil, fmt.Errorf("remote host is required")
	}
	if spec.RemotePort <= 0 {
		return nil, fmt.Errorf("remote port is required")
	}
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("listen local tcp forward: %w", err)
	}
	forward := &sshTCPForward{
		ctx:        ctx,
		client:     client,
		listener:   listener,
		remoteAddr: net.JoinHostPort(remoteHost, fmt.Sprint(spec.RemotePort)),
		done:       make(chan struct{}),
	}
	go forward.acceptLoop()
	return forward, nil
}

func (t Transport) ensureSSHBridge(ctx context.Context, location locationdomain.Location) (locationapp.Bridge, error) {
	started := time.Now()
	address := location.Address()
	t.logger.InfoContext(ctx, "ssh bridge ensure starting",
		"location_id", location.ID(),
		"host", address.Host,
		"port", address.Port,
		"username", address.Username,
	)
	port, err := reserveLocalTCPPort()
	if err != nil {
		t.logger.ErrorContext(ctx, "ssh bridge reserve remote port failed", "location_id", location.ID(), "error", err)
		return locationapp.Bridge{}, err
	}
	remotePort := port
	t.logger.InfoContext(ctx, "ssh bridge launching remote process", "location_id", location.ID(), "remote_port", remotePort)
	remotePort, err = t.launchSSHBridge(ctx, location, remotePort)
	if err != nil {
		t.logger.ErrorContext(ctx, "ssh bridge launch failed", "location_id", location.ID(), "remote_port", remotePort, "error", err)
		return locationapp.Bridge{}, err
	}
	t.logger.InfoContext(ctx, "ssh bridge forward starting", "location_id", location.ID(), "remote_port", remotePort)
	forward, err := t.forwardSSHTCP(ctx, location, tcpForwardSpec{
		RemoteHost: "127.0.0.1",
		RemotePort: remotePort,
	})
	if err != nil {
		t.logger.ErrorContext(ctx, "ssh bridge forward failed", "location_id", location.ID(), "remote_port", remotePort, "error", err)
		return locationapp.Bridge{}, err
	}
	localAddr := strings.TrimSpace(forward.LocalAddr())
	if localAddr == "" {
		_ = forward.Close()
		t.logger.ErrorContext(ctx, "ssh bridge forward returned no local address", "location_id", location.ID(), "remote_port", remotePort)
		return locationapp.Bridge{}, fmt.Errorf("ssh bridge tcp forward returned no local address")
	}
	baseURL := "http://" + localAddr
	t.logger.InfoContext(ctx, "ssh bridge ready check starting", "location_id", location.ID(), "base_url", baseURL)
	if err := waitForHTTPReady(ctx, baseURL+"/readyz"); err != nil {
		_ = forward.Close()
		diagnostics := strings.TrimSpace(t.sshBridgeLaunchDiagnostics(ctx, location))
		t.logger.ErrorContext(ctx, "ssh bridge ready check failed", "location_id", location.ID(), "base_url", baseURL, "error", err, "remote_diagnostics", diagnostics)
		if diagnostics != "" {
			return locationapp.Bridge{}, fmt.Errorf("ssh bridge ready: %w; remote diagnostics:\n%s", err, diagnostics)
		}
		return locationapp.Bridge{}, fmt.Errorf("ssh bridge ready: %w", err)
	}
	diagnostics, err := bridgeDiagnostics(ctx, baseURL)
	if err != nil {
		_ = forward.Close()
		t.logger.ErrorContext(ctx, "ssh bridge diagnostics failed", "location_id", location.ID(), "base_url", baseURL, "error", err)
		return locationapp.Bridge{}, fmt.Errorf("ssh bridge diagnostics: %w", err)
	}
	t.logger.InfoContext(ctx, "ssh bridge diagnostics ready", "location_id", location.ID(), "diagnostics", diagnostics)
	t.logger.InfoContext(ctx, "ssh bridge reverse mcp tunnel starting", "location_id", location.ID(), "local_daemon_addr", strings.TrimSpace(t.localDaemonAddr))
	mcpForward, err := t.reverseSSHTCP(ctx, location, strings.TrimSpace(t.localDaemonAddr))
	if err != nil {
		_ = forward.Close()
		t.logger.ErrorContext(ctx, "ssh bridge reverse mcp tunnel failed", "location_id", location.ID(), "error", err)
		return locationapp.Bridge{}, fmt.Errorf("ssh bridge mcp reverse tunnel: %w", err)
	}
	mcpAddr := strings.TrimSpace(mcpForward.LocalAddr())
	if mcpAddr == "" {
		_ = forward.Close()
		_ = mcpForward.Close()
		t.logger.ErrorContext(ctx, "ssh bridge reverse mcp tunnel returned no remote address", "location_id", location.ID())
		return locationapp.Bridge{}, fmt.Errorf("ssh bridge mcp reverse tunnel returned no remote address")
	}
	t.logger.InfoContext(ctx, "ssh bridge ensure complete",
		"location_id", location.ID(),
		"base_url", baseURL,
		"mcp_remote_addr", mcpAddr,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return locationapp.Bridge{
		BaseURL:      baseURL,
		WebSocketURL: "ws://" + localAddr,
		MCPBaseURL:   "http://" + mcpAddr,
		Diagnostics:  diagnostics,
	}, nil
}

func bridgeDiagnostics(ctx context.Context, baseURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/diagnostics", nil)
	if err != nil {
		return "", err
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bridge diagnostics returned %s: %s", resp.Status, text)
	}
	if text == "" {
		return "", fmt.Errorf("bridge diagnostics returned an empty response")
	}
	return text, nil
}

func (t Transport) launchSSHBridge(ctx context.Context, location locationdomain.Location, remotePort int) (int, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	remoteBinary, err := t.ensureSSHBridgeBinary(ctx, client)
	if err != nil {
		return 0, err
	}
	command := sshBridgeLaunchCommand(string(location.ID()), remoteBinary, remotePort)
	output, err := runSSHCommand(ctx, client, command)
	if err != nil {
		return 0, fmt.Errorf("launch ssh bridge: %w: %s", err, strings.TrimSpace(output))
	}
	actualPort, err := parseSSHBridgeLaunchPort(output)
	if err != nil {
		return 0, err
	}
	return actualPort, nil
}

func (t Transport) sshBridgeLaunchDiagnostics(ctx context.Context, location locationdomain.Location) string {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return "failed to collect remote bridge diagnostics: " + err.Error()
	}
	defer client.Close()
	output, err := runSSHCommandOutput(ctx, client, sshBridgeDiagnosticsCommand(string(location.ID())))
	output = strings.TrimSpace(output)
	if err != nil {
		if output == "" {
			return "failed to collect remote bridge diagnostics: " + err.Error()
		}
		return strings.TrimSpace(output + "\nfailed to collect remote bridge diagnostics: " + err.Error())
	}
	return output
}

func sshBridgeLaunchCommand(locationID, remoteBinary string, remotePort int) string {
	stateDir := "$HOME/.agen8/ssh-launch/" + shellQuote(locationID)
	return strings.Join([]string{
		"set -e",
		"STATE_DIR=" + stateDir,
		"mkdir -p \"$STATE_DIR\"",
		"LOG_FILE=\"$STATE_DIR/bridge.log\"",
		"PID_FILE=\"$STATE_DIR/bridge.pid\"",
		"PORT_FILE=\"$STATE_DIR/bridge.port\"",
		"BINARY_FILE=\"$STATE_DIR/bridge.binary\"",
		codexResolveFunctionCommand(),
		"CODEX_BIN=$(resolve_codex_path 2>/dev/null || true)",
		fmt.Sprintf("EXPECTED_BINARY=%s", shellQuote(remoteBinary)),
		"if [ -f \"$PID_FILE\" ]; then PID=$(cat \"$PID_FILE\" 2>/dev/null || true); RUNNING_BINARY=$(cat \"$BINARY_FILE\" 2>/dev/null || true); if [ -n \"$PID\" ] && kill -0 \"$PID\" 2>/dev/null && [ -s \"$PORT_FILE\" ] && [ \"$RUNNING_BINARY\" = \"$EXPECTED_BINARY\" ]; then printf 'port=%s\\n' \"$(cat \"$PORT_FILE\")\"; exit 0; fi; if [ -n \"$PID\" ] && kill -0 \"$PID\" 2>/dev/null; then kill \"$PID\" 2>/dev/null || true; fi; rm -f \"$PID_FILE\" \"$PORT_FILE\" \"$BINARY_FILE\"; fi",
		fmt.Sprintf("printf '%%s\\n' %s > \"$PORT_FILE\"", shellQuote(fmt.Sprint(remotePort))),
		"printf '%s\\n' \"$EXPECTED_BINARY\" > \"$BINARY_FILE\"",
		fmt.Sprintf("AGEN8_CODEX_BIN=\"$CODEX_BIN\" nohup %s serve --http-addr 127.0.0.1:%d >>\"$LOG_FILE\" 2>&1 < /dev/null & echo $! > \"$PID_FILE\"", shellQuote(remoteBinary), remotePort),
		"printf 'port=%s\\n' \"$(cat \"$PORT_FILE\")\"",
	}, " && ")
}

func sshBridgeDiagnosticsCommand(locationID string) string {
	stateDir := "$HOME/.agen8/ssh-launch/" + shellQuote(locationID)
	return strings.Join([]string{
		"STATE_DIR=" + stateDir,
		"LOG_FILE=\"$STATE_DIR/bridge.log\"",
		"PID_FILE=\"$STATE_DIR/bridge.pid\"",
		"PORT_FILE=\"$STATE_DIR/bridge.port\"",
		"BINARY_FILE=\"$STATE_DIR/bridge.binary\"",
		"printf 'pid_file=%s\\n' \"$PID_FILE\"",
		"if [ -f \"$PID_FILE\" ]; then PID=$(cat \"$PID_FILE\" 2>/dev/null || true); printf 'pid=%s\\n' \"$PID\"; if [ -n \"$PID\" ] && kill -0 \"$PID\" 2>/dev/null; then printf 'process=running\\n'; else printf 'process=not_running\\n'; fi; else printf 'pid=\\nprocess=no_pid_file\\n'; fi",
		"if [ -f \"$PORT_FILE\" ]; then printf 'port=%s\\n' \"$(cat \"$PORT_FILE\")\"; else printf 'port=\\n'; fi",
		"if [ -f \"$BINARY_FILE\" ]; then printf 'binary=%s\\n' \"$(cat \"$BINARY_FILE\")\"; else printf 'binary=\\n'; fi",
		"printf 'log_file=%s\\n' \"$LOG_FILE\"",
		"if [ -f \"$LOG_FILE\" ]; then tail -n 80 \"$LOG_FILE\"; else printf 'bridge log missing\\n'; fi",
	}, " && ")
}

func parseSSHBridgeLaunchPort(output string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "port=") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "port="))
		port, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("parse ssh bridge launch port %q: %w", raw, err)
		}
		if port <= 0 {
			return 0, fmt.Errorf("parse ssh bridge launch port %q: port must be positive", raw)
		}
		return port, nil
	}
	return 0, fmt.Errorf("ssh bridge launch did not return a port: %s", strings.TrimSpace(output))
}

func (t Transport) ensureSSHBridgeBinary(ctx context.Context, client *ssh.Client) (string, error) {
	platform, err := detectSSHPlatform(ctx, client)
	if err != nil {
		return "", err
	}
	localBinary, err := ensureLocalBridgeBinary(ctx, platform)
	if err != nil {
		return "", err
	}
	home, err := runSSHCommand(ctx, client, "printf %s \"$HOME\"")
	if err != nil {
		return "", fmt.Errorf("resolve ssh home for bridge upload: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("ssh home for bridge upload is empty")
	}
	hash, err := fileSHA256Prefix(localBinary, 12)
	if err != nil {
		return "", err
	}
	remoteBinary := strings.TrimRight(home, "/") + "/.agen8/bin/agen8-bridge-" + hash
	if err := uploadSSHFile(client, localBinary, remoteBinary, 0o755); err != nil {
		return "", err
	}
	return remoteBinary, nil
}

type bridgePlatform struct {
	GOOS   string
	GOARCH string
}

func detectSSHPlatform(ctx context.Context, client *ssh.Client) (bridgePlatform, error) {
	output, err := runSSHCommand(ctx, client, "printf '%s %s' \"$(uname -s)\" \"$(uname -m)\"")
	if err != nil {
		return bridgePlatform{}, fmt.Errorf("detect ssh bridge platform: %w", err)
	}
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return bridgePlatform{}, fmt.Errorf("detect ssh bridge platform: expected uname output with 2 fields, got %q", strings.TrimSpace(output))
	}
	platform, err := normalizeBridgePlatform(fields[0], fields[1])
	if err != nil {
		return bridgePlatform{}, err
	}
	return platform, nil
}

func normalizeBridgePlatform(kernel, machine string) (bridgePlatform, error) {
	goos := ""
	switch strings.ToLower(strings.TrimSpace(kernel)) {
	case "darwin":
		goos = "darwin"
	case "linux":
		goos = "linux"
	default:
		return bridgePlatform{}, fmt.Errorf("unsupported bridge remote os %q", kernel)
	}
	goarch := ""
	switch strings.ToLower(strings.TrimSpace(machine)) {
	case "x86_64", "amd64":
		goarch = "amd64"
	case "aarch64", "arm64":
		goarch = "arm64"
	default:
		return bridgePlatform{}, fmt.Errorf("unsupported bridge remote arch %q", machine)
	}
	return bridgePlatform{GOOS: goos, GOARCH: goarch}, nil
}

func ensureLocalBridgeBinary(ctx context.Context, platform bridgePlatform) (string, error) {
	if strings.TrimSpace(platform.GOOS) == "" || strings.TrimSpace(platform.GOARCH) == "" {
		return "", fmt.Errorf("bridge platform is required")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve bridge cache dir: %w", err)
	}
	targetDir := filepath.Join(cacheDir, "agen8", "bridge", platform.GOOS+"-"+platform.GOARCH)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create bridge cache dir: %w", err)
	}
	binaryPath := filepath.Join(targetDir, "agen8-bridge")
	repoRoot, err := repoRootFromSource()
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(binaryPath); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
		stale, err := bridgeBinaryIsStale(repoRoot, info.ModTime())
		if err != nil {
			return "", err
		}
		if !stale {
			return binaryPath, nil
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat cached bridge binary: %w", err)
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./cmd/agen8-bridge")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS="+platform.GOOS, "GOARCH="+platform.GOARCH, "GOMAXPROCS=2")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build bridge binary for %s/%s: %w: %s", platform.GOOS, platform.GOARCH, err, strings.TrimSpace(string(output)))
	}
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod cached bridge binary: %w", err)
	}
	return binaryPath, nil
}

func bridgeBinaryIsStale(repoRoot string, builtAt time.Time) (bool, error) {
	paths := []string{
		filepath.Join(repoRoot, "go.mod"),
		filepath.Join(repoRoot, "go.sum"),
		filepath.Join(repoRoot, "cmd", "agen8-bridge"),
		filepath.Join(repoRoot, "internal", "bridge"),
	}
	for _, path := range paths {
		stale, err := pathModifiedAfter(path, builtAt)
		if err != nil {
			return false, err
		}
		if stale {
			return true, nil
		}
	}
	return false, nil
}

func pathModifiedAfter(path string, cutoff time.Time) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat bridge source %s: %w", path, err)
	}
	if !info.IsDir() {
		return info.ModTime().After(cutoff), nil
	}
	stale := false
	err = filepath.WalkDir(path, func(child string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(child) != ".go" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			stale = true
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("walk bridge source %s: %w", path, err)
	}
	return stale, nil
}

func repoRootFromSource() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve source path for bridge build")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("resolve repository root for bridge build: %w", err)
	}
	return root, nil
}

func uploadSSHFile(client *ssh.Client, localPath, remotePath string, mode os.FileMode) error {
	local, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local bridge binary: %w", err)
	}
	defer local.Close()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("open ssh bridge uploader: %w", err)
	}
	defer sftpClient.Close()
	remoteDir := filepath.ToSlash(filepath.Dir(remotePath))
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("create remote bridge dir %s: %w", remoteDir, err)
	}
	if info, err := sftpClient.Stat(remotePath); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat remote bridge binary %s: %w", remotePath, err)
	}
	remoteTemp := remotePath + ".tmp"
	_ = sftpClient.Remove(remoteTemp)
	remote, err := sftpClient.OpenFile(remoteTemp, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return fmt.Errorf("open remote bridge binary temp %s: %w", remoteTemp, err)
	}
	if _, err := io.Copy(remote, local); err != nil {
		_ = remote.Close()
		_ = sftpClient.Remove(remoteTemp)
		return fmt.Errorf("upload remote bridge binary temp %s: %w", remoteTemp, err)
	}
	if err := remote.Close(); err != nil {
		_ = sftpClient.Remove(remoteTemp)
		return fmt.Errorf("close remote bridge binary temp %s: %w", remoteTemp, err)
	}
	if err := sftpClient.Chmod(remoteTemp, mode); err != nil {
		_ = sftpClient.Remove(remoteTemp)
		return fmt.Errorf("chmod remote bridge binary temp %s: %w", remoteTemp, err)
	}
	if err := sftpClient.Rename(remoteTemp, remotePath); err != nil {
		_ = sftpClient.Remove(remoteTemp)
		return fmt.Errorf("replace remote bridge binary %s: %w", remotePath, err)
	}
	return nil
}

func fileSHA256Prefix(path string, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("hash prefix length is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file for hash %s: %w", path, err)
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", fmt.Errorf("hash file %s: %w", path, err)
	}
	encoded := hex.EncodeToString(sum.Sum(nil))
	if length > len(encoded) {
		length = len(encoded)
	}
	return encoded[:length], nil
}

func reserveLocalTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve local tcp port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForHTTPReady(ctx context.Context, url string) error {
	client := http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", url)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build readiness request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

type sshTCPForward struct {
	ctx        context.Context
	client     *ssh.Client
	listener   net.Listener
	remoteAddr string
	done       chan struct{}
}

func (t Transport) reverseSSHTCP(ctx context.Context, location locationdomain.Location, localAddr string) (locationapp.TCPForward, error) {
	localAddr = strings.TrimSpace(localAddr)
	if localAddr == "" {
		return nil, fmt.Errorf("local daemon address is required")
	}
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return nil, err
	}
	listener, err := client.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("listen remote tcp forward: %w", err)
	}
	forward := &sshRemoteTCPForward{
		ctx:       ctx,
		client:    client,
		listener:  listener,
		localAddr: localAddr,
		done:      make(chan struct{}),
	}
	go forward.acceptLoop()
	return forward, nil
}

type sshRemoteTCPForward struct {
	ctx       context.Context
	client    *ssh.Client
	listener  net.Listener
	localAddr string
	done      chan struct{}
}

func (f *sshRemoteTCPForward) LocalAddr() string {
	if f == nil || f.listener == nil {
		return ""
	}
	return f.listener.Addr().String()
}

func (f *sshRemoteTCPForward) Close() error {
	if f == nil {
		return nil
	}
	var err error
	if f.listener != nil {
		err = f.listener.Close()
	}
	if f.client != nil {
		if closeErr := f.client.Close(); err == nil {
			err = closeErr
		}
	}
	if f.done != nil {
		select {
		case <-f.done:
		case <-time.After(2 * time.Second):
		}
	}
	return err
}

func (f *sshRemoteTCPForward) acceptLoop() {
	defer close(f.done)
	for {
		remote, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.proxy(remote)
	}
}

func (f *sshRemoteTCPForward) proxy(remote net.Conn) {
	defer remote.Close()
	local, err := net.Dial("tcp", f.localAddr)
	if err != nil {
		return
	}
	defer local.Close()
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(local, remote)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(remote, local)
		errCh <- err
	}()
	select {
	case <-f.ctx.Done():
	case <-errCh:
	}
}

func (f *sshTCPForward) LocalAddr() string {
	if f == nil || f.listener == nil {
		return ""
	}
	return f.listener.Addr().String()
}

func (f *sshTCPForward) Close() error {
	if f == nil {
		return nil
	}
	var err error
	if f.listener != nil {
		err = f.listener.Close()
	}
	if f.client != nil {
		if closeErr := f.client.Close(); err == nil {
			err = closeErr
		}
	}
	if f.done != nil {
		select {
		case <-f.done:
		case <-time.After(2 * time.Second):
		}
	}
	return err
}

func (f *sshTCPForward) acceptLoop() {
	defer close(f.done)
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.proxy(conn)
	}
}

func (f *sshTCPForward) proxy(local net.Conn) {
	defer local.Close()
	remote, err := f.client.Dial("tcp", f.remoteAddr)
	if err != nil {
		return
	}
	defer remote.Close()
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(remote, local)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(local, remote)
		errCh <- err
	}()
	select {
	case <-f.ctx.Done():
	case <-errCh:
	}
}

func cleanPath(path string) string {
	return strings.TrimSpace(path)
}

func remoteJoin(base, name string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "/" + name
	}
	if base == "/" {
		return "/" + name
	}
	return base + "/" + name
}

func (t Transport) openSFTPPath(ctx context.Context, location locationdomain.Location, path string) (*sftp.Client, func(), string, error) {
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return nil, nil, "", err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, nil, "", fmt.Errorf("open ssh file repository: %w", err)
	}
	cleanPath, err := resolveSSHPath(ctx, client, path)
	if err != nil {
		sftpClient.Close()
		client.Close()
		return nil, nil, "", err
	}
	closeFn := func() {
		sftpClient.Close()
		client.Close()
	}
	return sftpClient, closeFn, cleanPath, nil
}

func resolveSSHPath(ctx context.Context, client *ssh.Client, path string) (string, error) {
	cleanPath := cleanPath(path)
	if cleanPath == "" {
		return "", fmt.Errorf("path is required")
	}
	if cleanPath == "~" {
		home, err := runSSHCommand(ctx, client, "printf %s \"$HOME\"")
		if err != nil {
			return "", fmt.Errorf("resolve remote home dir: %w", err)
		}
		cleanPath = strings.TrimSpace(home)
	}
	return pathpkg.Clean(cleanPath), nil
}

func fileInfoFromOS(info os.FileInfo) filedomain.Info {
	return filedomain.Info{
		IsDir:      info.IsDir(),
		Size:       info.Size(),
		ModifiedAt: info.ModTime().UTC(),
	}
}

func sortFileEntries(out []filedomain.Entry) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Info.IsDir != out[j].Info.IsDir {
			return out[i].Info.IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
}

func sortEntries(out []locationapp.DirEntry) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == locationdomain.DirEntryDirectory
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
}

func dirEntryType(info os.FileInfo) locationdomain.DirEntryType {
	if info.Mode()&os.ModeSymlink != 0 {
		return locationdomain.DirEntrySymlink
	}
	if info.IsDir() {
		return locationdomain.DirEntryDirectory
	}
	return locationdomain.DirEntryFile
}

func classifySSHFailure(err error) locationdomain.FailureCode {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unable to authenticate") || strings.Contains(message, "permission denied") || strings.Contains(message, "ssh_auth_sock"):
		return locationdomain.FailureCodeAuthFailed
	case strings.Contains(message, "known_hosts"):
		return locationdomain.FailureCodeAuthFailed
	default:
		return locationdomain.FailureCodeUnreachable
	}
}
