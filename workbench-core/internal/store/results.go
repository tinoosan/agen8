package store

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
)

// InMemoryResultsStore is the simplest ResultsStore implementation.
//
// It is fast and deterministic, making it ideal for:
// - demos
// - tests
// - early iterations before choosing a durable store
//
// It is NOT durable; everything is lost when the process exits.
type InMemoryResultsStore struct {
	mu    sync.RWMutex
	calls map[string]*inMemoryCall
}

type inMemoryCall struct {
	responseJSON []byte
	responseTime time.Time
	artifacts    map[string]inMemoryArtifact
}

type inMemoryArtifact struct {
	b         []byte
	mediaType string
	modTime   time.Time
}

// NewInMemoryResultsStore creates an empty in-memory ResultsStore.
func NewInMemoryResultsStore() *InMemoryResultsStore {
	return &InMemoryResultsStore{
		calls: make(map[string]*inMemoryCall),
	}
}

func (s *InMemoryResultsStore) PutCall(callID string, responseJSON []byte) error {
	if s == nil {
		return fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return err
	}
	if responseJSON == nil {
		return fmt.Errorf("responseJSON is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.calls[callID]
	if c == nil {
		c = &inMemoryCall{artifacts: make(map[string]inMemoryArtifact)}
		s.calls[callID] = c
	}
	c.responseJSON = append([]byte(nil), responseJSON...)
	c.responseTime = time.Now().UTC()
	return nil
}

func (s *InMemoryResultsStore) GetCallResponseJSON(callID string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.calls[callID]
	if c == nil || c.responseJSON == nil {
		return nil, pkgstore.ErrResultsNotFound
	}
	return append([]byte(nil), c.responseJSON...), nil
}

func (s *InMemoryResultsStore) ListCallIDs() ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]string, 0, len(s.calls))
	for id := range s.calls {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (s *InMemoryResultsStore) PutArtifact(callID, artifactPath, mediaType string, content []byte) error {
	if s == nil {
		return fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return err
	}
	artifactPath = strings.TrimSpace(artifactPath)
	if err := validate.NonEmpty("artifactPath", artifactPath); err != nil {
		return err
	}
	if err := validate.NonEmpty("mediaType", mediaType); err != nil {
		return err
	}
	if content == nil {
		return fmt.Errorf("content is required")
	}
	clean, err := vfsutil.CleanRelPath(artifactPath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.calls[callID]
	if c == nil {
		c = &inMemoryCall{artifacts: make(map[string]inMemoryArtifact)}
		s.calls[callID] = c
	}
	if c.artifacts == nil {
		c.artifacts = make(map[string]inMemoryArtifact)
	}
	c.artifacts[clean] = inMemoryArtifact{
		b:         append([]byte(nil), content...),
		mediaType: mediaType,
		modTime:   time.Now().UTC(),
	}
	return nil
}

func (s *InMemoryResultsStore) GetArtifact(callID, artifactPath string) ([]byte, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return nil, "", err
	}
	artifactPath = strings.TrimSpace(artifactPath)
	if err := validate.NonEmpty("artifactPath", artifactPath); err != nil {
		return nil, "", err
	}
	clean, err := vfsutil.CleanRelPath(artifactPath)
	if err != nil {
		return nil, "", err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.calls[callID]
	if c == nil || c.artifacts == nil {
		return nil, "", pkgstore.ErrResultsNotFound
	}
	a, ok := c.artifacts[clean]
	if !ok {
		return nil, "", pkgstore.ErrResultsNotFound
	}
	return append([]byte(nil), a.b...), a.mediaType, nil
}

func (s *InMemoryResultsStore) ListArtifacts(callID string) ([]pkgstore.ArtifactMeta, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.calls[callID]
	if c == nil {
		return nil, pkgstore.ErrResultsNotFound
	}

	out := make([]pkgstore.ArtifactMeta, 0, len(c.artifacts))
	for p, a := range c.artifacts {
		out = append(out, pkgstore.ArtifactMeta{
			Path:      p,
			MediaType: a.mediaType,
			Size:      int64(len(a.b)),
			ModTime:   a.modTime,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
