package domain

import (
	"testing"
	"time"
)

func TestTimingExpressionRequiresTimezoneForCron(t *testing.T) {
	timing := TimingExpression{Mode: TimingModeCron, Cron: "0 9 * * 1-5"}
	if err := timing.Validate(); err == nil {
		t.Fatalf("Validate() should reject cron timing without timezone")
	}
}

func TestTimingExpressionCronUsesTimezone(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	timing := TimingExpression{
		Mode:     TimingModeCron,
		Cron:     "0 9 * * 1-5",
		Timezone: "Europe/London",
	}
	next, err := timing.FirstRunAfter(now)
	if err != nil {
		t.Fatalf("FirstRunAfter() failed: %v", err)
	}
	want := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("FirstRunAfter() = %v, want %v", next, want)
	}
}

func TestTimingExpressionIntervalAdvancesFromPreviousRun(t *testing.T) {
	previous := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	timing := TimingExpression{Mode: TimingModeInterval, Interval: 30 * time.Minute}
	next, ok, err := timing.NextRunAfter(previous)
	if err != nil {
		t.Fatalf("NextRunAfter() failed: %v", err)
	}
	if !ok {
		t.Fatalf("NextRunAfter() ok = false, want true")
	}
	want := previous.Add(30 * time.Minute)
	if !next.Equal(want) {
		t.Fatalf("NextRunAfter() = %v, want %v", next, want)
	}
}
