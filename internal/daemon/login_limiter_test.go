package daemon

import (
	"fmt"
	"testing"
	"time"
)

func TestLoginAttemptLimiterExpiresAndBoundsAccountBuckets(t *testing.T) {
	limiter := newLoginAttemptLimiter()
	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	for index := 0; index < loginLimiterMaxAccounts+100; index++ {
		limiter.RecordFailure(fmt.Sprintf("account-%d", index))
	}
	if got := len(limiter.attempts); got > loginLimiterMaxAccounts+1 {
		t.Fatalf("attempt buckets=%d want at most %d", got, loginLimiterMaxAccounts+1)
	}

	now = now.Add(loginFailureWindow + time.Minute)
	limiter.RecordFailure("current-account")
	if got := len(limiter.attempts); got != 1 {
		t.Fatalf("attempt buckets after expiry=%d want 1", got)
	}
}
