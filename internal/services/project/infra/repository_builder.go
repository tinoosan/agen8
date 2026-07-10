package infra

import (
	"fmt"

	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

// NewRepository builds the SQLite project repository used by the composition root.
func NewRepository(handle *storagedb.Handle) (project.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("project repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("project repository requires SQLite storage")
	}
	return NewSQLiteRepository(handle.DB())
}

// NewMemberRepository is the storage-strategy entry point for the project
// member roster.
func NewMemberRepository(handle *storagedb.Handle) (member.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("project member repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("project member repository requires SQLite storage")
	}
	return NewMemberSQLiteRepository(handle.DB())
}

// NewWorkspaceRepository is the storage-strategy entry point for project
// workspaces: the (location, root, machine) places a project is linked. A
// project owns many workspaces.
func NewWorkspaceRepository(handle *storagedb.Handle) (workspace.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("project workspace repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("project workspace repository requires SQLite storage")
	}
	return NewWorkspaceSQLiteRepository(handle.DB())
}
