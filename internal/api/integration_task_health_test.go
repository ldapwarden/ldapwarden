//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestIntegration_TaskHealth lists each configured task with its next fire
// time. The test server never starts the scheduler, so nothing can be overdue:
// a task that has not had the chance to run has not missed one.
func TestIntegration_TaskHealth(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	resp, body := doJSON(t, env, http.MethodGet, "/api/admin/scheduled-tasks/health", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var result struct {
		Data []struct {
			TaskName string          `json:"taskName"`
			Schedule string          `json:"schedule"`
			LastRun  json.RawMessage `json:"lastRun"`
			Overdue  bool            `json:"overdue"`
			NextRun  string          `json:"nextRun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}

	configured := map[string]string{
		"users_expiration":     env.Config.ScheduledTasks.UsersExpiration,
		"passwords_expiration": env.Config.ScheduledTasks.PasswordsExpiration,
		"lockouts_scan":        env.Config.ScheduledTasks.LockoutScan,
	}
	want := 0
	for _, spec := range configured {
		if spec != "" {
			want++
		}
	}
	if len(result.Data) != want {
		t.Fatalf("got %d tasks, want the %d with a schedule: %s", len(result.Data), want, body)
	}
	for _, task := range result.Data {
		if configured[task.TaskName] != task.Schedule {
			t.Errorf("%s: schedule = %q, want %q", task.TaskName, task.Schedule, configured[task.TaskName])
		}
		if task.Overdue {
			t.Errorf("%s is overdue although the scheduler never started", task.TaskName)
		}
		next, err := time.Parse(time.RFC3339, task.NextRun)
		if err != nil {
			t.Errorf("%s: nextRun %q: %v", task.TaskName, task.NextRun, err)
		} else if !next.After(time.Now().Add(-time.Minute)) {
			t.Errorf("%s: nextRun %v is in the past", task.TaskName, next)
		}
	}
}

// A read-only session has no settings:read, like the run history.
func TestIntegration_TaskHealth_RequiresSettingsRead(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "viewer", "viewer123").Token

	resp, _ := doJSON(t, env, http.MethodGet, "/api/admin/scheduled-tasks/health", nil, token)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status=%d, want 403", resp.StatusCode)
	}
}
