package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// maxImportRows bounds a single bulk-import request. Rows carry no photos, so
// 1000 stays well within the route's body-size cap while keeping the operation
// (and its per-row audit writes) reasonable.
const maxImportRows = 1000

type importUsersRequest struct {
	Rows []ldap.CreateUserRequest `json:"rows"`
}

type importGroupsRequest struct {
	Rows []ldap.CreateGroupRequest `json:"rows"`
}

// importRowResult is the outcome of one imported row. Key is the uid/cn so the
// client can line results up with its parsed input.
type importRowResult struct {
	Index  int    `json:"index"`
	Key    string `json:"key"`
	Status string `json:"status"` // "created" | "error"
	Error  string `json:"error,omitempty"`
}

type importResponse struct {
	Created int               `json:"created"`
	Failed  int               `json:"failed"`
	Results []importRowResult `json:"results"`
}

func (s *Server) handleImportUsers(w http.ResponseWriter, r *http.Request) {
	var req importUsersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "no rows to import")
		return
	}
	if len(req.Rows) > maxImportRows {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many rows (max %d)", maxImportRows))
		return
	}

	// Per-row audit rows are written, but their notification emails are
	// suppressed; one summary email is sent at the end instead.
	ctx := audit.ContextSuppressingNotify(r.Context())
	resp := importResponse{Results: make([]importRowResult, 0, len(req.Rows))}

	for i, row := range req.Rows {
		res := importRowResult{Index: i, Key: row.UID, Status: "created"}
		if err := validateImportUser(row, s.ldapClient.MinUID(), s.ldapClient.MinGID()); err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, importFailed(res, err.Error()))
			continue
		}
		plannedDN := "uid=" + row.UID + "," + s.ldapClient.UserBaseDN()
		details := map[string]interface{}{
			audit.DetailsKeyResourceName: createUserDisplayName(row),
			audit.DetailsKeyChanges:      userCreateFields(row),
		}
		if err := s.auditLogger.Log(ctx, audit.ActionUserCreate, audit.ResourceUser, plannedDN, details); err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, importFailed(res, "audit unavailable; row skipped"))
			continue
		}
		if _, err := s.ldapClient.CreateUser(row); err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, importFailed(res, importErrorMessage(err)))
			continue
		}
		resp.Created++
		resp.Results = append(resp.Results, res)
	}

	s.notifyBulkImport(r, "users", resp.Created, resp.Failed)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleImportGroups(w http.ResponseWriter, r *http.Request) {
	var req importGroupsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "no rows to import")
		return
	}
	if len(req.Rows) > maxImportRows {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many rows (max %d)", maxImportRows))
		return
	}

	ctx := audit.ContextSuppressingNotify(r.Context())
	resp := importResponse{Results: make([]importRowResult, 0, len(req.Rows))}

	for i, row := range req.Rows {
		res := importRowResult{Index: i, Key: row.CN, Status: "created"}
		if err := validateImportGroup(row, s.ldapClient.MinGID()); err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, importFailed(res, err.Error()))
			continue
		}
		plannedDN := "cn=" + row.CN + "," + s.ldapClient.GroupBaseDN()
		details := map[string]interface{}{
			audit.DetailsKeyResourceName: labelWithID(row.Description, row.CN),
			audit.DetailsKeyChanges:      groupCreateFields(row),
		}
		if err := s.auditLogger.Log(ctx, audit.ActionGroupCreate, audit.ResourceGroup, plannedDN, details); err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, importFailed(res, "audit unavailable; row skipped"))
			continue
		}
		if _, err := s.ldapClient.CreateGroup(row); err != nil {
			resp.Failed++
			resp.Results = append(resp.Results, importFailed(res, importErrorMessage(err)))
			continue
		}
		resp.Created++
		resp.Results = append(resp.Results, res)
	}

	s.notifyBulkImport(r, "groups", resp.Created, resp.Failed)
	writeJSON(w, http.StatusOK, resp)
}

func importFailed(res importRowResult, msg string) importRowResult {
	res.Status = "error"
	res.Error = msg
	return res
}

// validateImportUser mirrors the checks in handleCreateUser so an imported row
// is rejected by the same rules a single create would apply.
func validateImportUser(req ldap.CreateUserRequest, minUID, minGID int) error {
	if req.SN == "" || req.GivenName == "" {
		return fmt.Errorf("sn and givenName are required")
	}
	if err := validateRDNValue("uid", req.UID); err != nil {
		return err
	}
	for _, groupCN := range req.Groups {
		if err := validateRDNValue("group cn", groupCN); err != nil {
			return err
		}
	}
	if err := validatePOSIXID("uidNumber", req.UIDNumber, minUID); err != nil {
		return err
	}
	if err := validatePOSIXID("gidNumber", req.GIDNumber, minGID); err != nil {
		return err
	}
	return nil
}

func validateImportGroup(req ldap.CreateGroupRequest, minGID int) error {
	if err := validateRDNValue("cn", req.CN); err != nil {
		return err
	}
	if err := validatePOSIXID("gidNumber", req.GIDNumber, minGID); err != nil {
		return err
	}
	return nil
}

// importErrorMessage maps a per-row create failure to a client-safe message,
// reusing the directory-error mapping so useful hints ("entry already exists")
// survive without leaking raw LDAP diagnostics.
func importErrorMessage(err error) string {
	if msg, _, ok := ldapErrorResponse(err); ok {
		return msg
	}
	return "failed to create entry"
}

// notifyBulkImport sends the single post-import summary email, off the request
// path so SMTP latency never blocks the response. No-op when no recipients are
// configured.
func (s *Server) notifyBulkImport(r *http.Request, kind string, created, failed int) {
	recipients := s.config.App.AuditNotifyEmails
	if len(recipients) == 0 || s.mailer == nil {
		return
	}
	actor := ""
	if session := auth.GetSessionFromContext(r.Context()); session != nil {
		actor = labelWithID(session.DisplayName, session.UserUID)
	}
	info := audit.RequestInfoFromContext(r.Context())
	recipientsCopy := append([]string(nil), recipients...)
	timestamp := time.Now()

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("bulk import notification: panic recovered: %v", rec)
			}
		}()
		if err := s.mailer.SendBulkImportSummary(recipientsCopy, actor, kind, created, failed, timestamp, info.IPAddress, info.UserAgent); err != nil {
			log.Printf("bulk import notification: %v", err)
		}
	}()
}
