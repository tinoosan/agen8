package infra

import (
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type Repository interface {
	domain.Repository
	channel.Repository
}

func NewConversationRepository(handle *storagedb.Handle) (conversation.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("message conversation repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteConversationRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresConversationRepository(handle)
	default:
		return nil, fmt.Errorf("message conversation repository: unsupported storage driver %q", handle.Driver())
	}
}

func NewRepository(handle *storagedb.Handle) (Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("message repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresRepository(handle)
	default:
		return nil, fmt.Errorf("message repository: unsupported storage driver %q", handle.Driver())
	}
}
