package infra

import (
	"fmt"

	pindomain "github.com/tinoosan/agen8/internal/services/pin/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

// NewRepository selects the dialect-specific pin repository for the handle's
// driver. Both implementations self-migrate their schema at construction.
func NewRepository(handle *storagedb.Handle) (pindomain.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("pin repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("pin repository: unsupported storage driver %q", handle.Driver())
	}
}
