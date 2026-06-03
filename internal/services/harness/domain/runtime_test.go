package domain_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

func TestNormalizeRuntimeKind(t *testing.T) {
	assert.Equal(t, "codex", domain.NormalizeRuntimeKind("  CoDeX "))
	assert.Equal(t, "claude-cli", domain.NormalizeRuntimeKind("Claude-CLI"))
	assert.Equal(t, "", domain.NormalizeRuntimeKind("  "))
}

func TestWriteJSONLine_NilWriter(t *testing.T) {
	err := domain.WriteJSONLine(nil, map[string]string{"ok": "true"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writer is nil")
}

func TestWriteJSONLine_Valid(t *testing.T) {
	var buf bytes.Buffer
	err := domain.WriteJSONLine(&buf, map[string]string{"ok": "true"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"ok":"true"`)
}

func TestParseNDJSON_SkipsZeroTypeEvent(t *testing.T) {
	stream := []byte("{\"type\":\"skip\"}\n{\"type\":\"emit\"}\n")
	events, err := domain.ParseNDJSON(stream, func(line []byte, lineNo int) (domain.Event, error) {
		switch string(line) {
		case `{"type":"skip"}`:
			return domain.Event{}, nil
		case `{"type":"emit"}`:
			return domain.Event{Type: domain.EventText, Text: "ok"}, nil
		default:
			return domain.Event{}, fmt.Errorf("unexpected line %d", lineNo)
		}
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, domain.EventText, events[0].Type)
	assert.Equal(t, "ok", events[0].Text)
}

func TestParseNDJSON_AllowsLargeLines(t *testing.T) {
	large := strings.Repeat("x", 128*1024)
	stream := []byte(large + "\n")
	events, err := domain.ParseNDJSON(stream, func(line []byte, lineNo int) (domain.Event, error) {
		require.Equal(t, 1, lineNo)
		require.Equal(t, len(large), len(line))
		return domain.Event{Type: domain.EventText, Text: "ok"}, nil
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
}
