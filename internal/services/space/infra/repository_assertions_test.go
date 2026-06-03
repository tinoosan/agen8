package infra

import (
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

var _ domain.Repository = (*SQLiteRepository)(nil)
var _ domain.Repository = (*PostgresRepository)(nil)
var _ member.Repository = (*MemberSQLiteRepository)(nil)
var _ member.Repository = (*MemberPostgresRepository)(nil)
