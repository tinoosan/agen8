package infra

import (
	"fmt"

	"github.com/tinoosan/agen8/internal/services/task/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

// NewRepository builds the SQLite task repository used by the composition root.
func NewRepository(handle *storagedb.Handle) (domain.TaskRepository, error) {
	if handle == nil {
		return nil, fmt.Errorf("task repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("task repository requires SQLite storage")
	}
	return NewSQLiteRepository(handle)
}
