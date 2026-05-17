package api

import (
	"net/http"
)

type NextIDsResponse struct {
	NextUID int `json:"nextUid"`
	NextGID int `json:"nextGid"`
	MinUID  int `json:"minUid"`
	MinGID  int `json:"minGid"`
}

func (s *Server) handleGetNextIDs(w http.ResponseWriter, r *http.Request) {
	nextUID, err := s.ldapClient.NextUID()
	if err != nil {
		writeServerError(w, r, "get next UID", err)
		return
	}

	nextGID, err := s.ldapClient.NextGID()
	if err != nil {
		writeServerError(w, r, "get next GID", err)
		return
	}

	writeJSON(w, http.StatusOK, NextIDsResponse{
		NextUID: nextUID,
		NextGID: nextGID,
		MinUID:  s.ldapClient.MinUID(),
		MinGID:  s.ldapClient.MinGID(),
	})
}
