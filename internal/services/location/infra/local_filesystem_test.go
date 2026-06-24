package infra

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	locationapp "github.com/tinoosan/agen8/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
)

func TestValidateLocalPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		errOK bool
	}{
		{name: "missing", input: "", errOK: false},
		{name: "relative", input: "tmp/project", errOK: false},
		{name: "backtrack", input: "/tmp/../etc", errOK: false},
		{name: "null-byte", input: "/tmp/\x00bad", errOK: false},
		{name: "tilde", input: "~", errOK: true},
		{name: "absolute", input: "/tmp", errOK: true},
		{name: "absolute-without-traversal", input: "/", errOK: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateLocalPath(tt.input)
			if tt.errOK && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.errOK && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestValidateLocalPathRejectsSymlinkPaths(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	projectDir := filepath.Join(base, "project")
	escapeTarget := filepath.Join(base, "escape-target")
	safeFile := filepath.Join(projectDir, "safe.txt")
	unsafeTarget := filepath.Join(escapeTarget, "passwd")
	symlinkPath := filepath.Join(base, "link")
	existingNested := filepath.Join(base, "nested")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.MkdirAll(escapeTarget, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.MkdirAll(existingNested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(safeFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write safe file: %v", err)
	}
	if err := os.WriteFile(unsafeTarget, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write escape file: %v", err)
	}
	if err := os.Symlink(escapeTarget, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := validateLocalPath(safeFile); err != nil {
		t.Fatalf("safe file should be allowed: %v", err)
	}
	if _, err := validateLocalPath(filepath.Join(symlinkPath, "passwd")); err == nil {
		t.Fatalf("expected symlinked path to be rejected")
	}

	siblingSymlink := filepath.Join(existingNested, "sibling")
	if err := os.Symlink(projectDir, siblingSymlink); err != nil {
		t.Fatalf("create sibling symlink: %v", err)
	}
	if _, err := validateLocalPath(filepath.Join(siblingSymlink, "safe.txt")); err == nil {
		t.Fatalf("expected symlink directory escape to be rejected")
	}
}

func TestValidateTopLevelSymlinkPolicy(t *testing.T) {
	t.Parallel()

	if _, ok := topLevelSymlinkAllowlist["/tmp"]; !ok {
		t.Fatalf("top-level symlink allowlist missing /tmp")
	}
	if !isAllowedTopLevelSymlinkTarget("/tmp", "/private/tmp") {
		t.Fatalf("/tmp symlink target /private/tmp should be allowed")
	}
	if isAllowedTopLevelSymlinkTarget("/tmp", "/etc") {
		t.Fatalf("/tmp symlink target /etc should be rejected")
	}
	if isAllowedTopLevelSymlinkTarget("/dev", "/private/tmp") {
		t.Fatalf("unlisted top-level symlink roots should be rejected")
	}
}

func TestValidateLocalPathTopLevelSymlinkAllowlistMatchesPlatform(t *testing.T) {
	t.Parallel()

	for root := range topLevelSymlinkAllowlist {
		info, err := os.Lstat(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("lstat %s: %v", root, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := resolveSymlinkTarget(root)
		if err != nil {
			t.Fatalf("resolveSymlinkTarget %s: %v", root, err)
		}
		if !isAllowedTopLevelSymlinkTarget(root, target) {
			t.Fatalf("platform symlinked %s -> %s is not in allowlist", root, target)
		}
	}
}

func TestTransportLocalMethodsRejectUnsafePaths(t *testing.T) {
	t.Parallel()

	transport := NewTransport()
	location := mustTestLocalLocation(t)
	ctx := context.Background()
	bad := filepath.Join("tmp", "..", "etc")
	root := t.TempDir()
	escape := filepath.Join(root, "escape")
	if err := os.MkdirAll(escape, 0o755); err != nil {
		t.Fatalf("mkdir escape: %v", err)
	}
	if err := os.WriteFile(filepath.Join(escape, "secret"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write escape file: %v", err)
	}
	symlinkDir := filepath.Join(root, "symlink")
	if err := os.Symlink(escape, symlinkDir); err != nil {
		t.Fatalf("create symlink dir: %v", err)
	}
	symlinkFile := filepath.Join(root, "symlink-file.txt")
	if err := os.WriteFile(symlinkFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write symlink file: %v", err)
	}
	if err := os.Symlink(symlinkFile, filepath.Join(root, "secret-link.txt")); err != nil {
		t.Fatalf("create symlink file: %v", err)
	}
	badSymlink := filepath.Join(symlinkDir, "secret")
	badSymlinkFile := filepath.Join(root, "secret-link.txt")

	if _, err := transport.ListDir(ctx, location, bad); err == nil {
		t.Fatalf("ListDir expected reject")
	}
	if _, err := transport.ListFiles(ctx, location, bad); err == nil {
		t.Fatalf("ListFiles expected reject")
	}
	if _, err := transport.ReadFile(ctx, location, bad, 16); err == nil {
		t.Fatalf("ReadFile expected reject")
	}
	if _, err := transport.StatFile(ctx, location, bad); err == nil {
		t.Fatalf("StatFile expected reject")
	}
	if err := transport.CreateDir(ctx, location, bad); err == nil {
		t.Fatalf("CreateDir expected reject")
	}
	if err := transport.CreateFile(ctx, location, bad); err == nil {
		t.Fatalf("CreateFile expected reject")
	}
	if err := transport.MoveFile(ctx, location, bad, bad); err == nil {
		t.Fatalf("MoveFile expected reject")
	}
	if err := transport.DeleteFile(ctx, location, bad); err == nil {
		t.Fatalf("DeleteFile expected reject")
	}
	if err := transport.WriteFile(ctx, location, bad, []byte("x")); err == nil {
		t.Fatalf("WriteFile expected reject")
	}
	if err := transport.CopyFile(ctx, location, bad, filepath.Join("tmp", "out")); err == nil {
		t.Fatalf("CopyFile expected reject")
	}

	if _, err := transport.ListDir(ctx, location, badSymlink); err == nil {
		t.Fatalf("ListDir expected symlink reject")
	}
	if _, err := transport.ListFiles(ctx, location, badSymlink); err == nil {
		t.Fatalf("ListFiles expected symlink reject")
	}
	if _, err := transport.ReadFile(ctx, location, badSymlink, 16); err == nil {
		t.Fatalf("ReadFile expected symlink reject")
	}
	if _, err := transport.StatFile(ctx, location, badSymlink); err == nil {
		t.Fatalf("StatFile expected symlink reject")
	}
	if err := transport.CreateDir(ctx, location, filepath.Join(symlinkDir, "new-dir")); err == nil {
		t.Fatalf("CreateDir expected symlink reject")
	}
	if err := transport.CreateFile(ctx, location, badSymlinkFile); err == nil {
		t.Fatalf("CreateFile expected symlink file reject")
	}
	if err := transport.MoveFile(ctx, location, symlinkFile, filepath.Join(symlinkDir, "moved")); err == nil {
		t.Fatalf("MoveFile expected symlink destination reject")
	}
	if err := transport.DeleteFile(ctx, location, badSymlink); err == nil {
		t.Fatalf("DeleteFile expected symlink reject")
	}
	if err := transport.WriteFile(ctx, location, badSymlinkFile, []byte("x")); err == nil {
		t.Fatalf("WriteFile expected symlink reject")
	}
	if err := transport.CopyFile(ctx, location, symlinkFile, filepath.Join(symlinkDir, "copied")); err == nil {
		t.Fatalf("CopyFile expected symlink destination reject")
	}
}

func TestCopyLocalPathRejectsNestedSymlinkEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	escape := filepath.Join(root, "escape")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.MkdirAll(escape, 0o755); err != nil {
		t.Fatalf("mkdir escape: %v", err)
	}
	if err := os.WriteFile(filepath.Join(escape, "secret"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write escape: %v", err)
	}
	if err := os.Symlink(filepath.Join(escape, "secret"), filepath.Join(src, "secret-link")); err != nil {
		t.Fatalf("create nested symlink: %v", err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat src: %v", err)
	}
	if err := copyLocalPath(src, dst, info); err == nil {
		t.Fatalf("copyLocalPath expected nested symlink reject")
	}
}

func TestTransportLocalCopyAndReadFiles(t *testing.T) {
	t.Parallel()

	transport := NewTransport()
	location := mustTestLocalLocation(t)
	ctx := context.Background()
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "nested", "dest.txt")

	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := transport.CopyFile(ctx, location, src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	read, err := transport.ReadFile(ctx, location, dst, 16)
	if err != nil {
		t.Fatalf("ReadFile copied file: %v", err)
	}
	if string(read.Bytes) != "hello" {
		t.Fatalf("copied content mismatch: %q", string(read.Bytes))
	}
}

func TestReadWithCloseReturnsCloseError(t *testing.T) {
	t.Parallel()

	reader := &fakeReadCloser{
		reader:   bytes.NewReader([]byte("payload")),
		closeErr: errors.New("forced read close failure"),
	}
	if _, err := readWithClose(reader, 16); !errors.Is(err, reader.closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}
}

func TestCopyWithCloseReturnsOutputCloseError(t *testing.T) {
	t.Parallel()

	in := &fakeReadCloser{
		reader: bytes.NewReader([]byte("payload")),
	}
	out := &fakeWriteCloser{
		closeErr: errors.New("forced write close failure"),
	}
	if err := copyWithClose(in, out); !errors.Is(err, out.closeErr) {
		t.Fatalf("expected output close error, got %v", err)
	}
}

func TestTransportSSHAuthMethodsResolvePasswordCredential(t *testing.T) {
	t.Parallel()

	resolver := &recordingCredentialResolver{
		result: locationapp.ResolvedCredential{
			ID:      "cred_password",
			Kind:    locationapp.CredentialKindSSHPassword,
			Purpose: locationapp.CredentialPurposeLocationSSH,
			Values:  map[string]string{"password": "secret"},
		},
	}
	transport := NewTransport(TransportConfig{Credentials: resolver})
	location := mustTestSSHLocation(t, "cred_password")

	methods, err := transport.sshAuthMethods(context.Background(), location)
	if err != nil {
		t.Fatalf("sshAuthMethods: %v", err)
	}
	if len(methods) != 2 {
		t.Fatalf("auth methods length = %d, want 2", len(methods))
	}
	if resolver.input.CredentialID != "cred_password" {
		t.Fatalf("credential id = %q, want cred_password", resolver.input.CredentialID)
	}
	if resolver.input.Purpose != locationapp.CredentialPurposeLocationSSH {
		t.Fatalf("credential purpose = %q, want %q", resolver.input.Purpose, locationapp.CredentialPurposeLocationSSH)
	}
}

func TestTransportSSHAuthMethodsRejectMissingResolverForCredentialRef(t *testing.T) {
	t.Parallel()

	transport := NewTransport()
	location := mustTestSSHLocation(t, "cred_password")

	if _, err := transport.sshAuthMethods(context.Background(), location); err == nil {
		t.Fatalf("expected missing credential resolver error")
	}
}

func TestListAndReadHelpersRejectUnsafePaths(t *testing.T) {
	t.Parallel()

	if _, err := listLocalDir("tmp"); err == nil {
		t.Fatal("listLocalDir should reject relative path")
	}
	if _, err := listLocalFiles("tmp/../etc"); err == nil {
		t.Fatal("listLocalFiles should reject traversal")
	}
	if _, err := readLocalFile("tmp/../etc", 16); err == nil {
		t.Fatal("readLocalFile should reject traversal")
	}
}

func mustTestLocalLocation(t *testing.T) locationdomain.Location {
	t.Helper()
	record := locationdomain.NewInput{
		ID:        "local",
		Kind:      locationdomain.KindLocal,
		Label:     "local",
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedTestTime(),
		UpdatedAt: fixedTestTime(),
	}
	location, err := locationdomain.New(record)
	if err != nil {
		t.Fatalf("locationdomain.New: %v", err)
	}
	return location
}

func mustTestSSHLocation(t *testing.T, credentialRef string) locationdomain.Location {
	t.Helper()
	location, err := locationdomain.New(locationdomain.NewInput{
		ID:   "ssh",
		Kind: locationdomain.KindSSH,
		Address: locationdomain.Address{
			Host:     "example.test",
			Port:     22,
			Username: "agent",
		},
		Label:         "ssh",
		Status:        locationdomain.StatusOnline,
		Ready:         true,
		CredentialRef: credentialRef,
		CreatedAt:     fixedTestTime(),
		UpdatedAt:     fixedTestTime(),
	})
	if err != nil {
		t.Fatalf("locationdomain.New ssh: %v", err)
	}
	return location
}

type recordingCredentialResolver struct {
	input  locationapp.ResolveCredentialInput
	result locationapp.ResolvedCredential
	err    error
}

func (r *recordingCredentialResolver) ResolveCredential(_ context.Context, input locationapp.ResolveCredentialInput) (locationapp.ResolvedCredential, error) {
	r.input = input
	if r.err != nil {
		return locationapp.ResolvedCredential{}, r.err
	}
	return r.result, nil
}

func fixedTestTime() time.Time {
	return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
}

type fakeReadCloser struct {
	reader   io.Reader
	closeErr error
}

func (f *fakeReadCloser) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *fakeReadCloser) Close() error {
	return f.closeErr
}

type fakeWriteCloser struct {
	bytes.Buffer
	closeErr error
}

func (f *fakeWriteCloser) Close() error {
	return f.closeErr
}
