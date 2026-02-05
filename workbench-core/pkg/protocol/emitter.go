package protocol

// NotificationHandler receives protocol notifications.
//
// Phase 3 will implement this to send JSON-RPC notifications to clients.
type NotificationHandler func(method string, params any) error

// Emitter sends protocol notifications via a NotificationHandler.
type Emitter struct {
	handler NotificationHandler
}

// NewEmitter creates a new Emitter.
func NewEmitter(handler NotificationHandler) *Emitter {
	return &Emitter{handler: handler}
}

// EmitTurnStarted sends a turn.started notification.
func (e *Emitter) EmitTurnStarted(turn Turn) error {
	return e.emit(NotifyTurnStarted, TurnNotificationParams{Turn: turn})
}

// EmitTurnCompleted sends a turn.completed notification.
func (e *Emitter) EmitTurnCompleted(turn Turn) error {
	return e.emit(NotifyTurnCompleted, TurnNotificationParams{Turn: turn})
}

// EmitTurnFailed sends a turn.failed notification.
func (e *Emitter) EmitTurnFailed(turn Turn) error {
	return e.emit(NotifyTurnFailed, TurnNotificationParams{Turn: turn})
}

// EmitItemStarted sends an item.started notification.
func (e *Emitter) EmitItemStarted(item Item) error {
	return e.emit(NotifyItemStarted, ItemNotificationParams{Item: item})
}

// EmitItemDelta sends an item.delta notification.
func (e *Emitter) EmitItemDelta(itemID ItemID, delta ItemDelta) error {
	return e.emit(NotifyItemDelta, ItemDeltaParams{ItemID: itemID, Delta: delta})
}

// EmitItemCompleted sends an item.completed notification.
func (e *Emitter) EmitItemCompleted(item Item) error {
	return e.emit(NotifyItemCompleted, ItemNotificationParams{Item: item})
}

func (e *Emitter) emit(method string, params any) error {
	if e == nil || e.handler == nil {
		return nil
	}
	return e.handler(method, params)
}
