package api

import (
	"net/http"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/scheduler"
)

// TriggerTaskResponse is the response for manual task triggers
type TriggerTaskResponse struct {
	RunID             string   `json:"runId"`
	TaskName          string   `json:"taskName"`
	UsersChecked      int      `json:"usersChecked"`
	NotificationsSent int      `json:"notificationsSent"`
	Errors            []string `json:"errors,omitempty"`
}

// TaskRunResponse represents a task run in API responses
type TaskRunResponse struct {
	ID                string   `json:"id"`
	TaskName          string   `json:"taskName"`
	StartedAt         string   `json:"startedAt"`
	CompletedAt       *string  `json:"completedAt,omitempty"`
	Status            string   `json:"status"`
	UsersChecked      int      `json:"usersChecked"`
	NotificationsSent int      `json:"notificationsSent"`
	Errors            []string `json:"errors,omitempty"`
	TriggeredBy       string   `json:"triggeredBy"`
}

// TaskRunsResponse is the list response for task runs
type TaskRunsResponse struct {
	Data  []TaskRunResponse `json:"data"`
	Total int               `json:"total"`
}

// ScheduledTasksConfigResponse contains the scheduled tasks configuration
type ScheduledTasksConfigResponse struct {
	UsersExpiration     ConfigValue `json:"usersExpiration"`
	PasswordsExpiration ConfigValue `json:"passwordsExpiration"`
	LockoutScan         ConfigValue `json:"lockoutScan"`
}

func (s *Server) handleTriggerAccountExpirationTask(w http.ResponseWriter, r *http.Request) {
	// Get the current user from context for audit
	session := s.getSession(r)
	triggeredBy := "unknown"
	if session != nil {
		triggeredBy = session.UserUID
	}

	result, err := s.scheduler.TriggerAccountExpiration(r.Context(), triggeredBy)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, TriggerTaskResponse{
		RunID:             result.RunID.String(),
		TaskName:          result.TaskName,
		UsersChecked:      result.UsersChecked,
		NotificationsSent: result.NotificationsSent,
		Errors:            result.Errors,
	})
}

func (s *Server) handleTriggerPasswordExpirationTask(w http.ResponseWriter, r *http.Request) {
	// Get the current user from context for audit
	session := s.getSession(r)
	triggeredBy := "unknown"
	if session != nil {
		triggeredBy = session.UserUID
	}

	result, err := s.scheduler.TriggerPasswordExpiration(r.Context(), triggeredBy)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, TriggerTaskResponse{
		RunID:             result.RunID.String(),
		TaskName:          result.TaskName,
		UsersChecked:      result.UsersChecked,
		NotificationsSent: result.NotificationsSent,
		Errors:            result.Errors,
	})
}

func (s *Server) handleTriggerLockoutScanTask(w http.ResponseWriter, r *http.Request) {
	session := s.getSession(r)
	triggeredBy := "unknown"
	if session != nil {
		triggeredBy = session.UserUID
	}

	result, err := s.scheduler.TriggerLockoutScan(r.Context(), triggeredBy)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, TriggerTaskResponse{
		RunID:             result.RunID.String(),
		TaskName:          result.TaskName,
		UsersChecked:      result.UsersChecked,
		NotificationsSent: result.NotificationsSent,
		Errors:            result.Errors,
	})
}

func (s *Server) handleGetTaskRuns(w http.ResponseWriter, r *http.Request) {
	taskName := r.URL.Query().Get("taskName")
	limit := 50

	runs, err := s.scheduler.GetTaskRuns(r.Context(), taskName, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	response := TaskRunsResponse{
		Data:  make([]TaskRunResponse, 0, len(runs)),
		Total: len(runs),
	}

	for _, run := range runs {
		response.Data = append(response.Data, taskRunResponse(run))
	}

	writeJSON(w, http.StatusOK, response)
}

func taskRunResponse(run scheduler.TaskRun) TaskRunResponse {
	tr := TaskRunResponse{
		ID:                run.ID.String(),
		TaskName:          run.TaskName,
		StartedAt:         run.StartedAt.Format(time.RFC3339),
		Status:            run.Status,
		UsersChecked:      run.UsersChecked,
		NotificationsSent: run.NotificationsSent,
		Errors:            run.Errors,
		TriggeredBy:       run.TriggeredBy,
	}
	if run.CompletedAt != nil {
		completed := run.CompletedAt.Format(time.RFC3339)
		tr.CompletedAt = &completed
	}
	return tr
}

// TaskHealthResponse is one configured task's standing, for the dashboard.
type TaskHealthResponse struct {
	TaskName   string           `json:"taskName"`
	Schedule   string           `json:"schedule"`
	LastRun    *TaskRunResponse `json:"lastRun"`
	Overdue    bool             `json:"overdue"`
	ExpectedAt *string          `json:"expectedAt,omitempty"`
	NextRun    string           `json:"nextRun"`
}

// handleGetTaskHealth answers, per configured task, whether it is running as
// scheduled. The run history alone cannot: a run that never happened leaves
// no row to show.
func (s *Server) handleGetTaskHealth(w http.ResponseWriter, r *http.Request) {
	health, err := s.scheduler.TaskHealth(r.Context(), time.Now())
	if err != nil {
		writeServerError(w, r, "read task health", err)
		return
	}

	data := make([]TaskHealthResponse, 0, len(health))
	for _, h := range health {
		item := TaskHealthResponse{
			TaskName: h.TaskName,
			Schedule: h.Schedule,
			Overdue:  h.Overdue,
			NextRun:  h.NextRun.Format(time.RFC3339),
		}
		if h.LastRun != nil {
			run := taskRunResponse(*h.LastRun)
			item.LastRun = &run
		}
		if h.ExpectedAt != nil {
			expected := h.ExpectedAt.Format(time.RFC3339)
			item.ExpectedAt = &expected
		}
		data = append(data, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) handleGetScheduledTasksConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.config.ScheduledTasks
	response := ScheduledTasksConfigResponse{
		UsersExpiration:     getConfigValue("LDAPWARDEN_SCHEDULED_TASKS_USERS_EXPIRATION", cfg.UsersExpiration, "42 3 * * *"),
		PasswordsExpiration: getConfigValue("LDAPWARDEN_SCHEDULED_TASKS_PASSWORDS_EXPIRATION", cfg.PasswordsExpiration, "42 3 * * *"),
		LockoutScan:         getConfigValue("LDAPWARDEN_SCHEDULED_TASKS_LOCKOUT_SCAN", cfg.LockoutScan, "*/5 * * * *"),
	}
	writeJSON(w, http.StatusOK, response)
}

// Helper to get session from request context
func (s *Server) getSession(r *http.Request) *auth.Session {
	return auth.GetSessionFromContext(r.Context())
}
