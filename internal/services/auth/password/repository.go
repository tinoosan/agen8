package password

import (
	"context"

	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	Get(ctx context.Context, userID user.ID) (Credential, error)
}

type Writer interface {
	Save(ctx context.Context, credential Credential) error
	Delete(ctx context.Context, userID user.ID) error
}
