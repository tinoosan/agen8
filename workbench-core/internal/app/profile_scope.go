package app

import (
	"strings"

	"github.com/tinoosan/workbench-core/pkg/profile"
)

func buildRoleRuntimeProfile(role profile.RoleConfig) *profile.Profile {
	codeExecOnly := false
	if role.CodeExecOnly != nil {
		codeExecOnly = *role.CodeExecOnly
	}
	return &profile.Profile{
		ID:           strings.TrimSpace(role.Name),
		Description:  strings.TrimSpace(role.Description),
		CodeExecOnly: codeExecOnly,
		Prompts:      role.Prompts,
		Skills:       append([]string(nil), role.Skills...),
		AllowedTools: append([]string(nil), role.AllowedTools...),
		Heartbeat:    append([]profile.HeartbeatJob(nil), role.Heartbeat...),
	}
}
