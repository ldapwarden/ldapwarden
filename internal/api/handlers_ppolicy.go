package api

import (
	"encoding/json"
	"net/http"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

func (s *Server) handleListPasswordPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.ldapClient.ListPasswordPolicies()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list password policies: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  policies,
		"total": len(policies),
	})
}

func (s *Server) handleGetPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.PpolicyBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	policy, err := s.ldapClient.GetPasswordPolicy(dn)
	if err != nil {
		writeError(w, http.StatusNotFound, "password policy not found")
		return
	}

	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) handleCreatePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	var req ldap.CreatePasswordPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateRDNValue("cn", req.CN); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	policy, err := s.ldapClient.CreatePasswordPolicy(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create password policy: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionPwdPolicyCreate, audit.ResourcePwdPolicy, policy.DN,
		map[string]interface{}{"cn": policy.CN})

	writeJSON(w, http.StatusCreated, policy)
}

func (s *Server) handleUpdatePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.PpolicyBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	var req ldap.UpdatePasswordPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	policy, err := s.ldapClient.UpdatePasswordPolicy(dn, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password policy: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionPwdPolicyUpdate, audit.ResourcePwdPolicy, policy.DN, nil)

	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) handleDeletePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	dn, err := resolveDN(r, s.ldapClient.PpolicyBaseDN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid dn")
		return
	}

	if err := s.ldapClient.DeletePasswordPolicy(dn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete password policy: "+err.Error())
		return
	}

	_ = s.auditLogger.Log(r.Context(), audit.ActionPwdPolicyDelete, audit.ResourcePwdPolicy, dn, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": "password policy deleted"})
}
