package task

import taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"

var _ Service = (*taskapp.Service)(nil)
