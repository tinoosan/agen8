package rpcscope

import (
	"errors"
	"strings"
)

var ErrScopeUnavailable = errors.New("rpc scope unavailable")

func IsScopeUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrScopeUnavailable) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "thread not found") ||
		strings.Contains(msg, "run scope is unavailable for thread") ||
		strings.Contains(msg, "scope unavailable") ||
		strings.Contains(msg, "scope is unavailable")
}

func IsNonRetryableScopeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "permission") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "unknown method")
}
