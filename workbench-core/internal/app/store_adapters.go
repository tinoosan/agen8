package app

import (
	"context"

	internalstore "github.com/tinoosan/workbench-core/internal/store"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
)

type historyStoreAdapter struct {
	internalstore.HistoryStore
}

func (a historyStoreAdapter) AppendLine(ctx context.Context, line []byte) error {
	return a.HistoryStore.AppendLine(ctx, line)
}

func (a historyStoreAdapter) ReadAll(ctx context.Context) ([]byte, error) {
	return a.HistoryStore.ReadAll(ctx)
}

func (a historyStoreAdapter) LinesSince(ctx context.Context, cursor pkgstore.HistoryCursor, opts pkgstore.HistorySinceOptions) (pkgstore.HistoryBatch, error) {
	batch, err := a.HistoryStore.LinesSince(ctx, internalstore.HistoryCursor(cursor), internalstore.HistorySinceOptions{
		MaxBytes: opts.MaxBytes,
		Limit:    opts.Limit,
	})
	return pkgstore.HistoryBatch{
		Lines:          batch.Lines,
		CursorAfter:    pkgstore.HistoryCursor(batch.CursorAfter),
		BytesRead:      batch.BytesRead,
		LinesTotal:     batch.LinesTotal,
		Returned:       batch.Returned,
		ReturnedCapped: batch.ReturnedCapped,
		Truncated:      batch.Truncated,
	}, err
}

func (a historyStoreAdapter) LinesLatest(ctx context.Context, opts pkgstore.HistoryLatestOptions) (pkgstore.HistoryBatch, error) {
	batch, err := a.HistoryStore.LinesLatest(ctx, internalstore.HistoryLatestOptions{
		MaxBytes: opts.MaxBytes,
		Limit:    opts.Limit,
	})
	return pkgstore.HistoryBatch{
		Lines:          batch.Lines,
		CursorAfter:    pkgstore.HistoryCursor(batch.CursorAfter),
		BytesRead:      batch.BytesRead,
		LinesTotal:     batch.LinesTotal,
		Returned:       batch.Returned,
		ReturnedCapped: batch.ReturnedCapped,
		Truncated:      batch.Truncated,
	}, err
}

type traceStoreAdapter struct {
	internalstore.TraceStore
}

func (a traceStoreAdapter) EventsSince(ctx context.Context, cursor pkgstore.TraceCursor, opts pkgstore.TraceSinceOptions) (pkgstore.TraceBatch, error) {
	batch, err := a.TraceStore.EventsSince(ctx, internalstore.TraceCursor(cursor), internalstore.TraceSinceOptions{
		MaxBytes: opts.MaxBytes,
		Limit:    opts.Limit,
	})
	return pkgstore.TraceBatch{
		Events:         convertTraceEvents(batch.Events),
		CursorAfter:    pkgstore.TraceCursor(batch.CursorAfter),
		BytesRead:      batch.BytesRead,
		LinesTotal:     batch.LinesTotal,
		Parsed:         batch.Parsed,
		ParseErrors:    batch.ParseErrors,
		Returned:       batch.Returned,
		ReturnedCapped: batch.ReturnedCapped,
		Truncated:      batch.Truncated,
	}, err
}

func (a traceStoreAdapter) EventsLatest(ctx context.Context, opts pkgstore.TraceLatestOptions) (pkgstore.TraceBatch, error) {
	batch, err := a.TraceStore.EventsLatest(ctx, internalstore.TraceLatestOptions{
		MaxBytes: opts.MaxBytes,
		Limit:    opts.Limit,
	})
	return pkgstore.TraceBatch{
		Events:         convertTraceEvents(batch.Events),
		CursorAfter:    pkgstore.TraceCursor(batch.CursorAfter),
		BytesRead:      batch.BytesRead,
		LinesTotal:     batch.LinesTotal,
		Parsed:         batch.Parsed,
		ParseErrors:    batch.ParseErrors,
		Returned:       batch.Returned,
		ReturnedCapped: batch.ReturnedCapped,
		Truncated:      batch.Truncated,
	}, err
}

func convertTraceEvents(events []internalstore.TraceEvent) []pkgstore.TraceEvent {
	out := make([]pkgstore.TraceEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, pkgstore.TraceEvent{
			Timestamp: ev.Timestamp,
			Type:      ev.Type,
			Message:   ev.Message,
			Data:      ev.Data,
		})
	}
	return out
}

type resultsStoreAdapter struct {
	*internalstore.InMemoryResultsStore
}

func (a resultsStoreAdapter) PutCall(callID string, responseJSON []byte) error {
	return a.InMemoryResultsStore.PutCall(callID, responseJSON)
}

func (a resultsStoreAdapter) PutArtifact(callID, artifactPath, mediaType string, content []byte) error {
	return a.InMemoryResultsStore.PutArtifact(callID, artifactPath, mediaType, content)
}

func (a resultsStoreAdapter) GetCallResponseJSON(callID string) ([]byte, error) {
	return a.InMemoryResultsStore.GetCallResponseJSON(callID)
}

func (a resultsStoreAdapter) GetArtifact(callID, artifactPath string) ([]byte, string, error) {
	return a.InMemoryResultsStore.GetArtifact(callID, artifactPath)
}

func (a resultsStoreAdapter) ListCallIDs() ([]string, error) {
	return a.InMemoryResultsStore.ListCallIDs()
}

func (a resultsStoreAdapter) ListArtifacts(callID string) ([]pkgstore.ArtifactMeta, error) {
	arts, err := a.InMemoryResultsStore.ListArtifacts(callID)
	if err != nil {
		return nil, err
	}
	out := make([]pkgstore.ArtifactMeta, 0, len(arts))
	for _, a := range arts {
		out = append(out, pkgstore.ArtifactMeta{
			Path:      a.Path,
			MediaType: a.MediaType,
			Size:      a.Size,
			ModTime:   a.ModTime,
		})
	}
	return out, nil
}
