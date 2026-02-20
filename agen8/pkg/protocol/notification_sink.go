package protocol

import (
	"github.com/tinoosan/agen8/pkg/emit"
)

// Notification is a protocol notification (method + params).
type Notification struct {
	Method string
	Params any
}

// NotificationSink receives protocol notifications.
type NotificationSink = emit.Sink[Notification]
