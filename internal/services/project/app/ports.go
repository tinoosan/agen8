package app

import (
	"context"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type SpaceLoader interface {
	Get(ctx context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error)
	List(ctx context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error)
	ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error)
}
