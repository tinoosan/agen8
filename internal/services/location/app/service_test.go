package app

import (
	"context"
	"errors"
	"testing"
	"time"

	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
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
	probe   ProbeResult
	entries []DirEntry
}

func (t *transportSpy) Probe(context.Context, locationdomain.Location) (ProbeResult, error) {
	return t.probe, nil
}

func (t *transportSpy) ListDir(context.Context, locationdomain.Location, string) ([]DirEntry, error) {
	return t.entries, nil
}

type projectCheckerSpy struct {
	hasProjects bool
}

func (p projectCheckerSpy) HasProjectsForLocation(context.Context, locationdomain.ID) (bool, error) {
	return p.hasProjects, nil
}
