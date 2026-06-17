package mcp

import (
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The per-call session re-resolve runs harness detection on the body that
// toolCallRPCBody rebuilds — not the original wire body. Codex self-identifies
// ONLY through _meta (x-codex-turn-metadata / sessionId), so if toolCallRPCBody
// drops _meta, a Codex session resolves to harness "unknown" while Claude (which
// stamps arguments.session_id) survives. This pins that _meta rides the rebuilt
// body, keeping detection consistent across both resolve passes.
func TestToolCallRPCBodyPreservesMetaForHarnessDetection(t *testing.T) {
	cases := []struct {
		name string
		meta sdk.Meta
		args string
		want string
	}{
		{
			name: "codex via x-codex-turn-metadata",
			meta: sdk.Meta{"x-codex-turn-metadata": map[string]any{"session_id": "7c2f1e9a-3b4d-7c8e-9f01-2a3b4c5d6e7f"}},
			args: `{"action":"register"}`,
			want: "codex",
		},
		{
			name: "codex via _meta camelCase sessionId",
			meta: sdk.Meta{"sessionId": "0199d68d-14ef-70c0-bf1e-4b001a0992c1"},
			args: `{"action":"register"}`,
			want: "codex",
		},
		{
			name: "claude-code via arguments.session_id, no _meta",
			meta: nil,
			args: `{"action":"register","session_id":"1af9287d-2b8b-48a7-8e3c-ec2e4c8d8746"}`,
			want: "claude-code",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{
				Meta:      tc.meta,
				Name:      "project",
				Arguments: json.RawMessage(tc.args),
			}}
			body := toolCallRPCBody(req)
			refs := SessionRequestContextFromJSONRPCBody(body)
			nativeRef := refs.SessionID
			if nativeRef == "" {
				nativeRef = refs.ThreadID
			}
			if got := HarnessFromJSONRPCBody(body, nativeRef); got != tc.want {
				t.Fatalf("harness=%q want %q (body=%s)", got, tc.want, string(body))
			}
		})
	}
}
