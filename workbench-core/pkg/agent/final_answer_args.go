package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/types"
)

type finalAnswerArgs struct {
	Text      string
	Status    types.TaskStatus
	Error     string
	Artifacts []string
}

func parseFinalAnswerArgs(argsJSON string) (finalAnswerArgs, error) {
	argsJSON = strings.TrimSpace(argsJSON)
	if argsJSON == "" {
		return finalAnswerArgs{}, fmt.Errorf("final_answer args are required")
	}

	var raw map[string]json.RawMessage
	dec := json.NewDecoder(strings.NewReader(argsJSON))
	if err := dec.Decode(&raw); err != nil {
		return finalAnswerArgs{}, fmt.Errorf("final_answer args were not valid JSON: %w", err)
	}

	required := []string{"text", "status", "error", "artifacts"}
	for _, k := range required {
		if _, ok := raw[k]; !ok {
			return finalAnswerArgs{}, fmt.Errorf("final_answer.%s is required", k)
		}
	}

	var out finalAnswerArgs
	if err := json.Unmarshal(raw["text"], &out.Text); err != nil {
		return finalAnswerArgs{}, fmt.Errorf("final_answer.text must be a string: %w", err)
	}
	out.Text = strings.TrimSpace(out.Text)
	if out.Text == "" {
		return finalAnswerArgs{}, fmt.Errorf("final_answer.text is required")
	}

	var status string
	if err := json.Unmarshal(raw["status"], &status); err != nil {
		return finalAnswerArgs{}, fmt.Errorf("final_answer.status must be a string: %w", err)
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case string(types.TaskStatusSucceeded), string(types.TaskStatusFailed):
		out.Status = types.TaskStatus(status)
	default:
		return finalAnswerArgs{}, fmt.Errorf("final_answer.status must be 'succeeded' or 'failed'")
	}

	if err := json.Unmarshal(raw["error"], &out.Error); err != nil {
		return finalAnswerArgs{}, fmt.Errorf("final_answer.error must be a string: %w", err)
	}
	out.Error = strings.TrimSpace(out.Error)
	if out.Status == types.TaskStatusSucceeded && out.Error != "" {
		return finalAnswerArgs{}, fmt.Errorf("final_answer.error must be empty when status='succeeded'")
	}
	if out.Status == types.TaskStatusFailed && out.Error == "" {
		return finalAnswerArgs{}, fmt.Errorf("final_answer.error is required when status='failed'")
	}

	if err := json.Unmarshal(raw["artifacts"], &out.Artifacts); err != nil {
		return finalAnswerArgs{}, fmt.Errorf("final_answer.artifacts must be an array of strings: %w", err)
	}
	if out.Artifacts == nil {
		out.Artifacts = []string{}
	}

	return out, nil
}
