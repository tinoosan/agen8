// Package cron provides a minimal, dependency-free parser for standard 5-field
// cron expressions (minute hour dom month dow) and common aliases.
//
// Supported syntax per field:
//   - Wildcards: *
//   - Ranges: 1-5
//   - Steps: */5, 1-10/2
//   - Lists: 1,3,5
//
// Aliases: @yearly, @annually, @monthly, @weekly, @daily, @midnight, @hourly
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Expression is a parsed cron expression that can compute the next matching time.
type Expression struct {
	minutes  []bool // [0..59]
	hours    []bool // [0..23]
	doms     []bool // [1..31]  (index 0 unused)
	months   []bool // [1..12]  (index 0 unused)
	dows     []bool // [0..6]   (0=Sunday)
	domSet   bool   // true when dom field is not wildcard
	dowSet   bool   // true when dow field is not wildcard
	original string
}

// String returns the original cron expression.
func (e *Expression) String() string { return e.original }

// aliases maps shorthand names to their 5-field equivalents.
var aliases = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// Parse parses a 5-field cron expression or an alias and returns an Expression.
func Parse(expr string) (*Expression, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("cron: empty expression")
	}

	// Resolve aliases.
	if strings.HasPrefix(expr, "@") {
		resolved, ok := aliases[strings.ToLower(expr)]
		if !ok {
			return nil, fmt.Errorf("cron: unknown alias %q", expr)
		}
		return Parse(resolved)
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}

	e := &Expression{
		minutes: make([]bool, 60),
		hours:   make([]bool, 24),
		doms:    make([]bool, 32), // index 0 unused; 1-31
		months:  make([]bool, 13), // index 0 unused; 1-12
		dows:    make([]bool, 7),
	}
	e.original = expr

	if err := parseField(fields[0], e.minutes, 0, 59); err != nil {
		return nil, fmt.Errorf("cron: minute field: %w", err)
	}
	if err := parseField(fields[1], e.hours, 0, 23); err != nil {
		return nil, fmt.Errorf("cron: hour field: %w", err)
	}
	if err := parseField(fields[2], e.doms, 1, 31); err != nil {
		return nil, fmt.Errorf("cron: day-of-month field: %w", err)
	}
	if err := parseField(fields[3], e.months, 1, 12); err != nil {
		return nil, fmt.Errorf("cron: month field: %w", err)
	}
	if err := parseDOWField(fields[4], e.dows); err != nil {
		return nil, fmt.Errorf("cron: day-of-week field: %w", err)
	}

	e.domSet = fields[2] != "*"
	e.dowSet = fields[4] != "*"

	return e, nil
}

// Next returns the next time at or after t that matches the expression.
// It searches up to 4 years ahead to handle leap year and rare patterns.
func (e *Expression) Next(t time.Time) (time.Time, error) {
	// Start from the next whole minute after t.
	t = t.Truncate(time.Minute).Add(time.Minute)

	// Search limit: 4 years to cover leap year edge cases.
	limit := t.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		// Check month.
		if !e.months[t.Month()] {
			// Advance to next month, day 1, 00:00.
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		// Check day: when both dom and dow are restricted (not wildcard),
		// match if EITHER matches (standard cron union semantics).
		dayOK := e.matchDay(t)
		if !dayOK {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		// Check hour.
		if !e.hours[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}

		// Check minute.
		if !e.minutes[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}

		return t, nil
	}

	return time.Time{}, fmt.Errorf("cron: no matching time found within 4 years from %v", t)
}

// matchDay returns true if the given time matches the day-of-month/day-of-week
// constraints. Standard cron: if both dom and dow are restricted, match on either.
func (e *Expression) matchDay(t time.Time) bool {
	domOK := e.doms[t.Day()]
	dowOK := e.dows[int(t.Weekday())]

	if e.domSet && e.dowSet {
		// Both restricted: union semantics (match if either matches).
		return domOK || dowOK
	}
	// Only one restricted, or neither: both must match.
	return domOK && dowOK
}

// parseField parses a single cron field (minute, hour, dom, month) into a bool slice.
// lo and hi are the valid value range (inclusive).
func parseField(field string, bits []bool, lo, hi int) error {
	for _, part := range strings.Split(field, ",") {
		if err := parsePart(part, bits, lo, hi); err != nil {
			return err
		}
	}
	return nil
}

// parsePart parses one element of a comma-separated list: a value, range, wildcard,
// or any of these with a step suffix.
func parsePart(part string, bits []bool, lo, hi int) error {
	part = strings.TrimSpace(part)
	if part == "" {
		return fmt.Errorf("empty field part")
	}

	// Split on "/" for step.
	step := 1
	if idx := strings.IndexByte(part, '/'); idx >= 0 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s <= 0 {
			return fmt.Errorf("invalid step %q", part[idx+1:])
		}
		step = s
		part = part[:idx]
	}

	// Determine range.
	var start, end int
	if part == "*" {
		start, end = lo, hi
	} else if idx := strings.IndexByte(part, '-'); idx >= 0 {
		var err error
		start, err = strconv.Atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start %q", part[:idx])
		}
		end, err = strconv.Atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end %q", part[idx+1:])
		}
	} else {
		v, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid value %q", part)
		}
		start, end = v, v
	}

	if start < lo || end > hi || start > end {
		return fmt.Errorf("value out of range [%d-%d]: %d-%d", lo, hi, start, end)
	}

	for v := start; v <= end; v += step {
		bits[v] = true
	}
	return nil
}

// parseDOWField is like parseField but handles 7 as an alias for 0 (Sunday).
func parseDOWField(field string, bits []bool) error {
	for _, part := range strings.Split(field, ",") {
		if err := parseDOWPart(part, bits); err != nil {
			return err
		}
	}
	return nil
}

func parseDOWPart(part string, bits []bool) error {
	part = strings.TrimSpace(part)
	if part == "" {
		return fmt.Errorf("empty field part")
	}

	step := 1
	if idx := strings.IndexByte(part, '/'); idx >= 0 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s <= 0 {
			return fmt.Errorf("invalid step %q", part[idx+1:])
		}
		step = s
		part = part[:idx]
	}

	var start, end int
	if part == "*" {
		start, end = 0, 6
	} else if idx := strings.IndexByte(part, '-'); idx >= 0 {
		var err error
		start, err = strconv.Atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start %q", part[:idx])
		}
		end, err = strconv.Atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end %q", part[idx+1:])
		}
	} else {
		v, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid value %q", part)
		}
		start, end = v, v
	}

	// Normalize 7 → 0 (both mean Sunday).
	if start == 7 {
		start = 0
	}
	if end == 7 {
		end = 0
	}

	// Handle wrap-around ranges like 5-0 (Fri-Sun).
	if start > end {
		for v := start; v <= 6; v += step {
			bits[v] = true
		}
		for v := 0; v <= end; v += step {
			bits[v] = true
		}
		return nil
	}

	if start < 0 || end > 6 {
		return fmt.Errorf("day-of-week value out of range [0-7]: %d-%d", start, end)
	}

	for v := start; v <= end; v += step {
		bits[v] = true
	}
	return nil
}
