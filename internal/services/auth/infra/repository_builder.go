package infra

import (
	"fmt"

	"github.com/tinoosan/agen8/internal/services/auth/apikey"
	"github.com/tinoosan/agen8/internal/services/auth/linktoken"
	"github.com/tinoosan/agen8/internal/services/auth/password"
	"github.com/tinoosan/agen8/internal/services/auth/session"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type Repositories struct {
	Passwords  password.Repository
	Sessions   session.Repository
	APIKeys    apikey.Repository
	LinkTokens linktoken.Repository
}

func NewRepositories(handle *storagedb.Handle) (Repositories, error) {
	if handle == nil {
		return Repositories{}, fmt.Errorf("storage handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return Repositories{}, fmt.Errorf("auth repositories require SQLite storage")
	}
	return newSQLiteRepositories(handle)
}
