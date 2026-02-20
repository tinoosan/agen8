package rpcscope

import (
	"errors"
	"testing"

	"github.com/tinoosan/agen8/pkg/protocol"
)

func TestValidateInvariantsTaskListRequiresScope(t *testing.T) {
	err := validateInvariants(protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID("sess-1"),
		View:     "inbox",
	})
	if err == nil {
		t.Fatalf("expected invariant error")
	}
	if !errors.Is(err, ErrScopeUnavailable) {
		t.Fatalf("expected ErrScopeUnavailable, got %v", err)
	}
}

func TestValidateInvariantsTaskListAcceptsTeamScope(t *testing.T) {
	err := validateInvariants(protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID("sess-1"),
		TeamID:   "team-1",
		View:     "inbox",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInvariantsSessionActionRequiresThread(t *testing.T) {
	err := validateInvariants(protocol.MethodSessionResume, protocol.SessionResumeParams{SessionID: "sess-1"})
	if err == nil {
		t.Fatalf("expected invariant error")
	}
	if !errors.Is(err, ErrScopeUnavailable) {
		t.Fatalf("expected ErrScopeUnavailable, got %v", err)
	}
}

func TestIsScopeUnavailableMatchesThreadErrors(t *testing.T) {
	err := errors.New("rpc session.pause: protocol error -32002: thread not found")
	if !IsScopeUnavailable(err) {
		t.Fatalf("expected true for thread-not-found message")
	}
}
