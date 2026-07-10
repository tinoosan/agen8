package infra

import (
	"testing"

	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	"github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	"github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

func TestRepositoryInterfaces(t *testing.T) {
	var _ mission.Repository = (*SQLiteRepository)(nil)
	var _ kr.KeyResultRepository = (*SQLiteRepository)(nil)
	var _ kr.ProgressEntryRepository = (*SQLiteRepository)(nil)
	var _ missionapp.LifecycleEventRepository = (*SQLiteRepository)(nil)
}
