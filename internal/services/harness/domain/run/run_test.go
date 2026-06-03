package run_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
)

var testNow = time.Date(2026, 5, 30, 16, 0, 0, 0, time.UTC)

func TestStartValidatesRequiredFields(t *testing.T) {
	_, err := run.Start(validStartParams())
	require.NoError(t, err)

	params := validStartParams()
	params.SessionID = ""
	_, err = run.Start(params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session id is required")
}

func TestRequestStopIsIdempotent(t *testing.T) {
	r := validRun(t)

	require.NoError(t, r.RequestStop("user-1", testNow.Add(time.Minute)))
	require.Equal(t, run.StatusStopRequested, r.Status)
	require.NotNil(t, r.StopRequestedAt)

	require.NoError(t, r.RequestStop("user-2", testNow.Add(2*time.Minute)))
	assert.Equal(t, "user-1", r.StopRequestedBy)
	assert.Equal(t, testNow.Add(time.Minute), *r.StopRequestedAt)
}

func TestTerminalTransitions(t *testing.T) {
	r := validRun(t)

	require.NoError(t, r.RequestStop("user-1", testNow.Add(time.Minute)))
	require.NoError(t, r.MarkCanceled(testNow.Add(2*time.Minute)))
	assert.Equal(t, run.StatusCanceled, r.Status)
	require.NotNil(t, r.CompletedAt)
	assert.Empty(t, r.Error)

	err := r.RequestStop("user-1", testNow.Add(3*time.Minute))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status is \"canceled\"")
}

func TestFailedRequiresError(t *testing.T) {
	r := validRun(t)

	err := r.MarkFailed("", testNow.Add(time.Minute))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error is required")

	require.NoError(t, r.MarkFailed("runtime failed", testNow.Add(time.Minute)))
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, "runtime failed", r.Error)
}

func validRun(t *testing.T) run.Run {
	t.Helper()
	r, err := run.Start(validStartParams())
	require.NoError(t, err)
	return r
}

func validStartParams() run.StartParams {
	return run.StartParams{
		ID:               "run-1",
		ProjectID:        "project-1",
		SpaceID:          "space-1",
		ChannelID:        "channel-1",
		MemberID:         "member-1",
		SessionID:        "session-1",
		HarnessKind:      "codex",
		NativeSessionRef: "thread-1",
		TurnID:           "turn-1",
		StartedAt:        testNow,
	}
}
