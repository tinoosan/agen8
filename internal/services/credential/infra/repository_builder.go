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
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle, dataDir)
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle, dataDir)
	default:
		return nil, fmt.Errorf("credential repository: unsupported storage driver %q", handle.Driver())
	}
}
