package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type pollVoteRequest struct {
	Chat    string
	Sender  string
	Id      string
	FromMe  *bool
	Options *[]string
}

func (r pollVoteRequest) messageInfo() (*types.MessageInfo, error) {
	chat, err := apiChatJID(r.Chat)
	if err != nil {
		return nil, err
	}
	sender, err := apiChatJID(r.Sender)
	if err != nil || sender.Server == types.GroupServer {
		return nil, fmt.Errorf("Sender must identify the poll creator")
	}
	if strings.TrimSpace(r.Id) == "" || r.FromMe == nil || r.Options == nil {
		return nil, fmt.Errorf("Id, FromMe and Options are required")
	}
	seen := map[string]bool{}
	for _, option := range *r.Options {
		if strings.TrimSpace(option) == "" || seen[option] {
			return nil, fmt.Errorf("Options must contain distinct nonblank option names")
		}
		seen[option] = true
	}
	return &types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsGroup: chat.Server == types.GroupServer, IsFromMe: *r.FromMe}, ID: r.Id}, nil
}

type pollVoteExecutor func(context.Context, string, *types.MessageInfo, []string) (whatsmeow.SendResponse, error)

func sendAPIPollVote(ctx context.Context, user string, info *types.MessageInfo, options []string) (whatsmeow.SendResponse, error) {
	client := clientManager.GetWhatsmeowClient(user)
	if client == nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("no session")
	}
	message, err := client.BuildPollVote(ctx, info, options)
	if err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("could not build poll vote: %w", err)
	}
	return sendMessageWithRecovery(user, client, ctx, info.Chat, message)
}

func (s *server) pollVoteHandler(send pollVoteExecutor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request pollVoteRequest
		if err := decodeAPIRequest(r.Body, &request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		info, err := request.messageInfo()
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		response, err := send(r.Context(), r.Context().Value("userinfo").(Values).Get("Id"), info, *request.Options)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		data, _ := json.Marshal(map[string]any{"Id": response.ID, "Timestamp": response.Timestamp.Unix(), "Details": "Sent"})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}
