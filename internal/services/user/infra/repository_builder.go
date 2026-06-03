package infra

import (
	"fmt"

	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func NewRepository(handle *storagedb.Handle) (user.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("user repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("user repository: unsupported storage driver %q", handle.Driver())
	}
}
