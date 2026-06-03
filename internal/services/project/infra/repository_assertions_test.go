package infra

import (
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
)

func TestRepositoryInterfaces(t *testing.T) {
	var _ project.Repository = (*SQLiteRepository)(nil)
	var _ project.Reader = (*SQLiteRepository)(nil)
	var _ project.Writer = (*SQLiteRepository)(nil)
	var _ project.Repository = (*PostgresRepository)(nil)
	var _ project.Reader = (*PostgresRepository)(nil)
	var _ project.Writer = (*PostgresRepository)(nil)
	var _ cluster.Repository = (*SQLiteClusterRepository)(nil)
	var _ cluster.Reader = (*SQLiteClusterRepository)(nil)
	var _ cluster.Writer = (*SQLiteClusterRepository)(nil)
	var _ cluster.Repository = (*PostgresClusterRepository)(nil)
	var _ cluster.Reader = (*PostgresClusterRepository)(nil)
	var _ cluster.Writer = (*PostgresClusterRepository)(nil)
}
