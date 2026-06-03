package message

import (
	"encoding/json"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

const Name = "message"
const Description = "[COORDINATION] Member-addressed message gateway. Sends durable inbox messages to other active members and lets the current member inspect its durable inbox."

var allActions = []string{"send", "inbox"}

func (h Handler) Schema() json.RawMessage {
	return mustSchema()
}

func mustSchema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":                map[string]any{"type": "string", "enum": allActions},
			"destination_member_id": map[string]any{"type": "string", "description": "Destination active member id."},
			"kind":                  map[string]any{"type": "string", "enum": []string{string(types.AgentMessageKindInform), string(types.AgentMessageKindQuery), string(types.AgentMessageKindAck), string(types.AgentMessageKindResponse)}},
			"subject":               map[string]any{"type": "string"},
			"body":                  map[string]any{"type": "string"},
			"correlation_id":        map[string]any{"type": "string", "description": "Required for ack and response. Optional for inform and query; generated when omitted."},
			"status":                map[string]any{"type": "string", "enum": []string{string(types.MessageStatusQueuedTyped), string(types.MessageStatusConsumedTyped)}, "description": "For action=inbox. Defaults to all inbox statuses when omitted."},
			"limit":                 map[string]any{"type": "integer", "minimum": 0, "maximum": 50, "description": "For action=inbox. Defaults to 10 and caps at 50."},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("message schema encode: %v", err))
	}
	return body
}
