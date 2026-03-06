package agent

import (
	"context"
	"strings"
)

// SteeringPromptSource wraps another PromptSource and injects periodic steering reminders.
// It is intended as a low-overhead safety net for critical behaviors in long runs.
type SteeringPromptSource struct {
	base      PromptSource
	everyStep int
}

// NewSteeringPromptSource creates a PromptSource that appends a steering reminder
// every N steps (default: 5). If base is nil, it augments the provided basePrompt directly.
func NewSteeringPromptSource(base PromptSource) *SteeringPromptSource {
	return &SteeringPromptSource{base: base, everyStep: 5}
}

func (s *SteeringPromptSource) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	prompt := basePrompt
	if s != nil && s.base != nil {
		updated, err := s.base.SystemPrompt(ctx, basePrompt, step)
		if err != nil {
			return "", err
		}
		prompt = updated
	}

	every := 5
	if s != nil && s.everyStep > 0 {
		every = s.everyStep
	}
	if step > 0 && every > 0 && step%every == 0 {
		prompt = strings.TrimSpace(prompt) + "\n\n" + strings.TrimSpace(steeringReminderMessage())
	}
	return prompt, nil
}

func steeringReminderMessage() string {
	return `
<reminder>
Before ending the task:
1) Verify the goal is complete and your work is validated. For coordinators: delegating all specialist tasks counts as completing the current task — do not wait for callbacks.
2) Send the completion email using the email tool (email MUST happen BEFORE final_answer; final_answer ends the run).
3) Call final_answer with the completion report, "status", "error", and an "artifacts" array listing /workspace file paths. Use empty array when not applicable.
</reminder>
`
}
