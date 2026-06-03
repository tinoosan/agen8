package protocol

type ControlSetModelParams struct {
	SpaceID SpaceID `json:"spaceId"`
	Model   string  `json:"model"`
	Target  string  `json:"target,omitempty"`
}

type ControlSetModelResult struct {
	Accepted  bool     `json:"accepted"`
	AppliedTo []string `json:"appliedTo,omitempty"`
}

type ControlSetReasoningParams struct {
	SpaceID SpaceID `json:"spaceId"`
	Effort  string  `json:"effort,omitempty"`
	Summary string  `json:"summary,omitempty"`
	Target  string  `json:"target,omitempty"`
}

type ControlSetReasoningResult struct {
	Accepted  bool     `json:"accepted"`
	AppliedTo []string `json:"appliedTo,omitempty"`
	Effort    string   `json:"effort,omitempty"`
	Summary   string   `json:"summary,omitempty"`
}
