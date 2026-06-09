package infra

import (
	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func NewPostgresRepository(handle *storagedb.Handle, dataDir string) (credentialdomain.Repository, error) {
	return newSQLStore(handle, dataDir), nil
}
