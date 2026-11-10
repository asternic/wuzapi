package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

func (s *server) UserSIPGateway() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		target := ""
		var maxCalls int
		var allowOutbound bool
		var authUsername string
		var hasAuthPassword bool
		if r.Method == http.MethodPut {
			var request struct {
				Enabled       *bool   `json:"enabled"`
				Target        string  `json:"target"`
				MaxCalls      *int    `json:"maxCalls"`
				AllowOutbound *bool   `json:"allowOutbound"`
				AuthUsername  *string `json:"authUsername"`
				AuthPassword  *string `json:"authPassword"`
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

			if request.AuthUsername != nil && strings.ContainsAny(*request.AuthUsername, "\r\n") {
				s.respondAdminError(w, http.StatusBadRequest, "authUsername contains invalid characters")
				return
			}
			var encryptedPassword *string
			if request.AuthPassword != nil {
				value := ""
				if *request.AuthPassword != "" {
					encrypted, err := encryptHMACKey(*request.AuthPassword)
					if err != nil {
						s.respondAdminError(w, http.StatusInternalServerError, "cannot encrypt SIP password; check the global encryption key")
						return
					}
					value = base64.StdEncoding.EncodeToString(encrypted)
				}
				encryptedPassword = &value
			}
			err := s.db.QueryRowxContext(r.Context(), "UPDATE users SET sip_gateway_target = $1, sip_gateway_max_calls = COALESCE($2, sip_gateway_max_calls)  , sip_gateway_outbound = CASE WHEN $1 = '' THEN FALSE ELSE COALESCE($3, sip_gateway_outbound) END  , sip_gateway_auth_username = COALESCE($4, sip_gateway_auth_username), sip_gateway_auth_password = COALESCE($5, sip_gateway_auth_password) WHERE id = $6 RETURNING sip_gateway_max_calls, sip_gateway_outbound, sip_gateway_auth_username, (sip_gateway_auth_password <> '')", target, request.MaxCalls, request.AllowOutbound, request.AuthUsername, encryptedPassword, id).Scan(&maxCalls, &allowOutbound, &authUsername, &hasAuthPassword)
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
			if err := s.db.QueryRowxContext(r.Context(), "SELECT sip_gateway_auth_username, (sip_gateway_auth_password <> '') FROM users WHERE id = $1", id).Scan(&authUsername, &hasAuthPassword); err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "cannot load SIP authentication settings")
				return
			}
			if uri.Host != "" {
				target = uri.String()
			}
		}
		data, _ := json.Marshal(map[string]interface{}{"enabled": target != "", "target": target, "maxCalls": maxCalls, "allowOutbound": allowOutbound, "authUsername": authUsername, "hasAuthPassword": hasAuthPassword})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}
