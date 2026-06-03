package infra

import "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"

var _ domain.TaskRepository = (*SQLiteRepository)(nil)
var _ domain.TaskRepository = (*PostgresRepository)(nil)
