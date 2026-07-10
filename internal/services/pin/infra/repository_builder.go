package infra

import (
	"fmt"

	pindomain "github.com/tinoosan/agen8/internal/services/pin/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

// NewRepository builds the SQLite pin repository used by the application.
func NewRepository(handle *storagedb.Handle) (pindomain.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("pin repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("pin repository requires SQLite storage")
	}
	return NewSQLiteRepository(handle)
}
