package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// SendRawMessage: sends a waE2E.Message given as JSON, using the same
// field names whatsmeow uses when it serializes events (e.g. productMessage,
// eventInviteMessage, interactiveMessage). Lets us re-send message types that
// WuzAPI has no dedicated endpoint for (products, event invites, flows).
func (s *server) SendRawMessage() http.HandlerFunc {
	type rawRequest struct {
		Phone   string          `json:"Phone"`
		Message json.RawMessage `json:"Message"`
		Id      string          `json:"Id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		cli := clientManager.GetWhatsmeowClient(txtid)
		if cli == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}
		var req rawRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}
		if req.Phone == "" || len(req.Message) == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Phone and Message are required"))
			return
		}
		msg, err := decodeRawMessage(req.Message)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		recipient, err := validateMessageFields(r.Context(), cli, req.Phone, nil, nil)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		msgid := req.Id
		if msgid == "" {
			msgid = cli.GenerateMessageID()
		}
		resp, err := sendMessageWithRecovery(txtid, cli, context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to send raw message: %v", err))
			return
		}
		data, _ := json.Marshal(map[string]interface{}{"Details": "Sent", "Id": msgid, "Timestamp": resp.Timestamp})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}

func decodeRawMessage(raw json.RawMessage) (*waE2E.Message, error) {
	var msg waE2E.Message
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("invalid Message: %v", err)
	}
	if isEmptyMessage(&msg) {
		return nil, errors.New("Message has no known content")
	}
	return &msg, nil
}

func isEmptyMessage(m *waE2E.Message) bool {
	empty := true
	m.ProtoReflect().Range(func(_ protoreflect.FieldDescriptor, _ protoreflect.Value) bool { empty = false; return false })
	return empty
}
