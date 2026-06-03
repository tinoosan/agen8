package protocol

import "encoding/json"

const (
	MethodChannelHumanInputPending = "channel.human_input.pending"
	MethodChannelHumanInputSubmit  = "channel.human_input.submit"
	MethodChannelHumanInputCancel  = "channel.human_input.cancel"

	NotifyChannelHumanInputChanged = "channel.human_input.changed"
)

// PendingHumanInput is the wire-format projection of a pending row.
//
// SpaceID + MemberID identify the asker (who is blocked); ChannelID is
// the panel delivery target. The UI subscribes by ChannelID.
type PendingHumanInput struct {
	SpaceID     string          `json:"spaceId"`
	MemberID    string          `json:"memberId"`
	ChannelID   string          `json:"channelId"`
	ToolCallID  string          `json:"toolCallId"`
	ToolName    string          `json:"toolName"`
	Primitive   string          `json:"primitive"`
	PayloadJSON json.RawMessage `json:"payload"`
	ProjectID   string          `json:"projectId"`
	CreatedAt   string          `json:"createdAt"`
}

type ChannelHumanInputPendingParams struct {
	ChannelID string `json:"channelId"`
}

type ChannelHumanInputPendingResult struct {
	Pending *PendingHumanInput `json:"pending"`
}

type ChannelHumanInputSubmitParams struct {
	SpaceID    string          `json:"spaceId"`
	MemberID   string          `json:"memberId"`
	ToolCallID string          `json:"toolCallId"`
	Result     json.RawMessage `json:"result"`
}

type ChannelHumanInputSubmitResult struct {
	OK bool `json:"ok"`
}

type ChannelHumanInputCancelParams struct {
	SpaceID    string `json:"spaceId"`
	MemberID   string `json:"memberId"`
	ToolCallID string `json:"toolCallId"`
}

type ChannelHumanInputCancelResult struct {
	OK bool `json:"ok"`
}
