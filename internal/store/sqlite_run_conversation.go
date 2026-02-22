package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/tinoosan/agen8/pkg/config"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/store"
)

type SQLiteRunConversationStore struct {
	db *sql.DB
}

func NewSQLiteRunConversationStoreFromConfig(cfg config.Config) (*SQLiteRunConversationStore, error) {
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	return NewSQLiteRunConversationStore(db), nil
}

func NewSQLiteRunConversationStore(db *sql.DB) *SQLiteRunConversationStore {
	return &SQLiteRunConversationStore{db: db}
}

func (s *SQLiteRunConversationStore) LoadMessages(ctx context.Context, runID string) ([]llmtypes.LLMMessage, error) {
	row := s.db.QueryRowContext(ctx, "SELECT messages_json FROM run_conversations WHERE run_id = ?", runID)
	var data []byte
	err := row.Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var msgs []llmtypes.LLMMessage
	if len(data) > 0 {
		if err := json.Unmarshal(data, &msgs); err != nil {
			return nil, err
		}
	}
	return msgs, nil
}

func (s *SQLiteRunConversationStore) SaveMessages(ctx context.Context, runID string, msgs []llmtypes.LLMMessage) error {
	data, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO run_conversations (run_id, messages_json)
		VALUES (?, ?)
		ON CONFLICT(run_id) DO UPDATE SET messages_json = excluded.messages_json
	`, runID, data)
	return err
}

var _ store.RunConversationStore = (*SQLiteRunConversationStore)(nil)
