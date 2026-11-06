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
		if r.Method == http.MethodPut {
			var request struct {
				Enabled *bool  `json:"enabled"`
				Target  string `json:"target"`
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
			result, err := s.db.ExecContext(r.Context(), "UPDATE users SET sip_gateway_target = $1 WHERE id = $2", target, id)
			if err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "database error")
				return
			}
			count, err := result.RowsAffected()
			if err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "database error")
				return
			}
			if count == 0 {
				s.respondAdminError(w, http.StatusNotFound, "user not found")
				return
			}
		} else {
			uri, err := s.sipTarget(r.Context(), id)
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
		data, _ := json.Marshal(map[string]interface{}{"enabled": target != "", "target": target})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}
