package scheduler

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// TestExpectedRun pins when a task counts as having missed a scheduled run.
// The history cannot say it — a run that never happened leaves no row — so the
// schedule has to be compared against it.
func TestExpectedRun(t *testing.T) {
	daily, err := cron.ParseStandard("0 2 * * *") // 02:00 every day
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	ptr := func(v time.Time) *time.Time { return &v }

	tests := []struct {
		name          string
		lastScheduled *time.Time
		start         string
		now           string
		wantOverdue   bool
		wantExpected  string
	}{
		{
			name:          "ran last night, next run tonight",
			lastScheduled: ptr(at("2026-09-24 02:00")),
			start:         "2026-09-01 00:00",
			now:           "2026-09-24 15:00",
		},
		{
			name:          "last night's run is missing",
			lastScheduled: ptr(at("2026-09-23 02:00")),
			start:         "2026-09-01 00:00",
			now:           "2026-09-24 15:00",
			wantOverdue:   true,
			wantExpected:  "2026-09-24 02:00",
		},
		{
			name:          "within the grace after the fire time",
			lastScheduled: ptr(at("2026-09-23 02:00")),
			start:         "2026-09-01 00:00",
			now:           "2026-09-24 02:05",
		},
		{
			name:  "never ran, started after tonight's fire time",
			start: "2026-09-24 09:00",
			now:   "2026-09-24 15:00",
		},
		{
			name:         "never ran, and a fire time since start has passed",
			start:        "2026-09-23 09:00",
			now:          "2026-09-24 15:00",
			wantOverdue:  true,
			wantExpected: "2026-09-24 02:00",
		},
		{
			// The process was down at 02:00: that run was missed, and a restart
			// afterwards must not hide it.
			name:          "restarted after a missed fire time",
			lastScheduled: ptr(at("2026-09-23 02:00")),
			start:         "2026-09-24 09:00",
			now:           "2026-09-24 10:00",
			wantOverdue:   true,
			wantExpected:  "2026-09-24 02:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, overdue := expectedRun(daily, tt.lastScheduled, at(tt.start), at(tt.now))
			if overdue != tt.wantOverdue {
				t.Fatalf("overdue = %v, want %v (expected %v)", overdue, tt.wantOverdue, expected)
			}
			if tt.wantOverdue && !expected.Equal(at(tt.wantExpected)) {
				t.Errorf("expected run = %v, want %v", expected, at(tt.wantExpected))
			}
		})
	}
}
