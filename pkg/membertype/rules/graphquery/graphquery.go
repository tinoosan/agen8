package graphquery

import "github.com/tinoosan/agen8-mcp-server/pkg/membertype"

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "graph_query_autonomy",
		Order:     320,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeLoneCoordinator},
		Build: func(membertype.PromptContext) string {
			return membertype.JoinRuleLines([]string{
				"- GRAPH CONTEXT LOOP (autonomous execution):",
				`  1. Discover first: graph_query(action="search", node_type="all", query=...) to find existing mission/key_result/task/decision/escalation nodes before creating new ones.`,
				`  2. Inspect before acting: graph_query(action="node", node_id=..., depth=2) on any node you will mutate or depend on. node_type is optional when the ID prefix is typed (dec-, task-, mis-, kr-, esc-, oa-).`,
				"  3. Never guess IDs or relationships. If refs are unclear, search and inspect again before proceeding.",
				`  4. Persist relationships: graph_query(action="link", source_id=..., target_id=..., edge_type=..., rationale=...) whenever work creates or clarifies a mission/KR/task/decision/escalation linkage. source_type and target_type are optional when IDs have typed prefixes; otherwise include them explicitly.`,
				"  5. Verify after linking: re-run graph_query(action=\"node\") on the focal node so follow-up actions use confirmed graph state.",
				`  6. Correct bad graph edges explicitly with graph_query(action="unlink", edge_id=...) when an edge id is available. Otherwise use source_id, target_id, and edge_type; include source_type or target_type only when an endpoint ID prefix is not typed.`,
			})
		},
	})
}
