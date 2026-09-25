package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// overdueGrace is how late a scheduled run may start before the task counts as
// overdue: cron fires on the minute, and a run records itself once it starts,
// so anything under a few minutes is noise rather than a missed run.
const overdueGrace = 10 * time.Minute

// TaskHealth is where a configured task stands, for the dashboard.
//
// A run that fails leaves a row saying so. A run that never happens leaves
// nothing at all — the process was down at the scheduled time, or the cron
// entry was never registered — which is why "a task that stops running is a
// silent failure" and why this compares the schedule against the history
// instead of only listing the history.
type TaskHealth struct {
	TaskName string
	Schedule string
	// LastRun is the most recent run, manual or scheduled.
	LastRun *TaskRun
	// Overdue is set when a scheduled run should have started by now and has
	// not. A manual run does not clear it: it proves the task works, not that
	// the schedule fires.
	Overdue bool
	// ExpectedAt is the scheduled start that was missed, when Overdue.
	ExpectedAt *time.Time
	NextRun    time.Time
}

// expectedRun is the scheduled start the history should already show: the
// first fire time after the last scheduled run, or after the scheduler started
// when there is none (a fire time before it could not have happened here).
// Returns ok=false while that time plus the grace is still in the future.
//
// A last run from before this process started still anchors the schedule: a
// fire time that fell while the process was down was missed, and saying so is
// the point.
func expectedRun(schedule cron.Schedule, lastScheduled *time.Time, schedulerStart, now time.Time) (time.Time, bool) {
	from := schedulerStart
	if lastScheduled != nil {
		from = *lastScheduled
	}
	expected := schedule.Next(from)
	if expected.Add(overdueGrace).After(now) {
		return time.Time{}, false
	}
	return expected, true
}

// TaskHealth reports each task that has a schedule configured. Tasks with an
// empty schedule are disabled and left out: nothing is expected of them.
func (s *Scheduler) TaskHealth(ctx context.Context, now time.Time) ([]TaskHealth, error) {
	configured := []struct{ name, spec string }{
		{"users_expiration", s.config.ScheduledTasks.UsersExpiration},
		{"passwords_expiration", s.config.ScheduledTasks.PasswordsExpiration},
		{"lockouts_scan", s.config.ScheduledTasks.LockoutScan},
	}

	out := make([]TaskHealth, 0, len(configured))
	for _, task := range configured {
		if task.spec == "" {
			continue
		}
		// The same parser cron.New() uses, so a spec Start accepted parses here.
		schedule, err := cron.ParseStandard(task.spec)
		if err != nil {
			return nil, fmt.Errorf("parse schedule of %s: %w", task.name, err)
		}

		last, err := s.store.LastTaskRun(ctx, task.name, false)
		if err != nil {
			return nil, err
		}
		lastScheduled, err := s.store.LastTaskRun(ctx, task.name, true)
		if err != nil {
			return nil, err
		}

		health := TaskHealth{
			TaskName: task.name,
			Schedule: task.spec,
			LastRun:  last,
			NextRun:  schedule.Next(now),
		}
		var lastScheduledAt *time.Time
		if lastScheduled != nil {
			lastScheduledAt = &lastScheduled.StartedAt
		}
		// Before Start there is no schedule running to have missed anything.
		if s.startedAt.IsZero() {
			out = append(out, health)
			continue
		}
		if expected, overdue := expectedRun(schedule, lastScheduledAt, s.startedAt, now); overdue {
			health.Overdue = true
			health.ExpectedAt = &expected
		}
		out = append(out, health)
	}
	return out, nil
}
