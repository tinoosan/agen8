package rpc

import (
	"fmt"

	pinapp "github.com/tinoosan/agen8/internal/services/pin/app"
	pinrpc "github.com/tinoosan/agen8/internal/services/pin/rpc"
)

const (
	MethodPinAdd    = "pin.add"
	MethodPinRemove = "pin.remove"
	MethodPinList   = "pin.list"
)

// RegisterPin wires the pin RPC methods. Pins are per-project and shared, so
// no caller identity is threaded through - the project scope is the boundary.
func RegisterPin(reg *Registry, pinSvc *pinapp.Service) error {
	if pinSvc == nil {
		return fmt.Errorf("pin service is required")
	}
	handler := pinrpc.NewHandler(pinSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodPinAdd, false, handler.Add)
		},
		func() error {
			return AddBoundHandler(reg, MethodPinRemove, false, handler.Remove)
		},
		func() error {
			return AddBoundHandler(reg, MethodPinList, false, handler.List)
		},
	)
}
