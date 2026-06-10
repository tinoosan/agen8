package infra

import (
	"context"
	"fmt"
	"strings"

	filedomain "github.com/tinoosan/agen8/internal/services/file/domain/file"
	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
)

type LocationFileTransport interface {
	StatFile(ctx context.Context, location locationdomain.Location, path string) (filedomain.Info, error)
	ListFiles(ctx context.Context, location locationdomain.Location, path string) ([]filedomain.Entry, error)
	ReadFile(ctx context.Context, location locationdomain.Location, path string, maxBytes int64) (filedomain.Content, error)
	CreateDir(ctx context.Context, location locationdomain.Location, path string) error
	CreateFile(ctx context.Context, location locationdomain.Location, path string) error
	MoveFile(ctx context.Context, location locationdomain.Location, source string, destination string) error
	CopyFile(ctx context.Context, location locationdomain.Location, source string, destination string) error
	DeleteFile(ctx context.Context, location locationdomain.Location, path string) error
	WriteFile(ctx context.Context, location locationdomain.Location, path string, contents []byte) error
	GitShowBaseline(ctx context.Context, location locationdomain.Location, dir, name string) (filedomain.GitBaseline, error)
}

type LocationRepository struct {
	locations locationdomain.Repository
	transport LocationFileTransport
}

func NewLocationRepository(locations locationdomain.Repository, transport LocationFileTransport) (*LocationRepository, error) {
	if locations == nil {
		return nil, fmt.Errorf("file location repository: location repository is required")
	}
	if transport == nil {
		return nil, fmt.Errorf("file location repository: transport is required")
	}
	return &LocationRepository{locations: locations, transport: transport}, nil
}

var _ filedomain.Repository = (*LocationRepository)(nil)

func (r *LocationRepository) Stat(ctx context.Context, ref filedomain.Reference) (filedomain.Info, error) {
	location, err := r.location(ctx, ref)
	if err != nil {
		return filedomain.Info{}, err
	}
	return r.transport.StatFile(ctx, location, ref.Path)
}

func (r *LocationRepository) ListDir(ctx context.Context, ref filedomain.Reference) ([]filedomain.Entry, error) {
	location, err := r.location(ctx, ref)
	if err != nil {
		return nil, err
	}
	return r.transport.ListFiles(ctx, location, ref.Path)
}

func (r *LocationRepository) Read(ctx context.Context, ref filedomain.Reference, maxBytes int64) (filedomain.Content, error) {
	location, err := r.location(ctx, ref)
	if err != nil {
		return filedomain.Content{}, err
	}
	return r.transport.ReadFile(ctx, location, ref.Path, maxBytes)
}

func (r *LocationRepository) CreateDir(ctx context.Context, ref filedomain.Reference) error {
	location, err := r.location(ctx, ref)
	if err != nil {
		return err
	}
	return r.transport.CreateDir(ctx, location, ref.Path)
}

func (r *LocationRepository) CreateFile(ctx context.Context, ref filedomain.Reference) error {
	location, err := r.location(ctx, ref)
	if err != nil {
		return err
	}
	return r.transport.CreateFile(ctx, location, ref.Path)
}

func (r *LocationRepository) Move(ctx context.Context, source filedomain.Reference, destination filedomain.Reference) error {
	if source.LocationID != destination.LocationID {
		return fmt.Errorf("cannot move files across locations")
	}
	location, err := r.location(ctx, source)
	if err != nil {
		return err
	}
	return r.transport.MoveFile(ctx, location, source.Path, destination.Path)
}

func (r *LocationRepository) Copy(ctx context.Context, source filedomain.Reference, destination filedomain.Reference) error {
	if source.LocationID != destination.LocationID {
		return fmt.Errorf("cannot copy files across locations")
	}
	location, err := r.location(ctx, source)
	if err != nil {
		return err
	}
	return r.transport.CopyFile(ctx, location, source.Path, destination.Path)
}

func (r *LocationRepository) Delete(ctx context.Context, ref filedomain.Reference) error {
	location, err := r.location(ctx, ref)
	if err != nil {
		return err
	}
	return r.transport.DeleteFile(ctx, location, ref.Path)
}

func (r *LocationRepository) WriteFile(ctx context.Context, ref filedomain.Reference, contents []byte) error {
	location, err := r.location(ctx, ref)
	if err != nil {
		return err
	}
	return r.transport.WriteFile(ctx, location, ref.Path, contents)
}

// GitBaseline runs the read-only remote git baseline for a file, but only if
// the location has been granted the git-diff capability. The capability gate
// lives here (default-off, human-granted) so the file service never reaches a
// remote command without an explicit opt-in. Returns ErrGitBaselineNotPermitted
// when the grant is absent.
func (r *LocationRepository) GitBaseline(ctx context.Context, ref filedomain.Reference, dir, name string) (filedomain.GitBaseline, error) {
	location, err := r.location(ctx, ref)
	if err != nil {
		return filedomain.GitBaseline{}, err
	}
	if !location.GitDiffEnabled() {
		return filedomain.GitBaseline{}, filedomain.ErrGitBaselineNotPermitted
	}
	return r.transport.GitShowBaseline(ctx, location, dir, name)
}

func (r *LocationRepository) location(ctx context.Context, ref filedomain.Reference) (locationdomain.Location, error) {
	locationID := locationdomain.ID(strings.TrimSpace(string(ref.LocationID)))
	if locationID == "" {
		locationID = "local"
	}
	if strings.TrimSpace(ref.Path) == "" {
		return locationdomain.Location{}, fmt.Errorf("file path is required")
	}
	record, err := r.locations.Get(ctx, locationID)
	if err != nil {
		return locationdomain.Location{}, err
	}
	location, err := locationdomain.Wrap(record)
	if err != nil {
		return locationdomain.Location{}, err
	}
	if !location.Ready() {
		return locationdomain.Location{}, fmt.Errorf("location %q is not ready", location.ID())
	}
	return location, nil
}
