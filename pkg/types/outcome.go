package types

// HeartbeatOutcome is the structured result an agent sets after completing a
// heartbeat task. It flows through task metadata and event payloads.
type HeartbeatOutcome struct {
	Status  string   `json:"status"`
	Summary string   `json:"summary"`
	Actions []string `json:"actions,omitempty"`
}
