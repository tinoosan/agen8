package infra

import (
	"fmt"

	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func NewRepository(handle *storagedb.Handle, dataDir string) (credentialdomain.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("credential repository: db handle is required")
	}
	if dataDir == "" {
		return nil, fmt.Errorf("credential data dir is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("credential repository requires SQLite storage")
	}
	return NewSQLiteRepository(handle, dataDir)
}
