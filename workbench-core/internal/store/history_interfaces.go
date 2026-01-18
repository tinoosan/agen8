package store

import "context"

// HistoryAppender is used by sinks to record history.
type HistoryAppender interface {
	AppendLine(ctx context.Context, line []byte) error
}

// HistoryReader is used by VFS/context to read history.
type HistoryReader interface {
	ReadAll(ctx context.Context) ([]byte, error)
	LinesSince(ctx context.Context, cursor HistoryCursor, opts HistorySinceOptions) (HistoryBatch, error)
	LinesLatest(ctx context.Context, opts HistoryLatestOptions) (HistoryBatch, error)
}

