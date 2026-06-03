package infra

import (
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/apikey"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/password"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/session"
)

func TestRepositoryCompileTimeAssertions(t *testing.T) {
	var _ password.Repository = (*sqlitePasswordRepository)(nil)
	var _ session.Repository = (*sqliteSessionRepository)(nil)
	var _ apikey.Repository = (*sqliteAPIKeyRepository)(nil)

	var _ password.Repository = (*postgresPasswordRepository)(nil)
	var _ session.Repository = (*postgresSessionRepository)(nil)
	var _ apikey.Repository = (*postgresAPIKeyRepository)(nil)
}
