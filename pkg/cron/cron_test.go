package cron

import (
	"testing"
	"time"
)

func TestParseValid(t *testing.T) {
	tests := []struct {
		expr string
	}{
		{"* * * * *"},
		{"0 9 * * 1-5"},
		{"*/5 * * * *"},
		{"0 0 1 1 *"},
		{"0 0 * * 0"},
		{"1,15,30 * * * *"},
		{"0 9 1-10/2 * *"},
		{"0 0 1 * 1"},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			e, err := Parse(tc.expr)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.expr, err)
			}
			if e.String() != tc.expr {
				t.Errorf("String() = %q, want %q", e.String(), tc.expr)
			}
		})
	}
}

func TestParseAliases(t *testing.T) {
	tests := []struct {
		alias    string
		wantNext time.Time
	}{
		{"@hourly", time.Date(2026, 3, 22, 11, 0, 0, 0, time.UTC)},
		{"@daily", time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)},
		{"@weekly", time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC)},
		{"@monthly", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"@yearly", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"@annually", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"@midnight", time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)},
	}
	base := time.Date(2026, 3, 22, 10, 30, 0, 0, time.UTC) // Sunday
	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			e, err := Parse(tc.alias)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.alias, err)
			}
			next, err := e.Next(base)
			if err != nil {
				t.Fatalf("Next() failed: %v", err)
			}
			if !next.Equal(tc.wantNext) {
				t.Errorf("Next(%v) = %v, want %v", base, next, tc.wantNext)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		expr string
	}{
		{""},
		{"* * *"},
		{"* * * * * *"},
		{"60 * * * *"},
		{"* 24 * * *"},
		{"* * 32 * *"},
		{"* * * 13 *"},
		{"* * * * 8"},
		{"@unknown"},
		{"abc * * * *"},
		{"*/0 * * * *"},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			_, err := Parse(tc.expr)
			if err == nil {
				t.Fatalf("Parse(%q) should have failed", tc.expr)
			}
		})
	}
}

func TestNextWeekdays(t *testing.T) {
	// "At 09:00 on Monday through Friday"
	e, err := Parse("0 9 * * 1-5")
	if err != nil {
		t.Fatal(err)
	}

	// Sunday 2026-03-22 10:00 → next weekday 9am is Monday 2026-03-23
	base := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC) // Monday
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}

	// Friday 2026-03-27 09:01 → next is Monday 2026-03-30
	base = time.Date(2026, 3, 27, 9, 1, 0, 0, time.UTC)
	next, err = e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC) // Monday
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextEvery5Minutes(t *testing.T) {
	e, err := Parse("*/5 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 3, 22, 10, 3, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextMonthBoundary(t *testing.T) {
	// First of every month at midnight.
	e, err := Parse("0 0 1 * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextYearWrap(t *testing.T) {
	// Dec 31 at midnight.
	e, err := Parse("0 0 31 12 *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 12, 31, 0, 1, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextFeb29LeapYear(t *testing.T) {
	// Feb 29 at 6am — only fires on leap years.
	e, err := Parse("0 6 29 2 *")
	if err != nil {
		t.Fatal(err)
	}

	// 2026 is not a leap year; next leap year is 2028.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2028, 2, 29, 6, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextListValues(t *testing.T) {
	// At minutes 0, 15, 30, 45 of every hour.
	e, err := Parse("0,15,30,45 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 3, 22, 10, 16, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 22, 10, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestNextStepRange(t *testing.T) {
	// Every 2nd day from 1-10: days 1, 3, 5, 7, 9.
	e, err := Parse("0 9 1-10/2 * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 5, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestDOW7AsSunday(t *testing.T) {
	// 7 is an alias for 0 (Sunday).
	e, err := Parse("0 12 * * 7")
	if err != nil {
		t.Fatal(err)
	}

	// 2026-03-22 is a Sunday.
	base := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC) // Saturday
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestDOMAndDOWUnion(t *testing.T) {
	// When both dom and dow are restricted, match if either matches (union).
	// "On the 15th OR on Mondays, at 9am"
	e, err := Parse("0 9 15 * 1")
	if err != nil {
		t.Fatal(err)
	}

	// 2026-03-22 is Sunday. Next Monday is 23rd (not 15th, but dow matches).
	base := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	next, err := e.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC) // Monday
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}
