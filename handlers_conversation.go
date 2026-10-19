package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

// apiChatJID limits conversation operations to groups and individual accounts.
func apiChatJID(value string) (types.JID, error) {
	if strings.TrimSpace(value) != value || value == "" {
		return types.JID{}, fmt.Errorf("invalid Chat")
	}
	jid, ok := parseJID(value)
	if !ok || jid.User == "" || jid.Device != 0 || (jid.Server != types.GroupServer && jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer) {
		return types.JID{}, fmt.Errorf("invalid Chat")
	}
	return jid, nil
}

func decodeAPIRequest(r io.Reader, target any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("payload must contain one JSON object")
	}
	return nil
}

type patchBuilder func(io.Reader) (appstate.PatchInfo, error)
type patchSender func(context.Context, string, appstate.PatchInfo) error

func (s *server) appStateHandler(build patchBuilder, send patchSender) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		patch, err := build(r.Body)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		if err := send(r.Context(), userID, patch); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, `{"Details":"Updated"}`)
	}
}

func sendAPIPatch(ctx context.Context, userID string, patch appstate.PatchInfo) error {
	client := clientManager.GetWhatsmeowClient(userID)
	if client == nil {
		return fmt.Errorf("no session")
	}
	return client.SendAppState(ctx, patch)
}

func mutePatch(reader io.Reader) (appstate.PatchInfo, error) {
	var request struct {
		Chat            string
		Mute            *bool
		DurationSeconds *int64
	}
	if err := decodeAPIRequest(reader, &request); err != nil {
		return appstate.PatchInfo{}, err
	}
	chat, err := apiChatJID(request.Chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	if request.Mute == nil {
		return appstate.PatchInfo{}, fmt.Errorf("Mute is required")
	}
	var seconds int64
	if request.DurationSeconds != nil {
		seconds = *request.DurationSeconds
	}
	if seconds < 0 || seconds > int64((time.Duration(1<<63-1))/time.Second) || (!*request.Mute && seconds != 0) {
		return appstate.PatchInfo{}, fmt.Errorf("invalid DurationSeconds")
	}
	return appstate.BuildMute(chat, *request.Mute, time.Duration(seconds)*time.Second), nil
}

func unreadPatch(reader io.Reader) (appstate.PatchInfo, error) {
	var request struct {
		Chat   string
		Unread *bool
	}
	if err := decodeAPIRequest(reader, &request); err != nil {
		return appstate.PatchInfo{}, err
	}
	chat, err := apiChatJID(request.Chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	if request.Unread == nil {
		return appstate.PatchInfo{}, fmt.Errorf("Unread is required")
	}
	return appstate.BuildMarkChatAsRead(chat, !*request.Unread, time.Time{}, nil), nil
}
