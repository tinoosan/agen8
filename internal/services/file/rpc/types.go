package rpc

type ListDirParams struct {
	ProjectID   string `json:"projectId"`
	ProjectRoot string `json:"projectRoot"`
	Path        string `json:"path,omitempty"`
}

type GetParams struct {
	ProjectID   string `json:"projectId"`
	ProjectRoot string `json:"projectRoot"`
	Path        string `json:"path"`
	MaxBytes    int64  `json:"maxBytes,omitempty"`
}

type PathParams struct {
	ProjectID   string `json:"projectId"`
	ProjectRoot string `json:"projectRoot"`
	Path        string `json:"path"`
}

type MoveParams struct {
	ProjectID   string `json:"projectId"`
	ProjectRoot string `json:"projectRoot"`
	Path        string `json:"path"`
	Destination string `json:"destination"`
}

type UploadParams struct {
	ProjectID   string `json:"projectId"`
	ProjectRoot string `json:"projectRoot"`
	Path        string `json:"path"`
	Content     string `json:"content,omitempty"`
	BytesB64    string `json:"bytesB64,omitempty"`
}
