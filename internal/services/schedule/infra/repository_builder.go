package infra

import (
	"fmt"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func NewRepository(handle *storagedb.Handle) (schedule.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("schedule repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("schedule repository: unsupported storage driver %q", handle.Driver())
	}
}
