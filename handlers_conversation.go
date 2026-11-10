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
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
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

// deleteChatPatch builds the app state patch that deletes a whole conversation
// on every linked device, like "Delete chat" in the phone app. WhatsApp clients
// use the last message to decide which messages the deletion covers, so callers
// that know it should send LastMessageTimestamp and LastMessageId; without them
// the range ends at the current time and carries no message key.
func deleteChatPatch(reader io.Reader) (appstate.PatchInfo, error) {
	var request struct {
		Chat                 string
		DeleteMedia          *bool
		LastMessageTimestamp *int64
		LastMessageId        string
		LastMessageFromMe    *bool
		LastMessageSender    string
	}
	if err := decodeAPIRequest(reader, &request); err != nil {
		return appstate.PatchInfo{}, err
	}
	chat, err := apiChatJID(request.Chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	var lastTimestamp time.Time
	if request.LastMessageTimestamp != nil {
		if *request.LastMessageTimestamp <= 0 || *request.LastMessageTimestamp > time.Now().Add(24*time.Hour).Unix() {
			return appstate.PatchInfo{}, fmt.Errorf("invalid LastMessageTimestamp")
		}
		lastTimestamp = time.Unix(*request.LastMessageTimestamp, 0)
	}
	var lastKey *waCommon.MessageKey
	if request.LastMessageId != "" {
		if request.LastMessageTimestamp == nil || request.LastMessageFromMe == nil {
			return appstate.PatchInfo{}, fmt.Errorf("LastMessageTimestamp and LastMessageFromMe are required with LastMessageId")
		}
		lastKey = &waCommon.MessageKey{
			RemoteJID: proto.String(chat.String()),
			FromMe:    proto.Bool(*request.LastMessageFromMe),
			ID:        proto.String(request.LastMessageId),
		}
		// In groups the key names the author of a message someone else sent.
		if chat.Server == types.GroupServer && !*request.LastMessageFromMe {
			sender, ok := parseJID(request.LastMessageSender)
			if !ok || sender.IsEmpty() || sender.Device != 0 || (sender.Server != types.DefaultUserServer && sender.Server != types.HiddenUserServer) {
				return appstate.PatchInfo{}, fmt.Errorf("LastMessageSender is required for messages others sent in groups")
			}
			lastKey.Participant = proto.String(sender.String())
		} else if request.LastMessageSender != "" {
			return appstate.PatchInfo{}, fmt.Errorf("LastMessageSender is only used for messages others sent in groups")
		}
	} else if request.LastMessageFromMe != nil || request.LastMessageSender != "" {
		return appstate.PatchInfo{}, fmt.Errorf("LastMessageId is required with LastMessageFromMe or LastMessageSender")
	}
	deleteMedia := request.DeleteMedia != nil && *request.DeleteMedia
	return appstate.BuildDeleteChat(chat, lastTimestamp, lastKey, deleteMedia), nil
}
