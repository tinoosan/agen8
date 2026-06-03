package protocol

type ToolpolicyAuthorizeParams struct {
	SpaceID      string   `json:"spaceId,omitempty"`
	MemberType   string   `json:"memberType"`
	MemberCount  int      `json:"memberCount,omitempty"`
	HasReviewer  bool     `json:"hasReviewer,omitempty"`
	AllowedTools []string `json:"allowedTools,omitempty"`
}

type ToolpolicyAuthorizeResult struct {
	Allowed []string `json:"allowed"`
	Removed []string `json:"removed"`
}

type ToolpolicySystemToolsParams struct {
	MemberType  string `json:"memberType"`
	MemberCount int    `json:"memberCount,omitempty"`
	HasReviewer bool   `json:"hasReviewer,omitempty"`
}

type ToolpolicySystemToolsResult struct {
	Tools []string `json:"tools"`
}

type ToolpolicyDefaultsParams struct{}

type ToolpolicyDefaultsResult struct {
	WorkerTools            []string `json:"workerTools"`
	CoordinatorBase        []string `json:"coordinatorBase"`
	CoordinatorWithWorkers []string `json:"coordinatorWithWorkers"`
}
