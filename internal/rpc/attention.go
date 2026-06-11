package rpc

import (
	"context"
	"fmt"

	attentionapp "github.com/tinoosan/agen8/internal/services/attention"
)

const MethodAttentionList = "attention.list"

type attentionListParams struct {
	ProjectID string `json:"projectId"`
}

type attentionListResult struct {
	Entries []attentionapp.Entry `json:"entries"`
}

// RegisterAttention wires the attention RPC surface. Read-only: the dashboard
// lists which harness sessions are waiting on the human. Mutation happens only
// through the daemon's hook-ingest endpoint, never via RPC.
func RegisterAttention(reg *Registry, attentionSvc *attentionapp.Service) error {
	if attentionSvc == nil {
		return fmt.Errorf("attention service is required")
	}
	return AddBoundHandler(reg, MethodAttentionList, false, func(ctx context.Context, params attentionListParams) (attentionListResult, error) {
		if _, err := RequireIdentity(ctx); err != nil {
			return attentionListResult{}, err
		}
		entries := attentionSvc.List(ctx, params.ProjectID)
		if entries == nil {
			entries = []attentionapp.Entry{}
		}
		return attentionListResult{Entries: entries}, nil
	})
}
