package protocol

// Notification method names.
const (
	NotifyTurnStarted   = "turn.started"
	NotifyTurnCompleted = "turn.completed"
	NotifyTurnFailed    = "turn.failed"
	NotifyTurnCanceled  = "turn.canceled"

	NotifyItemStarted   = "item.started"
	NotifyItemDelta     = "item.delta"
	NotifyItemCompleted = "item.completed"

	// NotifyEventAppend is sent when the daemon appends an event to the store (real-time push).
	NotifyEventAppend               = "event.append"
	NotifyProjectReconcileStarted   = "project.reconcile.started"
	NotifyProjectReconcileDrift     = "project.reconcile.drift"
	NotifyProjectReconcileConverged = "project.reconcile.converged"
	NotifyProjectReconcileFailed    = "project.reconcile.failed"
)

// TurnNotificationParams are the params for turn.* notifications.
type TurnNotificationParams struct {
	Turn Turn `json:"turn"`
}

// ItemNotificationParams are the params for item.* notifications (except item.delta).
type ItemNotificationParams struct {
	Item Item `json:"item"`
}
