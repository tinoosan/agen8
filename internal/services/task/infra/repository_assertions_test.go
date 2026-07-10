package infra

import "github.com/tinoosan/agen8/internal/services/task/domain"

var _ domain.TaskRepository = (*SQLiteRepository)(nil)
