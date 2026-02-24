package webhook

import "errors"

var (
	errGoalRequired = errors.New("goal is required")
	errInvalidRole  = errors.New("assignedRole is not a valid team role")
)
