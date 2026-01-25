package resources_test

import (
	"bytes"
	"sort"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestVirtualResultsResource_ListAndRead(t *testing.T) {
	resultsStore := newTestResultsStore()
	if err := resultsStore.PutCall("abc", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("PutCall: %v", err)
	}
	if err := resultsStore.PutArtifact("abc", "stdout.txt", "text/plain", []byte("hello\n")); err != nil {
		t.Fatalf("PutArtifact: %v", err)
	}

	res, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, res)

	entries, err := fs.List("/results")
	if err != nil {
		t.Fatalf("List /results: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Path == "/results/abc" && e.IsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected /results/abc in list, got %+v", entries)
	}

	callEntries, err := fs.List("/results/abc")
	if err != nil {
		t.Fatalf("List /results/abc: %v", err)
	}
	wantPaths := map[string]bool{
		"/results/abc/response.json": true,
		"/results/abc/stdout.txt":    true,
	}
	for _, e := range callEntries {
		delete(wantPaths, e.Path)
	}
	if len(wantPaths) != 0 {
		t.Fatalf("missing entries: %+v", wantPaths)
	}

	b, err := fs.Read("/results/abc/response.json")
	if err != nil {
		t.Fatalf("Read response.json: %v", err)
	}
	if !bytes.Equal(b, []byte(`{"ok":true}`)) {
		t.Fatalf("unexpected response bytes: %q", string(b))
	}

	ab, err := fs.Read("/results/abc/stdout.txt")
	if err != nil {
		t.Fatalf("Read artifact: %v", err)
	}
	if string(ab) != "hello\n" {
		t.Fatalf("unexpected artifact bytes: %q", string(ab))
	}

	if err := fs.Write("/results/abc/response.json", []byte("nope")); err == nil {
		t.Fatalf("expected results write to be rejected")
	}
}

type testResultsStore struct {
	calls map[string]*testCall
}

type testCall struct {
	responseJSON []byte
	artifacts    map[string]testArtifact
}

type testArtifact struct {
	data      []byte
	mediaType string
	modTime   string
}

func newTestResultsStore() *testResultsStore {
	return &testResultsStore{calls: make(map[string]*testCall)}
}

func (s *testResultsStore) PutCall(callID string, responseJSON []byte) error {
	if s.calls[callID] == nil {
		s.calls[callID] = &testCall{artifacts: make(map[string]testArtifact)}
	}
	s.calls[callID].responseJSON = append([]byte(nil), responseJSON...)
	return nil
}

func (s *testResultsStore) PutArtifact(callID, artifactPath, mediaType string, content []byte) error {
	if s.calls[callID] == nil {
		s.calls[callID] = &testCall{artifacts: make(map[string]testArtifact)}
	}
	s.calls[callID].artifacts[artifactPath] = testArtifact{
		data:      append([]byte(nil), content...),
		mediaType: mediaType,
	}
	return nil
}

func (s *testResultsStore) GetCallResponseJSON(callID string) ([]byte, error) {
	c := s.calls[callID]
	if c == nil || c.responseJSON == nil {
		return nil, store.ErrResultsNotFound
	}
	return append([]byte(nil), c.responseJSON...), nil
}

func (s *testResultsStore) ListCallIDs() ([]string, error) {
	out := make([]string, 0, len(s.calls))
	for id := range s.calls {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (s *testResultsStore) GetArtifact(callID, artifactPath string) ([]byte, string, error) {
	c := s.calls[callID]
	if c == nil {
		return nil, "", store.ErrResultsNotFound
	}
	a, ok := c.artifacts[artifactPath]
	if !ok {
		return nil, "", store.ErrResultsNotFound
	}
	return append([]byte(nil), a.data...), a.mediaType, nil
}

func (s *testResultsStore) ListArtifacts(callID string) ([]store.ArtifactMeta, error) {
	c := s.calls[callID]
	if c == nil {
		return nil, store.ErrResultsNotFound
	}
	out := make([]store.ArtifactMeta, 0, len(c.artifacts))
	for p, a := range c.artifacts {
		out = append(out, store.ArtifactMeta{
			Path:      p,
			MediaType: a.mediaType,
			Size:      int64(len(a.data)),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
