package infra

import (
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
)

func TestRepositoryInterfaces(t *testing.T) {
	var _ domain.Repository = (*SQLiteRepository)(nil)
	var _ channel.Repository = (*SQLiteRepository)(nil)
	var _ Repository = (*SQLiteRepository)(nil)
	var _ conversation.Repository = (*SQLiteConversationRepository)(nil)
	var _ conversation.Reader = (*SQLiteConversationRepository)(nil)
	var _ conversation.Writer = (*SQLiteConversationRepository)(nil)
	var _ domain.Repository = (*PostgresRepository)(nil)
	var _ channel.Repository = (*PostgresRepository)(nil)
	var _ Repository = (*PostgresRepository)(nil)
	var _ conversation.Repository = (*PostgresConversationRepository)(nil)
	var _ conversation.Reader = (*PostgresConversationRepository)(nil)
	var _ conversation.Writer = (*PostgresConversationRepository)(nil)
}
