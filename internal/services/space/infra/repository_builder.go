package infra

import (
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// NewRepository selects the space repository implementation from the configured
// storage driver carried by the shared DB handle.
func NewRepository(handle *storagedb.Handle) (domain.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("space repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle.DB()), nil
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle.DB()), nil
	default:
		return nil, fmt.Errorf("space repository: unsupported storage driver %q", handle.Driver())
	}
}

// NewMemberRepository selects the space member repository implementation from
// the configured storage driver carried by the shared DB handle.
func NewMemberRepository(handle *storagedb.Handle) (member.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("space member repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewMemberSQLiteRepository(handle.DB()), nil
	case storagedb.DriverPostgres:
		return NewMemberPostgresRepository(handle.DB()), nil
	default:
		return nil, fmt.Errorf("space member repository: unsupported storage driver %q", handle.Driver())
	}
}
