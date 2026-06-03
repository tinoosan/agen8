package conversation

import (
	"context"
	"time"
)

type Reader interface {
	Get(ctx context.Context, id string) (*Message, error)
	ListByChannel(ctx context.Context, channelID string, limit int) ([]Message, error)
	ListActivitiesByChannel(ctx context.Context, channelID string, limit int) ([]Activity, error)
	NextQueuedForSession(ctx context.Context, sessionID string) (*Message, error)
	GetAttachments(ctx context.Context, ids []string) ([]Attachment, error)
}

type Writer interface {
	Save(ctx context.Context, message Message) error
	SaveActivity(ctx context.Context, activity Activity) error
	SaveAttachment(ctx context.Context, attachment Attachment) error
	AppendText(ctx context.Context, id string, delta string, updatedAt time.Time) (Message, error)
	UpdateDelivery(ctx context.Context, id string, state DeliveryState, errText string, updatedAt time.Time) (Message, error)
	UpdateDeliveryBinding(ctx context.Context, id string, state DeliveryState, sessionID string, turnID string, errText string, updatedAt time.Time) (Message, error)
	UpdateRender(ctx context.Context, id string, state RenderState, errText string, updatedAt time.Time) (Message, error)
}

type Repository interface {
	Reader
	Writer
}
