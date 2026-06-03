package infra

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	filedomain "github.com/tinoosan/agen8-mcp-server/internal/services/file/domain/file"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
)

func TestLocationRepositoryListDirUsesReferencedLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	locations := locationRepoStub{records: map[locationdomain.ID]locationdomain.Record{
		"ssh-build": testLocationRecord("ssh-build", true),
	}}
	transport := &locationFileTransportSpy{
		entries: []filedomain.Entry{{Name: "README.md", Info: filedomain.Info{Size: 12}}},
	}
	repo, err := NewLocationRepository(locations, transport)
	require.NoError(t, err)

	entries, err := repo.ListDir(ctx, filedomain.Reference{LocationID: "ssh-build", Path: "/srv/app"})
	require.NoError(t, err)

	require.Equal(t, []filedomain.Entry{{Name: "README.md", Info: filedomain.Info{Size: 12}}}, entries)
	require.Equal(t, locationdomain.ID("ssh-build"), transport.location.ID())
	require.Equal(t, "/srv/app", transport.path)
}

func TestLocationRepositoryRejectsNotReadyLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	locations := locationRepoStub{records: map[locationdomain.ID]locationdomain.Record{
		"ssh-build": testLocationRecord("ssh-build", false),
	}}
	transport := &locationFileTransportSpy{}
	repo, err := NewLocationRepository(locations, transport)
	require.NoError(t, err)

	_, err = repo.Stat(ctx, filedomain.Reference{LocationID: "ssh-build", Path: "/srv/app"})
	require.ErrorContains(t, err, `location "ssh-build" is not ready`)
	require.Empty(t, transport.path)
}

type locationRepoStub struct {
	records map[locationdomain.ID]locationdomain.Record
}

func (r locationRepoStub) Get(_ context.Context, id locationdomain.ID) (locationdomain.Record, error) {
	record, ok := r.records[id]
	if !ok {
		return locationdomain.Record{}, locationdomain.ErrNotFound
	}
	return record, nil
}

func (r locationRepoStub) List(context.Context, locationdomain.Filter) ([]locationdomain.Record, error) {
	out := make([]locationdomain.Record, 0, len(r.records))
	for _, record := range r.records {
		out = append(out, record)
	}
	return out, nil
}

func (r locationRepoStub) Save(context.Context, locationdomain.Record) (locationdomain.Record, error) {
	panic("unexpected Save")
}

func (r locationRepoStub) Delete(context.Context, locationdomain.ID) error {
	panic("unexpected Delete")
}

type locationFileTransportSpy struct {
	location locationdomain.Location
	path     string
	entries  []filedomain.Entry
}

func (t *locationFileTransportSpy) StatFile(_ context.Context, location locationdomain.Location, path string) (filedomain.Info, error) {
	t.location = location
	t.path = path
	return filedomain.Info{IsDir: true}, nil
}

func (t *locationFileTransportSpy) ListFiles(_ context.Context, location locationdomain.Location, path string) ([]filedomain.Entry, error) {
	t.location = location
	t.path = path
	return append([]filedomain.Entry(nil), t.entries...), nil
}

func (t *locationFileTransportSpy) ReadFile(context.Context, locationdomain.Location, string, int64) (filedomain.Content, error) {
	return filedomain.Content{}, nil
}

func (t *locationFileTransportSpy) CreateDir(context.Context, locationdomain.Location, string) error {
	return nil
}

func (t *locationFileTransportSpy) CreateFile(context.Context, locationdomain.Location, string) error {
	return nil
}

func (t *locationFileTransportSpy) MoveFile(context.Context, locationdomain.Location, string, string) error {
	return nil
}

func (t *locationFileTransportSpy) CopyFile(context.Context, locationdomain.Location, string, string) error {
	return nil
}

func (t *locationFileTransportSpy) DeleteFile(context.Context, locationdomain.Location, string) error {
	return nil
}

func (t *locationFileTransportSpy) WriteFile(context.Context, locationdomain.Location, string, []byte) error {
	return nil
}

func testLocationRecord(id locationdomain.ID, ready bool) locationdomain.Record {
	now := time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)
	return locationdomain.Record{
		ID:     id,
		Kind:   locationdomain.KindSSH,
		Label:  "Build host",
		Status: locationdomain.StatusOnline,
		Ready:  ready,
		Address: locationdomain.Address{
			Host:     "build.example.test",
			Port:     22,
			Username: "agent",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
