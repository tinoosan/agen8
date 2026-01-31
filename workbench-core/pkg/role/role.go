package role

import (
	"strings"
	"time"
)

// Role defines the agent's standing identity and autonomous triggers.
// This is intentionally simple and explicit.
type Role struct {
	Name        string
	Description string

	// StandingGoals are background objectives the agent should work on when idle.
	StandingGoals []string

	// Triggers generate work without user input.
	Triggers []Trigger
}

// Trigger defines when to enqueue a goal.
type Trigger struct {
	// Type is one of: "interval", "time_of_day".
	Type string

	// Interval is used when Type == "interval".
	Interval time.Duration

	// TimeOfDay is used when Type == "time_of_day" and is in HH:MM (24h) local time.
	TimeOfDay string

	// Goal is the goal text to enqueue when the trigger fires.
	Goal string
}

func (r Role) Normalize() Role {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	outGoals := make([]string, 0, len(r.StandingGoals))
	for _, g := range r.StandingGoals {
		if s := strings.TrimSpace(g); s != "" {
			outGoals = append(outGoals, s)
		}
	}
	r.StandingGoals = outGoals
	outTrig := make([]Trigger, 0, len(r.Triggers))
	for _, t := range r.Triggers {
		t.Type = strings.ToLower(strings.TrimSpace(t.Type))
		t.TimeOfDay = strings.TrimSpace(t.TimeOfDay)
		t.Goal = strings.TrimSpace(t.Goal)
		outTrig = append(outTrig, t)
	}
	r.Triggers = outTrig
	return r
}
