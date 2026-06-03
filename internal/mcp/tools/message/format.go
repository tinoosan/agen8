package message

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

func memberLabel(member member.Record) string {
	label := strings.TrimSpace(member.DisplayName)
	if label != "" {
		return label
	}
	return strings.TrimSpace(string(member.ID))
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("message: encode structured response: %w", err)
	}
	return string(encoded), nil
}
