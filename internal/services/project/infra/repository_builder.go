package infra

import (
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// NewRepository is the storage-strategy entry point used by the composition
// root. The concrete repository types remain SQLiteRepository and
// PostgresRepository; this function only selects between them from config.
func NewRepository(handle *storagedb.Handle) (project.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("project repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle.DB())
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("project repository: unsupported storage driver %q", handle.Driver())
	}
}

// NewClusterRepository is the storage-strategy entry point for project cluster
// topology. It selects the implementation from the configured DB driver.
func NewClusterRepository(handle *storagedb.Handle) (cluster.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("project cluster repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteClusterRepository(handle.DB())
	case storagedb.DriverPostgres:
		return NewPostgresClusterRepository(handle)
	default:
		return nil, fmt.Errorf("project cluster repository: unsupported storage driver %q", handle.Driver())
	}
}
