package protocol

type ConfigGetParams struct{}

type ConfigGetResult struct {
	Logging ConfigLogging `json:"logging"`
}

type ConfigLogging struct {
	Level     string `json:"level,omitempty"`
	Format    string `json:"format,omitempty"`
	Quiet     *bool  `json:"quiet,omitempty"`
	FilePath  string `json:"filePath,omitempty"`
	MaxSizeMB int    `json:"maxSizeMB,omitempty"`
}

type ConfigUpdateParams struct {
	Logging *ConfigLogging `json:"logging,omitempty"`
}

type ConfigUpdateResult struct {
	Config          ConfigGetResult `json:"config"`
	AppliedNow      []string        `json:"appliedNow"`
	RestartRequired []string        `json:"restartRequired"`
	Warnings        []string        `json:"warnings,omitempty"`
}

type ProjectConfigGetParams struct {
	ProjectRoot string `json:"projectRoot"`
}

type ProjectConfigGetResult struct {
	ProjectID        string `json:"projectId"`
	ProjectMountRoot string `json:"projectMountRoot,omitempty"`
}

type ProjectConfigUpdateParams struct {
	ProjectRoot      string `json:"projectRoot"`
	ProjectMountRoot string `json:"projectMountRoot,omitempty"`
}

type ProjectConfigUpdateResult struct {
	Config ProjectConfigGetResult `json:"config"`
}
