package infra

import locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"

var (
	_ locationdomain.Repository = (*SQLiteRepository)(nil)
	_ locationdomain.Repository = (*PostgresRepository)(nil)
)
