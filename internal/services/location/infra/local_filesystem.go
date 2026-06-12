package infra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	credentialapp "github.com/tinoosan/agen8/internal/services/credential/app"
	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
	filedomain "github.com/tinoosan/agen8/internal/services/file/domain/file"
	locationapp "github.com/tinoosan/agen8/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
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
		return locationapp.ProbeResult{
			Reachable:    true,
			FileBrowsing: true,
			Status:       locationdomain.ProbeStatusPassed,
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
		path, err := validateLocalPath(path)
		if err != nil {
			return filedomain.Info{}, err
		}
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
		path, err := validateLocalPath(path)
		if err != nil {
			return nil, err
		}
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
		path, err := validateLocalPath(path)
		if err != nil {
			return filedomain.Content{}, err
		}
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
		path, err := validateLocalPath(path)
		if err != nil {
			return err
		}
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
		path, err := validateLocalPath(path)
		if err != nil {
			return err
		}
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
		source, err := validateLocalPath(source)
		if err != nil {
			return err
		}
		destination, err := validateLocalPath(destination)
		if err != nil {
			return err
		}
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
		source, err := validateLocalPath(source)
		if err != nil {
			return err
		}
		destination, err := validateLocalPath(destination)
		if err != nil {
			return err
		}
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
		path, err := validateLocalPath(path)
		if err != nil {
			return err
		}
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
		path, err := validateLocalPath(path)
		if err != nil {
			return err
		}
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
	result.Status = locationdomain.ProbeStatusPassed

	// Detect whether git is available on the host so the UI can show the
	// git-diff capability as reachable. Detection only — it never enables the
	// capability; that stays a separate, human-granted opt-in. A fixed,
	// argument-free command, so there's nothing to escape.
	if _, err := runSSHCommandOutput(ctx, client, "command -v git"); err == nil {
		result.Exec = true
	}
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

func listLocalDir(path string) ([]locationapp.DirEntry, error) {
	path, err := validateLocalPath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("list directory %s: %w", path, err)
	}
	out := make([]locationapp.DirEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat directory entry %s: %w", entry.Name(), err)
		}
		out = append(out, locationapp.DirEntry{
			Name: entry.Name(),
			Path: filepath.Join(path, entry.Name()),
			Type: dirEntryType(info),
			Size: info.Size(),
		})
	}
	sortEntries(out)
	return out, nil
}

func listLocalFiles(path string) ([]filedomain.Entry, error) {
	path, err := validateLocalPath(path)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(path)
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
	path, err := validateLocalPath(path)
	if err != nil {
		return filedomain.Content{}, err
	}
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
	src, err := validateLocalPath(src)
	if err != nil {
		return err
	}
	dst, err = validateLocalPath(dst)
	if err != nil {
		return err
	}
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
			if entryInfo.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("copy source contains symlink component %s", path)
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

// gitBaselineMaxBytes caps how much committed content a remote baseline will
// return, matching the local preview cap so a huge file can't exhaust memory.
const gitBaselineMaxBytes = 16 * 1024 * 1024

// gitBaselineTimeout bounds a single remote git invocation. A wedged remote
// git (e.g. a giant repo or a hung filesystem) must not pin a daemon goroutine.
const gitBaselineTimeout = 20 * time.Second

// shellSingleQuote wraps an arbitrary string so a POSIX shell treats it as one
// literal argument, neutralizing every metacharacter. This is THE injection
// control for remote command execution: SSH exec passes a single string to the
// remote login shell (there is no argv-array path like local exec.Command), so
// any value interpolated into that string MUST go through here. The standard
// trick: wrap in single quotes and replace each embedded single quote with the
// four-character sequence '\” (close-quote, escaped-quote, reopen-quote).
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// GitShowBaseline returns the committed (git HEAD) content of a file living on
// this location, by running a FIXED, read-only git command where the repo
// lives. dir and name are the file's directory and bare base name; both are
// shell-quoted before interpolation. There is deliberately no generic remote
// exec on this transport — only this one capability-scoped, read-only command —
// so the blast radius of remote execution is bounded to `git show`.
//
// A non-zero git exit (untracked file, new file, or not a repo) is a normal
// answer: GitBaseline.Tracked=false, no error. Errors are reserved for the
// connection/transport failing.
func (t Transport) GitShowBaseline(ctx context.Context, location locationdomain.Location, dir, name string) (filedomain.GitBaseline, error) {
	dir = strings.TrimSpace(dir)
	name = strings.TrimSpace(name)
	if dir == "" || name == "" {
		return filedomain.GitBaseline{}, fmt.Errorf("git baseline requires a directory and file name")
	}
	if strings.ContainsAny(name, "/\\") || name == "." || name == ".." || strings.Contains(name, "..") {
		// Defense in depth: callers already pass a validated bare base name,
		// but never let a path-shaped name reach the remote command.
		return filedomain.GitBaseline{}, fmt.Errorf("git baseline file name must be a bare name")
	}
	client, err := t.dialSSH(ctx, location)
	if err != nil {
		return filedomain.GitBaseline{}, err
	}
	defer client.Close()

	runCtx, cancel := context.WithTimeout(ctx, gitBaselineTimeout)
	defer cancel()

	// `git -C <dir> show HEAD:./<name>` resolves the path relative to the
	// file's own directory, so it works whether the git root is the project
	// root or an ancestor. Both interpolated values are single-quoted.
	command := "git -C " + shellSingleQuote(dir) + " show " + shellSingleQuote("HEAD:./"+name)
	stdout, runErr := runSSHCommandStdout(runCtx, client, command, gitBaselineMaxBytes)
	if runErr != nil {
		if runCtx.Err() != nil {
			return filedomain.GitBaseline{}, fmt.Errorf("remote git baseline timed out: %w", runCtx.Err())
		}
		var exitErr *ssh.ExitError
		if errors.As(runErr, &exitErr) {
			// Non-zero exit = untracked/new/not-a-repo: a normal "no baseline".
			return filedomain.GitBaseline{Tracked: false}, nil
		}
		// Anything else (dial already succeeded, so this is a session/IO fault).
		return filedomain.GitBaseline{}, fmt.Errorf("remote git baseline failed: %w", runErr)
	}
	return filedomain.GitBaseline{Tracked: true, Bytes: stdout}, nil
}

// runSSHCommandStdout runs a command capturing ONLY stdout (stderr is dropped
// so remote diagnostics never leak into file content or the UI), bounded by ctx
// and capped at maxBytes. Returns *ssh.ExitError when the remote command exits
// non-zero, which callers use to distinguish "command ran and said no" from
// "the connection broke".
func runSSHCommandStdout(ctx context.Context, client *ssh.Client, command string, maxBytes int64) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := session.Start(command); err != nil {
		return nil, err
	}
	type readResult struct {
		data []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(io.LimitReader(stdout, maxBytes))
		// Wait for the command to finish so we observe its exit status.
		waitErr := session.Wait()
		if readErr != nil {
			done <- readResult{data: data, err: readErr}
			return
		}
		done <- readResult{data: data, err: waitErr}
	}()
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		return nil, ctx.Err()
	case res := <-done:
		return res.data, res.err
	}
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

func validateLocalPath(path string) (string, error) {
	// validateLocalPath enforces a conservative file-system contract for local
	// location operations. It rejects null-bytes and traversal-style segments,
	// requires absolute paths, and resolves home shorthand before clean.
	// This keeps all local filesystem operations deterministic and bounded.
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(trimmed, "\x00") {
		return "", fmt.Errorf("invalid path")
	}
	for _, segment := range strings.Split(filepath.ToSlash(trimmed), "/") {
		if segment == ".." {
			return "", fmt.Errorf("path traversal is not allowed")
		}
	}
	if trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		trimmed = home
	}
	clean := filepath.Clean(trimmed)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path must be absolute")
	}
	for _, segment := range strings.Split(filepath.ToSlash(clean), "/") {
		if segment == ".." {
			return "", fmt.Errorf("path traversal is not allowed")
		}
	}
	if err := ensureNoSymlinkPath(clean); err != nil {
		return "", err
	}
	return clean, nil
}

func ensureNoSymlinkPath(path string) error {
	cleanPath := filepath.Clean(path)
	parts := strings.Split(filepath.ToSlash(cleanPath), "/")
	if len(parts) == 0 {
		return nil
	}
	cursor := ""
	absolute := strings.HasPrefix(cleanPath, string(filepath.Separator))
	if absolute {
		cursor = string(filepath.Separator)
	}
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			continue
		}
		if cursor == "" {
			cursor = part
		} else {
			cursor = filepath.Join(cursor, part)
		}
		if absolute && i == 1 {
			if err := validateTopLevelSymlinkPath(cursor); err != nil {
				return err
			}
			continue
		}
		if !absolute && i == 0 {
			continue
		}
		info, err := os.Lstat(cursor)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat path %s: %w", cursor, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains symlink component %s", cursor)
		}
	}
	return nil
}

func validateTopLevelSymlinkPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat path %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	target, err := resolveSymlinkTarget(path)
	if err != nil {
		return fmt.Errorf("resolve symlink target %s: %w", path, err)
	}
	if !isAllowedTopLevelSymlinkTarget(path, target) {
		return fmt.Errorf("path contains disallowed top-level symlink component %s", path)
	}
	return nil
}

func resolveSymlinkTarget(path string) (string, error) {
	value, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(path), value)), nil
}

func isAllowedTopLevelSymlinkTarget(root string, resolvedTarget string) bool {
	allowed, ok := topLevelSymlinkAllowlist[filepath.Clean(filepath.ToSlash(root))]
	if !ok {
		return false
	}
	cleanTarget := filepath.Clean(filepath.ToSlash(resolvedTarget))
	for _, expected := range allowed {
		if cleanTarget == filepath.Clean(filepath.ToSlash(expected)) {
			return true
		}
	}
	return false
}

var topLevelSymlinkAllowlist = map[string][]string{
	"/tmp": {"/private/tmp"},
	"/var": {"/private/var"},
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
