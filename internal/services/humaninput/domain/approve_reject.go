package domain

type ApproveRejectPayload struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
}

type ApproveRejectResult struct {
	Cancelled bool   `json:"cancelled,omitempty"`
	Decision  string `json:"decision,omitempty"`
	Note      string `json:"note,omitempty"`
}
