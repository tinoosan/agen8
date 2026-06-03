package infra

import (
	"fmt"

	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func NewRepository(handle *storagedb.Handle) (locationdomain.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("location repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("location repository: unsupported storage driver %q", handle.Driver())
	}
}
