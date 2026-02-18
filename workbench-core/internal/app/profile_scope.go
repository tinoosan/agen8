package app

import (
	"strings"

	"github.com/tinoosan/workbench-core/pkg/profile"
)

func buildRoleRuntimeProfile(role profile.RoleConfig) *profile.Profile {
	return &profile.Profile{
		ID:           strings.TrimSpace(role.Name),
		Description:  strings.TrimSpace(role.Description),
		Prompts:      role.Prompts,
		Skills:       append([]string(nil), role.Skills...),
		AllowedTools: append([]string(nil), role.AllowedTools...),
		Heartbeat:    append([]profile.HeartbeatJob(nil), role.Heartbeat...),
	}
}
