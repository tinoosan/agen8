package team

// Manifest is the team identity and role bindings persisted to disk and exposed via RPC.
// JSON tags match existing team_daemon.go so manifest files and RPC responses remain valid.
type Manifest struct {
	TeamID          string       `json:"teamId"`
	ProfileID       string       `json:"profileId"`
	TeamModel       string       `json:"teamModel,omitempty"`
	ModelChange     *ModelChange `json:"modelChange,omitempty"`
	CoordinatorRole string       `json:"coordinatorRole"`
	CoordinatorRun  string       `json:"coordinatorRunId"`
	Roles           []RoleRecord `json:"roles"`
	// DesiredReplicasByRole optionally configures persistent worker pool size by role.
	// Missing entries mean "manual management" for that role.
	DesiredReplicasByRole map[string]int `json:"desiredReplicasByRole,omitempty"`
	CreatedAt             string         `json:"createdAt"`
}

// ModelChange represents a pending, applied, or failed team model change.
type ModelChange struct {
	RequestedModel string `json:"requestedModel,omitempty"`
	Status         string `json:"status,omitempty"` // pending|applied|failed
	RequestedAt    string `json:"requestedAt,omitempty"`
	AppliedAt      string `json:"appliedAt,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Error          string `json:"error,omitempty"`
}

// RoleRecord binds a role name to a run and session.
type RoleRecord struct {
	RoleName  string `json:"roleName"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
}
