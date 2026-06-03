package infra

import (
	"fmt"

	missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	"github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type RepositorySet struct {
	Missions        mission.Repository
	KeyResults      kr.KeyResultRepository
	ProgressEntries kr.ProgressEntryRepository
	LifecycleEvents missionapp.LifecycleEventRepository
}

func NewRepositories(handle *storagedb.Handle) (RepositorySet, error) {
	if handle == nil {
		return RepositorySet{}, fmt.Errorf("mission repositories: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		repo, err := NewSQLiteRepository(handle)
		if err != nil {
			return RepositorySet{}, err
		}
		return RepositorySet{Missions: repo, KeyResults: repo, ProgressEntries: repo, LifecycleEvents: repo}, nil
	case storagedb.DriverPostgres:
		repo, err := NewPostgresRepository(handle)
		if err != nil {
			return RepositorySet{}, err
		}
		return RepositorySet{Missions: repo, KeyResults: repo, ProgressEntries: repo, LifecycleEvents: repo}, nil
	default:
		return RepositorySet{}, fmt.Errorf("mission repositories: unsupported storage driver %q", handle.Driver())
	}
}
