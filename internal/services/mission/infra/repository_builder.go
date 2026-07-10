package infra

import (
	"fmt"

	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	"github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	"github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
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
	if handle.Driver() != storagedb.DriverSQLite {
		return RepositorySet{}, fmt.Errorf("mission repositories require SQLite storage")
	}
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		return RepositorySet{}, err
	}
	return RepositorySet{Missions: repo, KeyResults: repo, ProgressEntries: repo, LifecycleEvents: repo}, nil
}
