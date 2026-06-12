package caller

import (
	"context"
	"strings"
	"testing"
)

func TestContextResolverReturnsStampedCaller(t *testing.T) {
	ctx := ContextWithCaller(context.Background(), Caller{
		UserID:    " user-1 ",
		MemberID:  " member-1 ",
		ProjectID: " project-1 ",
		Role:      " admin ",
	})

	caller, err := (ContextResolver{}).ResolveCaller(ctx)
	if err != nil {
		t.Fatalf("ResolveCaller: %v", err)
	}
	if caller.UserID != "user-1" {
		t.Fatalf("UserID = %q, want user-1", caller.UserID)
	}
	if caller.MemberID != "member-1" {
		t.Fatalf("MemberID = %q, want member-1", caller.MemberID)
	}
	if string(caller.ProjectID) != "project-1" {
		t.Fatalf("ProjectID = %q, want project-1", caller.ProjectID)
	}
	if caller.Role != "admin" {
		t.Fatalf("Role = %q, want admin", caller.Role)
	}
}

func TestContextResolverAllowsUserOnlyCaller(t *testing.T) {
	ctx := ContextWithCaller(context.Background(), Caller{UserID: "user-1"})

	caller, err := (ContextResolver{}).ResolveCaller(ctx)
	if err != nil {
		t.Fatalf("ResolveCaller: %v", err)
	}
	if caller.UserID != "user-1" || caller.MemberID != "" {
		t.Fatalf("caller = %#v, want user-only caller", caller)
	}
}

func TestContextResolverAllowsMemberOnlyCaller(t *testing.T) {
	ctx := ContextWithCaller(context.Background(), Caller{MemberID: "member-1"})

	caller, err := (ContextResolver{}).ResolveCaller(ctx)
	if err != nil {
		t.Fatalf("ResolveCaller: %v", err)
	}
	if caller.MemberID != "member-1" || caller.UserID != "" {
		t.Fatalf("caller = %#v, want member-only caller", caller)
	}
}

func TestContextResolverRejectsMissingCaller(t *testing.T) {
	_, err := (ContextResolver{}).ResolveCaller(context.Background())
	if err == nil || !strings.Contains(err.Error(), "caller is required") {
		t.Fatalf("ResolveCaller error = %v, want missing caller error", err)
	}
}

func TestContextResolverRejectsEmptyCaller(t *testing.T) {
	ctx := ContextWithCaller(context.Background(), Caller{})

	_, err := (ContextResolver{}).ResolveCaller(ctx)
	if err == nil || !strings.Contains(err.Error(), "caller user id or member id is required") {
		t.Fatalf("ResolveCaller error = %v, want empty caller error", err)
	}
}
