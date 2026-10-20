package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

type chatPinRequest struct {
	Chat string
	Pin  *bool
}

func (t chatPinRequest) patch() (appstate.PatchInfo, error) {
	chat, ok := parseJID(t.Chat)
	if !ok || chat.IsEmpty() || (chat.Server != types.GroupServer && chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer) {
		return appstate.PatchInfo{}, fmt.Errorf("invalid Chat")
	}
	if t.Pin == nil {
		return appstate.PatchInfo{}, fmt.Errorf("Pin is required")
	}
	return appstate.BuildPin(chat, *t.Pin), nil
}

func (s *server) PinChat() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request chatPinRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, fmt.Errorf("could not decode Payload"))
			return
		}
		patch, err := request.patch()
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		client := clientManager.GetWhatsmeowClient(r.Context().Value("userinfo").(Values).Get("Id"))
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("no session"))
			return
		}
		if err = client.SendAppState(r.Context(), patch); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("error updating chat pin: %w", err))
			return
		}
		s.Respond(w, r, http.StatusOK, `{"Details":"Chat pin updated"}`)
	}
}
