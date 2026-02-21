package state

import (
	"errors"
	"testing"

	pkgstore "github.com/tinoosan/agen8/pkg/store"
)

func TestErrTaskNotFound_IsErrNotFound(t *testing.T) {
	if !errors.Is(ErrTaskNotFound, pkgstore.ErrNotFound) {
		t.Fatalf("expected errors.Is(ErrTaskNotFound, pkgstore.ErrNotFound) to be true")
	}
}
