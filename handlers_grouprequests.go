package main

// Group join-request handlers (membership approval queue).
//
// WhatsApp groups can require admin approval for new members. whatsmeow exposes
// the underlying calls but upstream wuzapi did not surface them, so these three
// handlers wrap them:
//   GET  /group/requestparticipants        -> list pending join requests
//   POST /group/updaterequestparticipants  -> approve/reject pending requests
//   POST /group/joinapprovalmode           -> toggle the approval requirement
//
// Kept in a dedicated file to minimise rebase conflicts against upstream.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// GetGroupRequestParticipants lists the participants who have requested to join
// the group. Only meaningful when join approval mode is enabled for the group.
func (s *server) GetGroupRequestParticipants() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// Get GroupJID from query parameter
		groupJID := r.URL.Query().Get("groupJID")
		if groupJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing groupJID parameter"))
			return
		}

		group, ok := parseJID(groupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		resp, err := client.GetGroupRequestParticipants(r.Context(), group)

		if err != nil {
			msg := fmt.Sprintf("Failed to get group request participants: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
			return
		}

		responseJson, err := json.Marshal(resp)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// UpdateGroupRequestParticipants approves or rejects pending requests to join
// the group. Action must be "approve" or "reject".
func (s *server) UpdateGroupRequestParticipants() http.HandlerFunc {

	type updateGroupRequestParticipantsStruct struct {
		GroupJID string
		Phone    []string
		Action   string // approve, reject
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t updateGroupRequestParticipantsStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		// parse phone numbers
		phoneParsed := make([]types.JID, len(t.Phone))
		for i, phone := range t.Phone {
			phoneParsed[i], ok = parseJID(phone)
			if !ok {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
				return
			}
		}

		if t.Action == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Action in Payload"))
			return
		}

		// parse action
		var action whatsmeow.ParticipantRequestChange
		switch t.Action {
		case "approve":
			action = whatsmeow.ParticipantChangeApprove
		case "reject":
			action = whatsmeow.ParticipantChangeReject
		default:
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid Action in Payload (must be approve or reject)"))
			return
		}

		_, err = client.UpdateGroupRequestParticipants(r.Context(), group, phoneParsed, action)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to update group request participants")
			msg := fmt.Sprintf("failed to update group request participants: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
			return
		}

		response := map[string]interface{}{"Details": "Group request participants updated successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// SetGroupJoinApprovalMode toggles whether new members must be approved by an
// admin before joining the group.
func (s *server) SetGroupJoinApprovalMode() http.HandlerFunc {

	type setGroupJoinApprovalModeStruct struct {
		GroupJID string `json:"groupjid"`
		Mode     bool   `json:"mode"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupJoinApprovalModeStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		err = client.SetGroupJoinApprovalMode(r.Context(), group, t.Mode)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to set group join approval mode")
			msg := fmt.Sprintf("failed to set group join approval mode: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
			return
		}

		response := map[string]interface{}{"Details": "Group join approval mode updated successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}
