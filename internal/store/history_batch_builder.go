package store

import (
	"bytes"

	"github.com/tinoosan/agen8/pkg/bytesutil"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
)

type historyRecord struct {
	line []byte
	size int
}

type historySinceResult struct {
	batch    pkgstore.HistoryBatch
	consumed int
}

func buildHistorySinceBatch(records []historyRecord, maxBytes, limit int) historySinceResult {
	var (
		lines     [][]byte
		bytesRead int
		linesTotal int
		truncated bool
		consumed  int
	)

	for _, record := range records {
		if bytesRead >= maxBytes {
			truncated = true
			break
		}
		linesTotal++
		bytesRead += record.size
		consumed++
		trimmed := bytesutil.TrimRightNewlines(record.line)
		if len(trimmed) > 0 {
			lines = append(lines, append([]byte(nil), trimmed...))
		}
		if len(lines) >= limit {
			truncated = true
			break
		}
	}

	return historySinceResult{
		consumed: consumed,
		batch: pkgstore.HistoryBatch{
			Lines:          lines,
			BytesRead:      bytesRead,
			LinesTotal:     linesTotal,
			Returned:       len(lines),
			ReturnedCapped: len(lines) >= limit,
			Truncated:      truncated,
		},
	}
}

func buildHistoryLatestBatch(records []historyRecord, maxBytes, limit int, inputTruncated bool) pkgstore.HistoryBatch {
	selected := make([][]byte, 0, minInt(len(records), limit))
	bytesRead := 0
	linesTotal := 0
	truncated := inputTruncated

	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		linesTotal++
		if len(selected) > 0 && bytesRead+record.size > maxBytes {
			truncated = true
			break
		}
		bytesRead += record.size
		trimmed := bytesutil.TrimRightNewlines(record.line)
		if len(bytes.TrimSpace(trimmed)) == 0 {
			continue
		}
		selected = append(selected, append([]byte(nil), trimmed...))
		if len(selected) >= limit {
			truncated = true
			break
		}
	}

	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}

	return pkgstore.HistoryBatch{
		Lines:          selected,
		BytesRead:      bytesRead,
		LinesTotal:     linesTotal,
		Returned:       len(selected),
		ReturnedCapped: len(selected) >= limit,
		Truncated:      truncated,
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
