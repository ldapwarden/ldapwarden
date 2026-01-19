package api

import (
	"net/http"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/auth"
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
		response.Data = append(response.Data, tr)
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetScheduledTasksConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.config.ScheduledTasks
	response := ScheduledTasksConfigResponse{
		UsersExpiration:     getConfigValue("LDAPWARDEN_SCHEDULED_TASKS_USERS_EXPIRATION", cfg.UsersExpiration, "42 3 * * *"),
		PasswordsExpiration: getConfigValue("LDAPWARDEN_SCHEDULED_TASKS_PASSWORDS_EXPIRATION", cfg.PasswordsExpiration, "42 3 * * *"),
	}
	writeJSON(w, http.StatusOK, response)
}

// Helper to get session from request context
func (s *Server) getSession(r *http.Request) *auth.Session {
	return auth.GetSessionFromContext(r.Context())
}
