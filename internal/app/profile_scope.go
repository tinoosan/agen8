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
	var heartbeatEnabled *bool
	if role.Heartbeat.Enabled != nil {
		v := *role.Heartbeat.Enabled
		heartbeatEnabled = &v
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
			Enabled: heartbeatEnabled,
			Jobs:    append([]profile.HeartbeatJob(nil), role.Heartbeat.Jobs...),
		},
	}
}

func buildReviewerRuntimeProfile(reviewer profile.ReviewerConfig) *profile.Profile {
	codeExecOnly := false
	if reviewer.CodeExecOnly != nil {
		codeExecOnly = *reviewer.CodeExecOnly
	}
	var heartbeatEnabled *bool
	if reviewer.Heartbeat.Enabled != nil {
		v := *reviewer.Heartbeat.Enabled
		heartbeatEnabled = &v
	}
	return &profile.Profile{
		ID:                      strings.TrimSpace(reviewer.EffectiveName()),
		Description:             strings.TrimSpace(reviewer.Description),
		CodeExecOnly:            codeExecOnly,
		CodeExecRequiredImports: append([]string(nil), reviewer.CodeExecRequiredImports...),
		Prompts:                 reviewer.Prompts,
		Skills:                  append([]string(nil), reviewer.Skills...),
		AllowedTools:            append([]string(nil), reviewer.AllowedTools...),
		Heartbeat: profile.HeartbeatConfig{
			Enabled: heartbeatEnabled,
			Jobs:    append([]profile.HeartbeatJob(nil), reviewer.Heartbeat.Jobs...),
		},
	}
}
