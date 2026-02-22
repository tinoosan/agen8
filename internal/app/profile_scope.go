package app

import (
	"strings"

	"github.com/tinoosan/agen8/pkg/profile"
)

func buildRoleRuntimeProfile(role profile.RoleConfig) *profile.Profile {
	codeExecOnly := false
	if role.CodeExecOnly != nil {
		codeExecOnly = *role.CodeExecOnly
	}
	return &profile.Profile{
		ID:                      strings.TrimSpace(role.Name),
		Description:             strings.TrimSpace(role.Description),
		CodeExecOnly:            codeExecOnly,
		CodeExecRequiredImports: append([]string(nil), role.CodeExecRequiredImports...),
		Prompts:                 role.Prompts,
		Skills:                  append([]string(nil), role.Skills...),
		AllowedTools:            append([]string(nil), role.AllowedTools...),
		Heartbeat: profile.HeartbeatConfig{
			Enabled: role.Heartbeat.Enabled,
			Jobs:    append([]profile.HeartbeatJob(nil), role.Heartbeat.Jobs...),
		},
	}
}
