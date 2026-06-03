package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const scheduleRunnerInterval = 30 * time.Second
const scheduleRunnerBatchLimit = 25

func (d *Daemon) startScheduleRunner(ctx context.Context) error {
	if d == nil || d.app == nil || d.app.ScheduleSvc == nil {
		return fmt.Errorf("schedule service is required")
	}
	go func() {
		ticker := time.NewTicker(scheduleRunnerInterval)
		defer ticker.Stop()
		d.runDueSchedules(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.runDueSchedules(ctx)
			}
		}
	}()
	return nil
}

func (d *Daemon) runDueSchedules(ctx context.Context) {
	runs, err := d.app.ScheduleSvc.RunDue(ctx, scheduleRunnerBatchLimit)
	if err != nil {
		slog.Warn("schedule: run due entries failed", "error", err, "runs", len(runs))
	}
}
