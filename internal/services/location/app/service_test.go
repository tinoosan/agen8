package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
)

func TestServiceEnsureLocalCreatesAndProbesDefaultLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	transport := &transportSpy{
		probe: ProbeResult{
			Reachable:    true,
			FileBrowsing: true,
			Exec:         true,
			Codex:        true,
			Claude:       true,
			ProbedAt:     fixedLocationTime.Add(time.Minute),
		},
	}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	location, err := svc.EnsureLocal(ctx)
	if err != nil {
		t.Fatalf("EnsureLocal: %v", err)
	}
	if location.ID() != "local" || !location.Ready() || location.Status() != locationdomain.StatusOnline {
		t.Fatalf("local location = %+v", location.Record())
	}
	if location.Address().Host != "test-host" {
		t.Fatalf("local host = %q", location.Address().Host)
	}
	if len(repo.saved) != 2 {
		t.Fatalf("saved records = %+v", repo.saved)
	}
}

func TestServiceEnsureLocalBackfillsExistingHostname(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["local"] = locationdomain.Record{
		ID:        "local",
		Kind:      locationdomain.KindLocal,
		Label:     "This machine",
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	svc := newServiceForTest(t, repo, &transportSpy{}, projectCheckerSpy{})

	location, err := svc.EnsureLocal(ctx)
	if err != nil {
		t.Fatalf("EnsureLocal: %v", err)
	}
	if location.Address().Host != "test-host" {
		t.Fatalf("local host = %q", location.Address().Host)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("saved records = %+v", repo.saved)
	}
}

func TestServiceListDirRequiresReadyLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["local"] = locationdomain.Record{
		ID:        "local",
		Kind:      locationdomain.KindLocal,
		Label:     "This machine",
		Status:    locationdomain.StatusNotReady,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	svc := newServiceForTest(t, repo, &transportSpy{}, projectCheckerSpy{})

	if _, err := svc.ListDir(ctx, "local", "/tmp"); err == nil {
		t.Fatalf("expected not ready error")
	}
}

func TestServiceCreateSSHStoresCredentialRef(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	transport := &transportSpy{
		probe: ProbeResult{
			Reachable:    true,
			FileBrowsing: true,
			Exec:         true,
			Codex:        true,
			Claude:       true,
			ProbedAt:     fixedLocationTime.Add(time.Minute),
		},
	}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	location, err := svc.CreateLocation(ctx, CreateLocationInput{
		Kind:          locationdomain.KindSSH,
		Label:         "Remote",
		Address:       locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		CredentialRef: "cred_ssh",
	})
	if err != nil {
		t.Fatalf("CreateLocation: %v", err)
	}
	if got := location.Record().CredentialRef; got != "cred_ssh" {
		t.Fatalf("credential ref = %q", got)
	}
}

func TestServiceInstallCodexRunsTransportAndReprobes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusNotReady,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &transportSpy{
		probe: ProbeResult{
			Reachable:    true,
			FileBrowsing: true,
			Exec:         true,
			Codex:        true,
			Claude:       true,
			ProbedAt:     fixedLocationTime.Add(time.Minute),
		},
	}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	location, err := svc.InstallCodex(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("InstallCodex: %v", err)
	}
	if !transport.installedCodex {
		t.Fatalf("InstallCodex did not call transport")
	}
	if !location.Ready() {
		t.Fatalf("location should be ready after install and reprobe: %+v", location.Record())
	}
}

func TestServiceInstallClaudeRunsTransportAndReprobes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusNotReady,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &transportSpy{
		probe: ProbeResult{
			Reachable:    true,
			FileBrowsing: true,
			Exec:         true,
			Codex:        true,
			Claude:       true,
			ProbedAt:     fixedLocationTime.Add(time.Minute),
		},
	}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	location, err := svc.InstallClaude(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("InstallClaude: %v", err)
	}
	if !transport.installedClaude {
		t.Fatalf("InstallClaude did not call transport")
	}
	if !location.Ready() {
		t.Fatalf("location should be ready after install and reprobe: %+v", location.Record())
	}
}

func TestServiceCodexAndClaudeAuthUseTransport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &transportSpy{
		codexAuthStatus:  CodexAuthStatusResult{LoggedIn: true, Method: "account"},
		codexLogin:       CodexLoginResult{LoginURL: "https://auth.openai.com/device", PID: "456"},
		claudeAuthStatus: ClaudeAuthStatusResult{LoggedIn: true, AuthMethod: "oauth", Provider: "firstParty"},
		claudeLogin:      ClaudeLoginResult{LoginURL: "https://claude.com/login", PID: "123"},
		claudeComplete:   ClaudeLoginResult{Output: "Login successful.", PID: "123"},
	}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	codexStatus, err := svc.CodexAuthStatus(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("CodexAuthStatus: %v", err)
	}
	if !codexStatus.LoggedIn || codexStatus.Method != "account" {
		t.Fatalf("codex status=%+v", codexStatus)
	}
	codexLogin, err := svc.BeginCodexLogin(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("BeginCodexLogin: %v", err)
	}
	if codexLogin.LoginURL != "https://auth.openai.com/device" || codexLogin.PID != "456" {
		t.Fatalf("codex login=%+v", codexLogin)
	}
	status, err := svc.ClaudeAuthStatus(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("ClaudeAuthStatus: %v", err)
	}
	if !status.LoggedIn || status.AuthMethod != "oauth" {
		t.Fatalf("status=%+v", status)
	}
	login, err := svc.BeginClaudeLogin(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("BeginClaudeLogin: %v", err)
	}
	if login.LoginURL != "https://claude.com/login" || login.PID != "123" {
		t.Fatalf("login=%+v", login)
	}
	complete, err := svc.CompleteClaudeLogin(ctx, "loc_ssh", " code-123 ")
	if err != nil {
		t.Fatalf("CompleteClaudeLogin: %v", err)
	}
	if complete.Output != "Login successful." || transport.claudeCompleteCode != "code-123" {
		t.Fatalf("complete=%+v code=%q", complete, transport.claudeCompleteCode)
	}
	if transport.codexAuthLocation.ID() != "loc_ssh" || transport.codexLoginLocation.ID() != "loc_ssh" || transport.claudeAuthLocation.ID() != "loc_ssh" || transport.claudeLoginLocation.ID() != "loc_ssh" || transport.claudeCompleteLocation.ID() != "loc_ssh" {
		t.Fatalf("locations not captured")
	}
}

func TestServiceStartCommandRequiresReadyLocationAndPassesSpec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &transportSpy{process: fakeCommandProcess{}}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	proc, err := svc.StartCommand(ctx, "loc_ssh", CommandSpec{
		Command: "claude",
		Args:    []string{"--print"},
		Workdir: "/srv/playground",
		Env:     []string{"AGEN8=1"},
	})
	if err != nil {
		t.Fatalf("StartCommand: %v", err)
	}
	if proc == nil {
		t.Fatalf("process is nil")
	}
	if transport.commandLocation.ID() != "loc_ssh" {
		t.Fatalf("location = %q", transport.commandLocation.ID())
	}
	if transport.commandSpec.Command != "claude" || strings.Join(transport.commandSpec.Args, " ") != "--print" {
		t.Fatalf("command spec = %+v", transport.commandSpec)
	}
	if transport.commandSpec.Workdir != "/srv/playground" {
		t.Fatalf("workdir = %q", transport.commandSpec.Workdir)
	}
	if strings.Join(transport.commandSpec.Env, " ") != "AGEN8=1" {
		t.Fatalf("env = %+v", transport.commandSpec.Env)
	}
}

func TestServiceStartCommandRejectsNotReadyLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusNotReady,
		Ready:     false,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &transportSpy{}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	_, err := svc.StartCommand(ctx, "loc_ssh", CommandSpec{Command: "claude", Workdir: "/srv/playground"})
	if err == nil {
		t.Fatalf("expected not ready error")
	}
	if transport.commandSpec.Command != "" {
		t.Fatalf("transport should not be called: %+v", transport.commandSpec)
	}
}

func TestServiceEnsureBridgeRequiresReadyLocationAndPassesSpec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &transportSpy{host: Bridge{BaseURL: "http://127.0.0.1:49152", WebSocketURL: "ws://127.0.0.1:49152"}}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	host, err := svc.EnsureBridge(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("EnsureBridge: %v", err)
	}
	if host.WebSocketURL != "ws://127.0.0.1:49152" {
		t.Fatalf("websocket url = %q", host.WebSocketURL)
	}
	if transport.hostLocation.ID() != "loc_ssh" {
		t.Fatalf("location = %q", transport.hostLocation.ID())
	}
}

func TestServiceEnsureBridgeSerializesPerLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &serialBridgeTransport{host: Bridge{BaseURL: "http://127.0.0.1:49152", WebSocketURL: "ws://127.0.0.1:49152"}}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.EnsureBridge(ctx, "loc_ssh"); err != nil {
				t.Errorf("EnsureBridge: %v", err)
			}
		}()
	}
	wg.Wait()

	if transport.maxConcurrent != 1 {
		t.Fatalf("max concurrent bridge ensures=%d want 1", transport.maxConcurrent)
	}
	if transport.calls != 2 {
		t.Fatalf("bridge ensure calls=%d want 2", transport.calls)
	}
}

func TestServiceEnsureBridgeRejectsNotReadyLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["loc_ssh"] = locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Label:     "Remote",
		Address:   locationdomain.Address{Host: "example.internal", Port: 22, Username: "dev"},
		Status:    locationdomain.StatusNotReady,
		Ready:     false,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	transport := &transportSpy{}
	svc := newServiceForTest(t, repo, transport, projectCheckerSpy{})

	_, err := svc.EnsureBridge(ctx, "loc_ssh")
	if err == nil {
		t.Fatalf("expected not ready error")
	}
	if transport.hostLocation.ID() != "" {
		t.Fatalf("transport should not be called: %q", transport.hostLocation.ID())
	}
}

func TestServiceDeleteLocationRefusesActiveProjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newLocationRepoSpy()
	repo.records["local"] = locationdomain.Record{
		ID:        "local",
		Kind:      locationdomain.KindLocal,
		Label:     "This machine",
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedLocationTime,
		UpdatedAt: fixedLocationTime,
	}
	svc := newServiceForTest(t, repo, &transportSpy{}, projectCheckerSpy{hasProjects: true})

	if err := svc.DeleteLocation(ctx, "local"); err == nil {
		t.Fatalf("expected active project error")
	}
	if repo.deleted != "" {
		t.Fatalf("deleted location = %q", repo.deleted)
	}
}

func newServiceForTest(t *testing.T, repo locationdomain.Repository, transport Transport, projects ProjectReferenceChecker) *Service {
	t.Helper()
	svc, err := NewService(Config{
		Locations: repo,
		Transport: transport,
		Projects:  projects,
		Clock:     fixedClock{},
		Hostname:  func() (string, error) { return "test-host", nil },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

var fixedLocationTime = time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

type fixedClock struct{}

func (fixedClock) Now() time.Time {
	return fixedLocationTime
}

type locationRepoSpy struct {
	records map[locationdomain.ID]locationdomain.Record
	saved   []locationdomain.Record
	deleted locationdomain.ID
}

func newLocationRepoSpy() *locationRepoSpy {
	return &locationRepoSpy{records: map[locationdomain.ID]locationdomain.Record{}}
}

func (r *locationRepoSpy) Get(_ context.Context, id locationdomain.ID) (locationdomain.Record, error) {
	record, ok := r.records[id]
	if !ok {
		return locationdomain.Record{}, locationdomain.ErrNotFound
	}
	return record, nil
}

func (r *locationRepoSpy) List(_ context.Context, filter locationdomain.Filter) ([]locationdomain.Record, error) {
	var out []locationdomain.Record
	for _, record := range r.records {
		if filter.Kind != "" && record.Kind != filter.Kind {
			continue
		}
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		if filter.Ready != nil && record.Ready != *filter.Ready {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (r *locationRepoSpy) Save(_ context.Context, record locationdomain.Record) (locationdomain.Record, error) {
	if record.ID == "" {
		return locationdomain.Record{}, errors.New("id is required")
	}
	r.saved = append(r.saved, record)
	r.records[record.ID] = record
	return record, nil
}

func (r *locationRepoSpy) Delete(_ context.Context, id locationdomain.ID) error {
	if _, ok := r.records[id]; !ok {
		return locationdomain.ErrNotFound
	}
	r.deleted = id
	delete(r.records, id)
	return nil
}

type transportSpy struct {
	probe                  ProbeResult
	entries                []DirEntry
	installedCodex         bool
	installedClaude        bool
	codexAuthStatus        CodexAuthStatusResult
	codexLogin             CodexLoginResult
	codexAuthLocation      locationdomain.Location
	codexLoginLocation     locationdomain.Location
	claudeAuthStatus       ClaudeAuthStatusResult
	claudeLogin            ClaudeLoginResult
	claudeComplete         ClaudeLoginResult
	claudeAuthLocation     locationdomain.Location
	claudeLoginLocation    locationdomain.Location
	claudeCompleteLocation locationdomain.Location
	claudeCompleteCode     string
	commandLocation        locationdomain.Location
	commandSpec            CommandSpec
	process                CommandProcess
	hostLocation           locationdomain.Location
	host                   Bridge
}

type serialBridgeTransport struct {
	transportSpy
	host          Bridge
	mu            sync.Mutex
	calls         int
	active        int
	maxConcurrent int
}

func (t *serialBridgeTransport) EnsureBridge(_ context.Context, location locationdomain.Location) (Bridge, error) {
	t.mu.Lock()
	t.calls++
	t.active++
	if t.active > t.maxConcurrent {
		t.maxConcurrent = t.active
	}
	t.mu.Unlock()

	time.Sleep(25 * time.Millisecond)

	t.mu.Lock()
	t.active--
	t.hostLocation = location
	t.mu.Unlock()
	return t.host, nil
}

func (t *transportSpy) Probe(context.Context, locationdomain.Location) (ProbeResult, error) {
	return t.probe, nil
}

func (t *transportSpy) ListDir(context.Context, locationdomain.Location, string) ([]DirEntry, error) {
	return t.entries, nil
}

func (t *transportSpy) InstallCodex(context.Context, locationdomain.Location) (InstallResult, error) {
	t.installedCodex = true
	return InstallResult{Output: "codex installed"}, nil
}

func (t *transportSpy) CodexAuthStatus(_ context.Context, location locationdomain.Location) (CodexAuthStatusResult, error) {
	t.codexAuthLocation = location
	return t.codexAuthStatus, nil
}

func (t *transportSpy) BeginCodexLogin(_ context.Context, location locationdomain.Location) (CodexLoginResult, error) {
	t.codexLoginLocation = location
	return t.codexLogin, nil
}

func (t *transportSpy) InstallClaude(context.Context, locationdomain.Location) (InstallResult, error) {
	t.installedClaude = true
	return InstallResult{Output: "claude installed"}, nil
}

func (t *transportSpy) ClaudeAuthStatus(_ context.Context, location locationdomain.Location) (ClaudeAuthStatusResult, error) {
	t.claudeAuthLocation = location
	return t.claudeAuthStatus, nil
}

func (t *transportSpy) BeginClaudeLogin(_ context.Context, location locationdomain.Location) (ClaudeLoginResult, error) {
	t.claudeLoginLocation = location
	return t.claudeLogin, nil
}

func (t *transportSpy) CompleteClaudeLogin(_ context.Context, location locationdomain.Location, code string) (ClaudeLoginResult, error) {
	t.claudeCompleteLocation = location
	t.claudeCompleteCode = code
	return t.claudeComplete, nil
}

func (t *transportSpy) StartCommand(_ context.Context, location locationdomain.Location, spec CommandSpec) (CommandProcess, error) {
	t.commandLocation = location
	t.commandSpec = spec
	return t.process, nil
}

func (t *transportSpy) EnsureBridge(_ context.Context, location locationdomain.Location) (Bridge, error) {
	t.hostLocation = location
	return t.host, nil
}

type fakeCommandProcess struct{}

func (fakeCommandProcess) StdinPipe() (io.WriteCloser, error) { return nil, nil }
func (fakeCommandProcess) StdoutPipe() (io.ReadCloser, error) { return nil, nil }
func (fakeCommandProcess) StderrText() string                 { return "" }
func (fakeCommandProcess) Start() error                       { return nil }
func (fakeCommandProcess) Wait() error                        { return nil }
func (fakeCommandProcess) Kill() error                        { return nil }

type projectCheckerSpy struct {
	hasProjects bool
}

func (p projectCheckerSpy) HasProjectsForLocation(context.Context, locationdomain.ID) (bool, error) {
	return p.hasProjects, nil
}
