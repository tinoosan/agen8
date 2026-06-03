package infra

import (
	"context"
	"fmt"
	"strings"

	filedomain "github.com/tinoosan/agen8-mcp-server/internal/services/file/domain/file"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
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
