package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/pkg/cron"
)

type TimingMode string

const (
	TimingModeOnce     TimingMode = "once"
	TimingModeInterval TimingMode = "interval"
	TimingModeCron     TimingMode = "cron"
)

type TimingExpression struct {
	Mode     TimingMode    `json:"mode"`
	RunAt    *time.Time    `json:"runAt,omitempty"`
	Interval time.Duration `json:"interval,omitempty"`
	Cron     string        `json:"cron,omitempty"`
	Timezone string        `json:"timezone,omitempty"`
}

func (t TimingExpression) Validate() error {
	switch t.Mode {
	case TimingModeOnce:
		if t.RunAt == nil {
			return fmt.Errorf("runAt is required for once timing")
		}
		if t.Interval != 0 {
			return fmt.Errorf("interval is not valid for once timing")
		}
		if strings.TrimSpace(t.Cron) != "" {
			return fmt.Errorf("cron is not valid for once timing")
		}
		if strings.TrimSpace(t.Timezone) != "" {
			return fmt.Errorf("timezone is not valid for once timing")
		}
	case TimingModeInterval:
		if t.Interval <= 0 {
			return fmt.Errorf("positive interval is required for interval timing")
		}
		if t.RunAt != nil {
			return fmt.Errorf("runAt is not valid for interval timing")
		}
		if strings.TrimSpace(t.Cron) != "" {
			return fmt.Errorf("cron is not valid for interval timing")
		}
		if strings.TrimSpace(t.Timezone) != "" {
			return fmt.Errorf("timezone is not valid for interval timing")
		}
	case TimingModeCron:
		if strings.TrimSpace(t.Cron) == "" {
			return fmt.Errorf("cron is required for cron timing")
		}
		if strings.TrimSpace(t.Timezone) == "" {
			return fmt.Errorf("timezone is required for cron timing")
		}
		if t.RunAt != nil {
			return fmt.Errorf("runAt is not valid for cron timing")
		}
		if t.Interval != 0 {
			return fmt.Errorf("interval is not valid for cron timing")
		}
		if _, err := time.LoadLocation(strings.TrimSpace(t.Timezone)); err != nil {
			return fmt.Errorf("load schedule timezone: %w", err)
		}
		if _, err := cron.Parse(strings.TrimSpace(t.Cron)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid timing mode %q", t.Mode)
	}
	return nil
}

func (t TimingExpression) FirstRunAfter(now time.Time) (time.Time, error) {
	if err := t.Validate(); err != nil {
		return time.Time{}, err
	}
	switch t.Mode {
	case TimingModeOnce:
		return t.RunAt.UTC(), nil
	case TimingModeInterval:
		return now.UTC().Add(t.Interval), nil
	case TimingModeCron:
		return t.nextCronAfter(now)
	default:
		return time.Time{}, fmt.Errorf("invalid timing mode %q", t.Mode)
	}
}

func (t TimingExpression) NextRunAfter(previous time.Time) (time.Time, bool, error) {
	if err := t.Validate(); err != nil {
		return time.Time{}, false, err
	}
	switch t.Mode {
	case TimingModeOnce:
		return time.Time{}, false, nil
	case TimingModeInterval:
		return previous.UTC().Add(t.Interval), true, nil
	case TimingModeCron:
		next, err := t.nextCronAfter(previous)
		return next, err == nil, err
	default:
		return time.Time{}, false, fmt.Errorf("invalid timing mode %q", t.Mode)
	}
}

func (t TimingExpression) normalized() TimingExpression {
	next := t
	next.Cron = strings.TrimSpace(next.Cron)
	next.Timezone = strings.TrimSpace(next.Timezone)
	if next.RunAt != nil {
		runAt := next.RunAt.UTC()
		next.RunAt = &runAt
	}
	return next
}

func (t TimingExpression) NormalizedForUpdate() TimingExpression {
	return t.normalized()
}

func (t TimingExpression) nextCronAfter(after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(strings.TrimSpace(t.Timezone))
	if err != nil {
		return time.Time{}, fmt.Errorf("load schedule timezone: %w", err)
	}
	expr, err := cron.Parse(strings.TrimSpace(t.Cron))
	if err != nil {
		return time.Time{}, err
	}
	next, err := expr.Next(after.In(loc))
	if err != nil {
		return time.Time{}, err
	}
	return next.UTC(), nil
}
