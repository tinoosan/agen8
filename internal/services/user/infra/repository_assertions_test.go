package infra

import user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"

var _ user.Repository = (*SQLiteRepository)(nil)
var _ user.Repository = (*PostgresRepository)(nil)
