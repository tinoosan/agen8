package protocol

type ControlSetModelParams struct {
	ThreadID ThreadID `json:"threadId"`
	Model    string   `json:"model"`
	Target   string   `json:"target,omitempty"` // optional run/role target
}

type ControlSetModelResult struct {
	Accepted  bool     `json:"accepted"`
	AppliedTo []string `json:"appliedTo,omitempty"`
}

type ControlSetReasoningParams struct {
	ThreadID ThreadID `json:"threadId"`
	Effort   string   `json:"effort,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Target   string   `json:"target,omitempty"` // optional run/role target
}

type ControlSetReasoningResult struct {
	Accepted  bool     `json:"accepted"`
	AppliedTo []string `json:"appliedTo,omitempty"`
	Effort    string   `json:"effort,omitempty"`
	Summary   string   `json:"summary,omitempty"`
}

type ControlSetProfileParams struct {
	ThreadID ThreadID `json:"threadId"`
	Profile  string   `json:"profile"`
	Target   string   `json:"target,omitempty"` // optional run/role target
}

type ControlSetProfileResult struct {
	Accepted                bool     `json:"accepted"`
	AppliedTo               []string `json:"appliedTo,omitempty"`
	PreservesSessionContext bool     `json:"preservesSessionContext"`
}
