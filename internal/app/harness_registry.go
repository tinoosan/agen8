package app

import (
	harnessdomain "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra/claudecli"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra/codex"
)

func defaultHarnessRuntimes() []harnessdomain.Runtime {
	return []harnessdomain.Runtime{
		codex.New(),
		claudecli.New(),
	}
}
