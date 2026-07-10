package infra

import (
	"testing"

	"github.com/tinoosan/agen8/internal/services/auth/apikey"
	"github.com/tinoosan/agen8/internal/services/auth/password"
	"github.com/tinoosan/agen8/internal/services/auth/session"
)

func TestRepositoryCompileTimeAssertions(t *testing.T) {
	var _ password.Repository = (*sqlitePasswordRepository)(nil)
	var _ session.Repository = (*sqliteSessionRepository)(nil)
	var _ apikey.Repository = (*sqliteAPIKeyRepository)(nil)
}
