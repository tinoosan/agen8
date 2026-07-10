package infra

import (
	"fmt"

	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func NewRepository(handle *storagedb.Handle) (locationdomain.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("location repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("location repository requires SQLite storage")
	}
	return NewSQLiteRepository(handle)
}
