package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

func TestWrapSpace_EmptyStatusNormalisesToOpen(t *testing.T) {
	s := domain.WrapSpace(domain.SpaceRecord{})
	assert.Equal(t, domain.SpaceStatusOpen, s.Status())
}

func TestClose_OpenToClosed(t *testing.T) {
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	s := domain.WrapSpace(domain.SpaceRecord{Status: domain.SpaceStatusOpen})

	next, err := s.Close(now)
	require.NoError(t, err)
	assert.Equal(t, domain.SpaceStatusClosed, next.Status())
	assert.Equal(t, now, next.Inner().UpdatedAt)
}

func TestClose_AlreadyClosed_Idempotent(t *testing.T) {
	s := domain.WrapSpace(domain.SpaceRecord{Status: domain.SpaceStatusClosed})
	next, err := s.Close(time.Now())
	require.NoError(t, err)
	assert.Equal(t, domain.SpaceStatusClosed, next.Status())
}

func TestReopen_ClosedToOpen(t *testing.T) {
	now := time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC)
	s := domain.WrapSpace(domain.SpaceRecord{Status: domain.SpaceStatusClosed})

	next, err := s.Reopen(now)
	require.NoError(t, err)
	assert.Equal(t, domain.SpaceStatusOpen, next.Status())
	assert.Equal(t, now, next.Inner().UpdatedAt)
}

func TestReopen_AlreadyOpen_Idempotent(t *testing.T) {
	s := domain.WrapSpace(domain.SpaceRecord{Status: domain.SpaceStatusOpen})
	next, err := s.Reopen(time.Now())
	require.NoError(t, err)
	assert.Equal(t, domain.SpaceStatusOpen, next.Status())
}

func TestTransition_UnknownStatus_ReturnsError(t *testing.T) {
	s := domain.WrapSpace(domain.SpaceRecord{Status: "bogus"})
	_, err := s.Close(time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no transitions defined")
}
