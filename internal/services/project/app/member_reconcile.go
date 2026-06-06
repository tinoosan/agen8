package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

// ReconcileDuplicateMembers retires the duplicate member rows left behind by the old
// harness-label fork bug. Member identity used to fold in the harness label, so one
// real session that registered under two labels (for example "claude" then
// "claude-cli") produced two member rows sharing the same (user, project, native
// session ref). Resolve now collapses such a fork to the earliest-registered member on
// every read, but the loser rows stay active in the table and inflate the roster.
//
// This pass groups active members by (user, project, native session ref), keeps the
// earliest-registered member of each group, and marks the rest removed. It picks the
// survivor with the same rule resolve uses (preferRegisteredMember), so the row that
// lives through reconciliation is exactly the one reads already resolve to.
//
// It is a system maintenance task: it runs without a caller and emits no lifecycle
// events. It is meant to run at startup and is safe to run every time because it is a
// no-op once each group has a single active member. It only ever collapses members that
// share a project, so it never merges across a project boundary — the cross-project
// ambiguity resolve refuses to guess is left untouched here too.
func (s *Service) ReconcileDuplicateMembers(ctx context.Context) (int, error) {
	// No Limit: we need every active member to find the full fork groups, not a page.
	actives, err := s.members.List(ctx, member.Filter{LifecycleState: member.LifecycleActive})
	if err != nil {
		return 0, fmt.Errorf("reconcile duplicate members: list active members: %w", err)
	}

	groups := map[string][]member.Record{}
	order := make([]string, 0)
	for _, m := range actives {
		nativeRef := strings.TrimSpace(m.NativeSessionRef)
		if nativeRef == "" {
			// No native session ref means this is not a harness session, so it can't be
			// a session fork. Leave it alone.
			continue
		}
		key := strings.TrimSpace(m.UserID) + "\x00" + strings.TrimSpace(m.ProjectID) + "\x00" + nativeRef
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], m)
	}

	now := s.clock.Now()
	retired := 0
	for _, key := range order {
		candidates := groups[key]
		if len(candidates) < 2 {
			continue
		}
		winner := candidates[0]
		for _, candidate := range candidates[1:] {
			if preferRegisteredMember(candidate, winner) {
				winner = candidate
			}
		}
		for _, m := range candidates {
			if m.ID == winner.ID {
				continue
			}
			// Every candidate came from the active list, so Remove cannot hit its
			// already-removed guard. If it ever does, that is a real invariant break and
			// we surface it rather than swallow it.
			removed, err := member.WrapMember(m).Remove(now)
			if err != nil {
				return retired, fmt.Errorf("reconcile duplicate members: retire %s: %w", m.ID, err)
			}
			if err := s.members.Update(ctx, removed.Inner()); err != nil {
				return retired, fmt.Errorf("reconcile duplicate members: persist retirement of %s: %w", m.ID, err)
			}
			s.logger.InfoContext(ctx, "retired duplicate session-fork member",
				"project_id", m.ProjectID,
				"native_session_ref", strings.TrimSpace(m.NativeSessionRef),
				"kept_member_id", string(winner.ID),
				"retired_member_id", string(m.ID),
			)
			retired++
		}
	}
	return retired, nil
}
