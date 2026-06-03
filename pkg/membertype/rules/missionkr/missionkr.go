package missionkr

import "github.com/tinoosan/agen8-mcp-server/pkg/membertype"

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "mission_and_kr",
		Order:     300,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator},
		Lines: []string{
			"- MISSION/KR SETUP ORDER (follow in strict order when you create a mission to track work):",
			`  1. mission(action="create", ...) creates a DRAFT mission. A draft mission does not track progress and is invisible to anyone reviewing objectives — it is scaffolding, not tracking.`,
			`  2. For each measurable objective, call mission(action="kr_create", mission_id=..., ...).`,
			`  3. For each KR, call mission(action="kr_set_space", key_result_id=..., space_id=...) to assign the accountable owning space. Activation will reject the mission if any non-dropped KR has no owning space, and the target space must be in "open" status.`,
			`  4. mission(action="update", mission_id=..., status="active") — only now does the mission start tracking. Never delegate measurable work against a draft mission.`,
			"  5. Delegate measurable work with task(action=\"create\", key_result_ref=<kr_id>, ...). Key result refs connect mission intent to execution and should be set when the task is created.",
			"- KRs are setup, not cleanup. Create them before delegating measurable work. If you realize mid-cycle that a KR is missing, add the KR now so future tasks link to it, and note the unlinked in-flight tasks with decision(action=\"log\") so the operator can see the gap.",
			"- Do not create a placeholder mission for work that has no measurable objective. Work that leaves no meaningful output stays unlinked — that is correct. Apply the same test used for KRs: if the output would be useful to reference in a future invocation, it belongs under a mission.",
			"- The strategy map is institutional memory. Every mission creates a record that future invocations can reference — past findings, decisions, KR outcomes, what worked and what didn't. This is why any work with trackable output belongs in a mission: the space builds on what was done before rather than starting from scratch every time.",
			`- When mission(action="list") shows an active mission that covers your current work, reuse its KRs. Do not create a parallel mission.`,
		},
	})
}
