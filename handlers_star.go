package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

type starRequest struct {
	Chat   string
	Sender string
	Id     string
	FromMe *bool
	Star   *bool
}

func (t starRequest) patch() (appstate.PatchInfo, error) {
	chat, ok := parseJID(t.Chat)
	if !ok || chat.IsEmpty() || (chat.Server != types.GroupServer && chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer) {
		return appstate.PatchInfo{}, fmt.Errorf("invalid Chat")
	}
	if t.Id == "" || t.FromMe == nil || t.Star == nil {
		return appstate.PatchInfo{}, fmt.Errorf("Id, FromMe and Star are required")
	}
	sender := chat
	if t.Sender != "" {
		sender, ok = parseJID(t.Sender)
		if !ok || sender.IsEmpty() || (sender.Server != types.DefaultUserServer && sender.Server != types.HiddenUserServer) {
			return appstate.PatchInfo{}, fmt.Errorf("invalid Sender")
		}
	} else if chat.Server == types.GroupServer {
		return appstate.PatchInfo{}, fmt.Errorf("Sender is required for group messages")
	}
	return appstate.BuildStar(chat, sender, t.Id, *t.FromMe, *t.Star), nil
}

func (s *server) StarMessage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request starRequest
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
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("error updating message star: %w", err))
			return
		}
		s.Respond(w, r, http.StatusOK, `{"Details":"Message star updated"}`)
	}
}
