package resources

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type fakeResource struct {
	entries       []vfs.Entry
	readData      []byte
	readErr       error
	writeErr      error
	appendErr     error
	appended      []byte
	appendCalls   int
	searchOut     []types.SearchResult
	searchErr     error
	nestedSupport bool
}

func (f *fakeResource) SupportsNestedList() bool { return f.nestedSupport }

func (f *fakeResource) List(path string) ([]vfs.Entry, error) { return f.entries, nil }

func (f *fakeResource) Read(path string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.readData, nil
}

func (f *fakeResource) Write(path string, data []byte) error { return f.writeErr }

func (f *fakeResource) Append(path string, data []byte) error {
	f.appendCalls++
	f.appended = append([]byte(nil), data...)
	return f.appendErr
}

func (f *fakeResource) Search(ctx context.Context, path string, query string, limit int) ([]types.SearchResult, error) {
	return f.searchOut, f.searchErr
}

func TestValidatingMemoryResource_DelegatesReadWriteListSearch(t *testing.T) {
	inner := &fakeResource{
		nestedSupport: true,
		entries:       []vfs.Entry{{Path: "x"}},
		readData:      []byte("a"),
		searchOut:     []types.SearchResult{{Title: "hit"}},
	}
	res := NewValidatingMemoryResource(inner)
	if !res.SupportsNestedList() {
		t.Fatalf("expected nested list support")
	}
	if _, err := res.List(""); err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := res.Read("x"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if err := res.Write("x", []byte("y")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out, err := res.Search(context.Background(), "", "q", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 search result")
	}
}

func TestValidatingMemoryResource_Append_ValidatesAndDedups(t *testing.T) {
	inner := &fakeResource{readData: []byte("07:00 | blocker | waiting on api key\n")}
	res := NewValidatingMemoryResource(inner)

	if err := res.Append("2026-02-20-memory.md", []byte("09:10 | blocker | Waiting on API key\n10:00 | handoff | leave notes for next run\n10:00 | handoff | leave notes for next run\n")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if inner.appendCalls != 1 {
		t.Fatalf("expected one append call, got %d", inner.appendCalls)
	}
	payload := string(inner.appended)
	if strings.Count(payload, "handoff") != 1 {
		t.Fatalf("expected one handoff line in payload, got %q", payload)
	}
	if strings.Contains(strings.ToLower(payload), "blocker") {
		t.Fatalf("expected duplicate blocker to be skipped, got %q", payload)
	}
}

func TestValidatingMemoryResource_Append_RejectsBadFormat(t *testing.T) {
	inner := &fakeResource{}
	res := NewValidatingMemoryResource(inner)

	err := res.Append("2026-02-20-memory.md", []byte("not valid\n"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "invalid memory line") {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.appendCalls != 0 {
		t.Fatalf("append should not be called on invalid input")
	}
}

func TestValidatingMemoryResource_Append_RejectsOutOfScopeCategory(t *testing.T) {
	inner := &fakeResource{}
	res := NewValidatingMemoryResource(inner)

	err := res.Append("2026-02-20-memory.md", []byte(fmt.Sprintf("%s | goal | ship feature\n", time.Now().Format("15:04"))))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "valid categories") {
		t.Fatalf("expected valid-categories hint, got %v", err)
	}
}
