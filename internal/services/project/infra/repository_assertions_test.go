package infra

import (
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
)

func TestRepositoryInterfaces(t *testing.T) {
	var _ project.Repository = (*SQLiteRepository)(nil)
	var _ project.Reader = (*SQLiteRepository)(nil)
	var _ project.Writer = (*SQLiteRepository)(nil)
	var _ project.Repository = (*PostgresRepository)(nil)
	var _ project.Reader = (*PostgresRepository)(nil)
	var _ project.Writer = (*PostgresRepository)(nil)
}
