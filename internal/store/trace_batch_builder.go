package store

import (
	"strings"

	pkgstore "github.com/tinoosan/agen8/pkg/store"
)

type traceRecord struct {
	raw  string
	size int
}

type traceSinceResult struct {
	batch    pkgstore.TraceBatch
	consumed int
}

func buildTraceSinceBatch(records []traceRecord, maxBytes, limit int) traceSinceResult {
	var (
		events      []pkgstore.TraceEvent
		bytesRead   int
		linesTotal  int
		parsed      int
		parseErrors int
		truncated   bool
		consumed    int
	)

	for _, record := range records {
		if bytesRead >= maxBytes {
			truncated = true
			break
		}
		linesTotal++
		bytesRead += record.size
		consumed++

		event, ok := parseTraceEvent(record.raw)
		if !ok {
			parseErrors++
		} else {
			parsed++
			event.Type = strings.TrimSpace(event.Type)
			event.Message = strings.TrimSpace(event.Message)
			events = append(events, event)
		}
		if len(events) >= limit {
			truncated = true
			break
		}
	}

	return traceSinceResult{
		consumed: consumed,
		batch: pkgstore.TraceBatch{
			Events:         events,
			BytesRead:      bytesRead,
			LinesTotal:     linesTotal,
			Parsed:         parsed,
			ParseErrors:    parseErrors,
			Returned:       len(events),
			ReturnedCapped: len(events) >= limit,
			Truncated:      truncated,
		},
	}
}

func buildTraceLatestBatch(records []traceRecord, maxBytes, limit int, inputTruncated bool) pkgstore.TraceBatch {
	selected := make([]pkgstore.TraceEvent, 0, minInt(len(records), limit))
	bytesRead := 0
	linesTotal := 0
	parsed := 0
	parseErrors := 0
	truncated := inputTruncated

	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if len(selected) > 0 && bytesRead+record.size > maxBytes {
			truncated = true
			break
		}
		linesTotal++
		bytesRead += record.size
		event, ok := parseTraceEvent(record.raw)
		if !ok {
			parseErrors++
		} else {
			parsed++
			event.Type = strings.TrimSpace(event.Type)
			event.Message = strings.TrimSpace(event.Message)
			selected = append(selected, event)
		}
		if len(selected) >= limit {
			truncated = true
			break
		}
	}

	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}

	return pkgstore.TraceBatch{
		Events:         selected,
		BytesRead:      bytesRead,
		LinesTotal:     linesTotal,
		Parsed:         parsed,
		ParseErrors:    parseErrors,
		Returned:       len(selected),
		ReturnedCapped: len(selected) >= limit,
		Truncated:      truncated,
	}
}
