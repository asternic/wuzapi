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
		var allowOutbound bool
		if r.Method == http.MethodPut {
			var request struct {
				Enabled       *bool  `json:"enabled"`
				Target        string `json:"target"`
				MaxCalls      *int   `json:"maxCalls"`
				AllowOutbound *bool  `json:"allowOutbound"`
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
			err := s.db.QueryRowxContext(r.Context(), "UPDATE users SET sip_gateway_target = $1, sip_gateway_max_calls = COALESCE($2, sip_gateway_max_calls)  , sip_gateway_outbound = CASE WHEN $1 = '' THEN FALSE ELSE COALESCE($3, sip_gateway_outbound) END WHERE id = $4 RETURNING sip_gateway_max_calls, sip_gateway_outbound", target, request.MaxCalls, request.AllowOutbound, id).Scan(&maxCalls, &allowOutbound)
			if errors.Is(err, sql.ErrNoRows) {
				s.respondAdminError(w, http.StatusNotFound, "user not found")
				return
			}
			if err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "database error")
				return
			}
		} else {
			uri, limit, outbound, err := s.sipTarget(r.Context(), id)
			allowOutbound = outbound
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
		data, _ := json.Marshal(map[string]interface{}{"enabled": target != "", "target": target, "maxCalls": maxCalls, "allowOutbound": allowOutbound})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}
