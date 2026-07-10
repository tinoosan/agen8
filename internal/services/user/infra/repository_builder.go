package infra

import (
	"fmt"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func NewRepository(handle *storagedb.Handle) (user.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("user repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("user repository requires SQLite storage")
	}
	return NewSQLiteRepository(handle)
}
