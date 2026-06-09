package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8/internal/caller"
	notificationapp "github.com/tinoosan/agen8/internal/services/notification/app"
	notificationrpc "github.com/tinoosan/agen8/internal/services/notification/rpc"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

const (
	MethodNotificationList        = "notification.list"
	MethodNotificationMarkRead    = "notification.markRead"
	MethodNotificationMarkAllRead = "notification.markAllRead"
	MethodNotificationDismiss     = "notification.dismiss"
)

// RegisterNotification wires the notification RPC surface. Every method runs
// under the caller's identity so the inbox is scoped to the signed-in user.
func RegisterNotification(reg *Registry, notificationSvc *notificationapp.Service) error {
	if notificationSvc == nil {
		return fmt.Errorf("notification service is required")
	}
	handler := notificationrpc.NewHandler(notificationSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodNotificationList, false, withNotificationIdentity(handler.List))
		},
		func() error {
			return AddBoundHandler(reg, MethodNotificationMarkRead, false, withNotificationIdentity(handler.MarkRead))
		},
		func() error {
			return AddBoundHandler(reg, MethodNotificationMarkAllRead, false, withNotificationIdentity(handler.MarkAllRead))
		},
		func() error {
			return AddBoundHandler(reg, MethodNotificationDismiss, false, withNotificationIdentity(handler.Dismiss))
		},
	)
}

func withNotificationIdentity[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
	return func(ctx context.Context, params Params) (Result, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			var zero Result
			return zero, err
		}
		ctx = caller.ContextWithCaller(ctx, caller.Caller{
			UserID:   identity.UserID,
			MemberID: member.ID(identity.MemberID),
			Role:     identity.Role,
		})
		return fn(ctx, params)
	}
}
