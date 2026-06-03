package protocol

type MemberRunListParams struct {
	SpaceID SpaceID `json:"spaceId"`
}

type MemberRunListItem struct {
	RunID       string `json:"runId"`
	SpaceID     string `json:"spaceId"`
	Status      string `json:"status,omitempty"`
	Goal        string `json:"goal,omitempty"`
	MemberName  string `json:"memberName,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

type MemberRunListResult struct {
	MemberRuns []MemberRunListItem `json:"memberRuns"`
}

type MemberRunStartParams struct {
	SpaceID SpaceID `json:"spaceId"`
	Goal    string  `json:"goal,omitempty"`
	Model   string  `json:"model,omitempty"`
}

type MemberRunStartResult struct {
	RunID   string `json:"runId"`
	SpaceID string `json:"spaceId"`
	Model   string `json:"model,omitempty"`
}
