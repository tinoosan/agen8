package infra

import (
	"testing"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
)

func TestRepositoryInterfaces(t *testing.T) {
	var _ schedule.Repository = (*SQLiteRepository)(nil)
	var _ schedule.Repository = (*PostgresRepository)(nil)
}
