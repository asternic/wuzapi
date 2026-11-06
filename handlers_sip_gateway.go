package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
)

func (s *server) UserSIPGateway() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		target := ""
		var maxCalls int
		if r.Method == http.MethodPut {
			var request struct {
				Enabled  *bool  `json:"enabled"`
				Target   string `json:"target"`
				MaxCalls *int   `json:"maxCalls"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Enabled == nil {
				s.respondAdminError(w, http.StatusBadRequest, "enabled must be a boolean")
				return
			}
			if *request.Enabled {
				if _, err := parseSIPTarget(request.Target); err != nil {
					s.respondAdminError(w, http.StatusBadRequest, err.Error())
					return
				}
				target = request.Target
			}
			if request.MaxCalls != nil && (*request.MaxCalls < 1 || *request.MaxCalls > sipAccountCallLimit) {
				s.respondAdminError(w, http.StatusBadRequest, "maxCalls must be between 1 and 32")
				return
			}
			err := s.db.QueryRowxContext(r.Context(), "UPDATE users SET sip_gateway_target = $1, sip_gateway_max_calls = COALESCE($2, sip_gateway_max_calls) WHERE id = $3 RETURNING sip_gateway_max_calls", target, request.MaxCalls, id).Scan(&maxCalls)
			if errors.Is(err, sql.ErrNoRows) {
				s.respondAdminError(w, http.StatusNotFound, "user not found")
				return
			}
			if err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "database error")
				return
			}
		} else {
			uri, limit, err := s.sipTarget(r.Context(), id)
			maxCalls = limit
			if errors.Is(err, sql.ErrNoRows) {
				s.respondAdminError(w, http.StatusNotFound, "user not found")
				return
			}
			if err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "cannot load SIP gateway configuration")
				return
			}
			if uri.Host != "" {
				target = uri.String()
			}
		}
		data, _ := json.Marshal(map[string]interface{}{"enabled": target != "", "target": target, "maxCalls": maxCalls})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}
