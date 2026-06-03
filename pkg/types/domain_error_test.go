package types

import (
	"errors"
	"testing"
)

func TestDomainErrorError(t *testing.T) {
	t.Run("message only", func(t *testing.T) {
		err := &DomainError{Code: "task_invalid", Message: "task is invalid"}
		if got := err.Error(); got != "task is invalid" {
			t.Fatalf("Error() = %q, want %q", got, "task is invalid")
		}
	})

	t.Run("message and cause", func(t *testing.T) {
		cause := errors.New("root cause")
		err := &DomainError{Code: "task_invalid", Message: "task is invalid", Cause: cause}
		if got := err.Error(); got != "task is invalid: root cause" {
			t.Fatalf("Error() = %q, want %q", got, "task is invalid: root cause")
		}
	})

	t.Run("cause only", func(t *testing.T) {
		cause := errors.New("root cause")
		err := &DomainError{Code: "task_invalid", Cause: cause}
		if got := err.Error(); got != "root cause" {
			t.Fatalf("Error() = %q, want %q", got, "root cause")
		}
	})
}

func TestDomainErrorUnwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &DomainError{Code: "task_invalid", Message: "task is invalid", Cause: cause}

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should match wrapped cause")
	}

	var target *DomainError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should match DomainError")
	}
	if target != err {
		t.Fatal("errors.As should return original DomainError")
	}
}
