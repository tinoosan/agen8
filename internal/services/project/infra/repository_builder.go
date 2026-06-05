package infra

import (
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// NewRepository is the storage-strategy entry point used by the composition
// root. The concrete repository types remain SQLiteRepository and
// PostgresRepository; this function only selects between them from config.
func NewRepository(handle *storagedb.Handle) (project.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("project repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle.DB())
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("project repository: unsupported storage driver %q", handle.Driver())
	}
}

// NewMemberRepository is the storage-strategy entry point for the project
// member roster. Projects own their roster directly, so the member store is
// selected from the same configured DB driver.
func NewMemberRepository(handle *storagedb.Handle) (member.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("project member repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewMemberSQLiteRepository(handle.DB())
	case storagedb.DriverPostgres:
		return NewMemberPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("project member repository: unsupported storage driver %q", handle.Driver())
	}
}
