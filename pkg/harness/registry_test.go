package harness

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

type fakeAdapter struct{ id string }

func (f fakeAdapter) ID() string { return f.id }
func (f fakeAdapter) RunTask(context.Context, TaskRequest) (TaskResult, error) {
	return TaskResult{Status: types.TaskStatusSucceeded}, nil
}

func TestRegistryRegisterLookupAndIDs(t *testing.T) {
	r, err := NewRegistry(fakeAdapter{id: "one"})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := r.Register(fakeAdapter{id: "Two"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := r.Get("two"); !ok {
		t.Fatalf("expected adapter 'two' to be registered")
	}
	ids := r.IDs()
	if len(ids) != 2 || ids[0] != "one" || ids[1] != "two" {
		t.Fatalf("IDs = %#v", ids)
	}
}

func TestRegistryRejectsDuplicateIDs(t *testing.T) {
	r, err := NewRegistry(fakeAdapter{id: "one"})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := r.Register(fakeAdapter{id: "ONE"}); err == nil {
		t.Fatalf("expected duplicate adapter registration to fail")
	}
}
