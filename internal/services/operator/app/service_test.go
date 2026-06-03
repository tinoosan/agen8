package app

import (
	"context"
	"testing"
)

func TestResolveRefs_AllowsStandaloneOperatorItems(t *testing.T) {
	service := &Service{}
	refs, err := service.resolveRefs(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("resolveRefs error: %v", err)
	}
	if refs.TaskRef != "" || refs.KeyResultRef != "" || refs.MissionRef != "" {
		t.Fatalf("unexpected refs: %+v", refs)
	}
}
