package rpc

import (
	"fmt"

	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	operatorrpc "github.com/tinoosan/agen8-mcp-server/internal/services/operator/rpc"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

func RegisterOperator(reg *Registry, operatorSvc *operatorapp.Service) error {
	if operatorSvc == nil {
		return fmt.Errorf("operator service is required")
	}
	handler := operatorrpc.NewHandler(operatorSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, protocol.MethodEscalationCreate, false, handler.CreateEscalation)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodEscalationGet, false, handler.GetEscalation)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodEscalationList, false, handler.ListEscalations)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodEscalationListPending, false, handler.ListPendingEscalations)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodEscalationResolve, false, handler.ResolveEscalation)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodEscalationCancel, false, handler.CancelEscalation)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodEscalationCountPending, false, handler.CountPendingEscalations)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionCreate, false, handler.CreateAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionGet, false, handler.GetAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionList, false, handler.ListActions)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionListPending, false, handler.ListPendingActions)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionAcknowledge, false, handler.AcknowledgeAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionStart, false, handler.StartAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionComplete, false, handler.CompleteAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionVerify, false, handler.VerifyAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionBlock, false, handler.BlockAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionUnblock, false, handler.UnblockAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionCancel, false, handler.CancelAction)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionAddNote, false, handler.AddNote)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionAddComment, false, handler.AddComment)
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodOpActionCountStatus, false, handler.CountActionStatus)
		},
	)
}
