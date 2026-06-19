package rpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tinoosan/agen8/internal/services/lastseen"
)

const (
	MethodLastSeenGet      = "lastseen.get"
	MethodLastSeenMarkSeen = "lastseen.markSeen"
)

type lastSeenGetParams struct {
	ProjectID string `json:"projectId"`
}

type lastSeenGetResult struct {
	// SeenAt is the ISO 8601 UTC timestamp of the last-seen marker, or "" when
	// no marker exists (the user has never visited this project dashboard).
	SeenAt string `json:"seenAt"`
}

type lastSeenMarkSeenParams struct {
	ProjectID string `json:"projectId"`
}

type lastSeenMarkSeenResult struct {
	// SeenAt is the new marker timestamp.
	SeenAt string `json:"seenAt"`
}

// RegisterLastSeen wires the last-seen RPC surface.
//
//	lastseen.get      — returns the current last-seen marker for the caller
//	lastseen.markSeen — sets the marker to now and returns the new value
//
// Both methods require an authenticated identity; the userID comes from the
// request context (RequireIdentity), not from the params, so the caller cannot
// spoof another user's marker.
func RegisterLastSeen(reg *Registry, store *lastseen.Store) error {
	if store == nil {
		return fmt.Errorf("lastseen: store is required")
	}
	if err := AddBoundHandler(reg, MethodLastSeenGet, false, func(ctx context.Context, params lastSeenGetParams) (lastSeenGetResult, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			return lastSeenGetResult{}, err
		}
		t, err := store.Get(ctx, identity.UserID, params.ProjectID)
		if errors.Is(err, lastseen.ErrNotFound) {
			return lastSeenGetResult{SeenAt: ""}, nil
		}
		if err != nil {
			return lastSeenGetResult{}, err
		}
		return lastSeenGetResult{SeenAt: t.UTC().Format(time.RFC3339Nano)}, nil
	}); err != nil {
		return err
	}

	return AddBoundHandler(reg, MethodLastSeenMarkSeen, false, func(ctx context.Context, params lastSeenMarkSeenParams) (lastSeenMarkSeenResult, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			return lastSeenMarkSeenResult{}, err
		}
		now := time.Now().UTC()
		if err := store.MarkSeen(ctx, identity.UserID, params.ProjectID, now); err != nil {
			return lastSeenMarkSeenResult{}, err
		}
		return lastSeenMarkSeenResult{SeenAt: now.Format(time.RFC3339Nano)}, nil
	})
}
