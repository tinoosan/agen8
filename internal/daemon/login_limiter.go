package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/agen8/internal/rpc"
)

const (
	loginFailureLimit       = 5
	loginFailureWindow      = 5 * time.Minute
	loginLimiterMaxAccounts = 4096
	loginLimiterOverflowKey = "overflow"
)

type loginAttempt struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

// loginAttemptLimiter bounds online password guessing per account without
// retaining email addresses in memory. Successful login clears prior failures.
type loginAttemptLimiter struct {
	mu        sync.Mutex
	attempts  map[string]loginAttempt
	now       func() time.Time
	lastPrune time.Time
}

func newLoginAttemptLimiter() *loginAttemptLimiter {
	return &loginAttemptLimiter{
		attempts: make(map[string]loginAttempt),
		now:      time.Now,
	}
}

func (l *loginAttemptLimiter) Allow(key string) (time.Duration, bool) {
	if l == nil || key == "" {
		return 0, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now().UTC()
	l.pruneExpired(now)
	key = l.boundedKey(key)
	attempt, ok := l.attempts[key]
	if !ok {
		return 0, true
	}
	if !attempt.blockedUntil.IsZero() && now.Before(attempt.blockedUntil) {
		return attempt.blockedUntil.Sub(now), false
	}
	if now.Sub(attempt.windowStart) >= loginFailureWindow {
		delete(l.attempts, key)
	}
	return 0, true
}

func (l *loginAttemptLimiter) RecordFailure(key string) {
	if l == nil || key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now().UTC()
	l.pruneExpired(now)
	key = l.boundedKey(key)
	attempt := l.attempts[key]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= loginFailureWindow {
		attempt = loginAttempt{windowStart: now}
	}
	attempt.failures++
	if attempt.failures >= loginFailureLimit {
		attempt.blockedUntil = now.Add(loginFailureWindow)
	}
	l.attempts[key] = attempt
}

func (l *loginAttemptLimiter) Reset(key string) {
	if l == nil || key == "" {
		return
	}
	l.mu.Lock()
	key = l.boundedKey(key)
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (l *loginAttemptLimiter) boundedKey(key string) string {
	if _, exists := l.attempts[key]; exists || len(l.attempts) < loginLimiterMaxAccounts {
		return key
	}
	return loginLimiterOverflowKey
}

func (l *loginAttemptLimiter) pruneExpired(now time.Time) {
	if !l.lastPrune.IsZero() && now.Sub(l.lastPrune) < time.Minute {
		return
	}
	for key, attempt := range l.attempts {
		if !attempt.blockedUntil.IsZero() && now.Before(attempt.blockedUntil) {
			continue
		}
		if now.Sub(attempt.windowStart) >= loginFailureWindow {
			delete(l.attempts, key)
		}
	}
	l.lastPrune = now
}

func loginAttemptKey(body []byte) string {
	var request struct {
		Params struct {
			Email string `json:"email"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return ""
	}
	email := strings.ToLower(strings.TrimSpace(request.Params.Email))
	if email == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(email))
	return hex.EncodeToString(digest[:])
}

func rpcResponseSucceeded(raw []byte) bool {
	var response rpc.Response
	return json.Unmarshal(raw, &response) == nil && response.Error == nil
}
