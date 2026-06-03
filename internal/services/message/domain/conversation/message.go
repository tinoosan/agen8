package conversation

import (
	"fmt"
	"time"
)

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
	DirectionSystem   Direction = "system"
)

type DeliveryState string

const (
	DeliveryQueued    DeliveryState = "queued"
	DeliveryDelivered DeliveryState = "delivered"
	DeliverySteered   DeliveryState = "steered"
	DeliveryFailed    DeliveryState = "failed"
)

type RenderState string

const (
	RenderVisible RenderState = "visible"
	RenderError   RenderState = "error"
)

type Message struct {
	ID          string
	ChannelID   string
	SpaceID     string
	MemberID    string
	SessionID   string
	TurnID      string
	Direction   Direction
	SenderType  string
	SenderID    string
	Text        string
	Attachments []Attachment
	Delivery    DeliveryState
	Render      RenderState
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Error       string
}

type MessageParams struct {
	ID          string
	ChannelID   string
	SpaceID     string
	MemberID    string
	SessionID   string
	TurnID      string
	Direction   Direction
	SenderType  string
	SenderID    string
	Text        string
	Attachments []Attachment
	Delivery    DeliveryState
	Render      RenderState
	Now         time.Time
}

func NewMessage(params MessageParams) (Message, error) {
	msg := Message{
		ID:          params.ID,
		ChannelID:   params.ChannelID,
		SpaceID:     params.SpaceID,
		MemberID:    params.MemberID,
		SessionID:   params.SessionID,
		TurnID:      params.TurnID,
		Direction:   params.Direction,
		SenderType:  params.SenderType,
		SenderID:    params.SenderID,
		Text:        params.Text,
		Attachments: append([]Attachment(nil), params.Attachments...),
		Delivery:    params.Delivery,
		Render:      params.Render,
		CreatedAt:   params.Now,
		UpdatedAt:   params.Now,
	}
	if err := ValidateMessage(msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func ValidateMessage(msg Message) error {
	if msg.ID == "" {
		return fmt.Errorf("id is required")
	}
	if msg.ChannelID == "" {
		return fmt.Errorf("channelID is required")
	}
	if msg.SpaceID == "" {
		return fmt.Errorf("spaceID is required")
	}
	if msg.MemberID == "" {
		return fmt.Errorf("memberID is required")
	}
	if err := ValidateDirection(msg.Direction); err != nil {
		return err
	}
	if msg.SenderType == "" {
		return fmt.Errorf("senderType is required")
	}
	if msg.Text == "" && len(msg.Attachments) == 0 {
		return fmt.Errorf("text or attachment is required")
	}
	for _, attachment := range msg.Attachments {
		if err := ValidateAttachment(attachment); err != nil {
			return err
		}
		if attachment.SpaceID != msg.SpaceID {
			return fmt.Errorf("attachment %s belongs to space %s, not %s", attachment.ID, attachment.SpaceID, msg.SpaceID)
		}
		if attachment.ChannelID != msg.ChannelID {
			return fmt.Errorf("attachment %s belongs to channel %s, not %s", attachment.ID, attachment.ChannelID, msg.ChannelID)
		}
	}
	if err := ValidateRenderState(msg.Render); err != nil {
		return err
	}
	if msg.CreatedAt.IsZero() {
		return fmt.Errorf("createdAt is required")
	}
	if msg.UpdatedAt.IsZero() {
		return fmt.Errorf("updatedAt is required")
	}
	switch msg.Direction {
	case DirectionInbound:
		if err := ValidateDeliveryState(msg.Delivery); err != nil {
			return err
		}
	case DirectionOutbound, DirectionSystem:
		if msg.Delivery != "" {
			return fmt.Errorf("delivery state is only valid for inbound messages")
		}
	}
	if msg.Render == RenderError && msg.Error == "" {
		return fmt.Errorf("error is required when render state is error")
	}
	if msg.Delivery == DeliveryFailed && msg.Error == "" {
		return fmt.Errorf("error is required when delivery state is failed")
	}
	return nil
}

func ValidateDirection(direction Direction) error {
	switch direction {
	case DirectionInbound, DirectionOutbound, DirectionSystem:
		return nil
	case "":
		return fmt.Errorf("direction is required")
	default:
		return fmt.Errorf("unsupported direction %q", direction)
	}
}

func ValidateDeliveryState(state DeliveryState) error {
	switch state {
	case DeliveryQueued, DeliveryDelivered, DeliverySteered, DeliveryFailed:
		return nil
	case "":
		return fmt.Errorf("delivery state is required")
	default:
		return fmt.Errorf("unsupported delivery state %q", state)
	}
}

func ValidateRenderState(state RenderState) error {
	switch state {
	case RenderVisible, RenderError:
		return nil
	case "":
		return fmt.Errorf("render state is required")
	default:
		return fmt.Errorf("unsupported render state %q", state)
	}
}
