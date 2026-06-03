package rpc

import (
	"fmt"

	locationapp "github.com/tinoosan/agen8-mcp-server/internal/services/location/app"
	locationrpc "github.com/tinoosan/agen8-mcp-server/internal/services/location/rpc"
)

const (
	MethodLocationList                = "location.list"
	MethodLocationGet                 = "location.get"
	MethodLocationCreate              = "location.create"
	MethodLocationUpdate              = "location.update"
	MethodLocationDelete              = "location.delete"
	MethodLocationProbe               = "location.probe"
	MethodLocationInstall             = "location.installCodex"
	MethodLocationCodexAuthStatus     = "location.codexAuthStatus"
	MethodLocationCodexLogin          = "location.codexLogin"
	MethodLocationClaudeInstall       = "location.installClaude"
	MethodLocationClaudeAuthStatus    = "location.claudeAuthStatus"
	MethodLocationClaudeLogin         = "location.claudeLogin"
	MethodLocationClaudeLoginComplete = "location.claudeLoginComplete"
	MethodLocationFSListDir           = "location.fs.listDir"
)

func RegisterLocation(reg *Registry, locationSvc *locationapp.Service) error {
	if locationSvc == nil {
		return fmt.Errorf("location service is required")
	}
	handler := locationrpc.MustNewHandler(locationSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodLocationList, true, handler.LocationList)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationGet, false, handler.LocationGet)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationCreate, false, handler.LocationCreate)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationUpdate, false, handler.LocationUpdate)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationDelete, false, handler.LocationDelete)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationProbe, false, handler.LocationProbe)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationInstall, false, handler.LocationInstallCodex)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationCodexAuthStatus, false, handler.LocationCodexAuthStatus)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationCodexLogin, false, handler.LocationCodexLogin)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationClaudeInstall, false, handler.LocationInstallClaude)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationClaudeAuthStatus, false, handler.LocationClaudeAuthStatus)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationClaudeLogin, false, handler.LocationClaudeLogin)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationClaudeLoginComplete, false, handler.LocationClaudeLoginComplete)
		},
		func() error {
			return AddBoundHandler(reg, MethodLocationFSListDir, false, handler.LocationFSListDir)
		},
	)
}
