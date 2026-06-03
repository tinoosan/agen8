package message

import (
	"encoding/json"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

const Name = "message"
const Description = "[COORDINATION] Member-addressed message gateway. Sends durable inbox messages to other active members."

var allActions = []string{"send"}

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
		},
		"required":             []string{"action", "destination_member_id", "kind", "subject", "body"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("message schema encode: %v", err))
	}
	return body
}
