package rpcscope

import "time"

// ScopeState is the canonical resolved RPC scope for a session.
type ScopeState struct {
	SessionID       string
	ThreadID        string
	RunID           string
	TeamID          string
	Mode            string
	CoordinatorRole string
}

// RecoveryPolicy controls how call recovery behaves.
type RecoveryPolicy struct {
	MaxRetries       int
	BaseBackoff      time.Duration
	MaxBackoff       time.Duration
	RefreshOnError   bool
	RetryableErrors  []string
	RetryableMatcher func(error) bool
}

// DefaultRecoveryPolicy returns the default policy used by TUIs.
func DefaultRecoveryPolicy() RecoveryPolicy {
	return RecoveryPolicy{
		MaxRetries:      1,
		BaseBackoff:     500 * time.Millisecond,
		MaxBackoff:      8 * time.Second,
		RefreshOnError:  true,
		RetryableErrors: []string{"thread not found", "run scope is unavailable for thread", "scope is unavailable"},
	}
}
