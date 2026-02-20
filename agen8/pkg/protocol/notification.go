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
)

// TurnNotificationParams are the params for turn.* notifications.
type TurnNotificationParams struct {
	Turn Turn `json:"turn"`
}

// ItemNotificationParams are the params for item.* notifications (except item.delta).
type ItemNotificationParams struct {
	Item Item `json:"item"`
}
