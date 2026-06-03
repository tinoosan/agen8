package infra

import (
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// NewRepository is the storage-strategy entry point used by the composition
// root. The concrete repository types remain SQLiteRepository and
// PostgresRepository; this function only selects between them from config.
func NewRepository(handle *storagedb.Handle) (domain.TaskRepository, error) {
	if handle == nil {
		return nil, fmt.Errorf("task repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("task repository: unsupported storage driver %q", handle.Driver())
	}
}
