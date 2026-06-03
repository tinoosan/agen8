package infra

import (
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func NewRepository(handle *storagedb.Handle) (domain.SessionRepository, error) {
	if handle == nil {
		return nil, fmt.Errorf("harness session repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteSessionRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresSessionRepository(handle)
	default:
		return nil, fmt.Errorf("harness session repository: unsupported storage driver %q", handle.Driver())
	}
}
