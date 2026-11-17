package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

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
		info := r.Context().Value("userinfo").(Values)
		s.recordRawMessage(info, recipient, msgid, msg, resp)
		s.publishSentMessageEvent(info.Get("Token"), txtid, txtid, recipient, msgid, msg, resp.Timestamp, rawMessageType(s, msg))
		data, _ := json.Marshal(map[string]interface{}{"Details": "Sent", "Id": msgid, "Timestamp": resp.Timestamp})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}

func decodeRawMessage(raw json.RawMessage) (*waE2E.Message, error) {
	var msg waE2E.Message
	if err := (protojson.UnmarshalOptions{RecursionLimit: 100}).Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("invalid Message: %v", err)
	}
	if isEmptyMessage(&msg) {
		return nil, errors.New("Message has no known content")
	}
	if err := validateRawFields(msg.ProtoReflect()); err != nil {
		return nil, err
	}
	return &msg, nil
}

func isEmptyMessage(m *waE2E.Message) bool {
	return !rawHasFields(m.ProtoReflect())
}

// Inspect all nested protobuf messages, including quoted messages and wrappers.
// Keep this a denylist so new experimental content types remain usable.
func validateRawFields(m protoreflect.Message) error {
	var invalid error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch fd.FullName() {
		case "WAWebProtobufsE2E.Message.protocolMessage",
			"WAWebProtobufsE2E.Message.senderKeyDistributionMessage",
			"WAWebProtobufsE2E.Message.fastRatchetKeySenderKeyDistributionMessage",
			"WAWebProtobufsE2E.Message.deviceSentMessage":
			invalid = fmt.Errorf("Message field %s is internal; use dedicated API operations where available", fd.Name())
			return false
		}
		if fd.Message() != nil && !fd.IsMap() {
			if fd.IsList() {
				list := v.List()
				for i := 0; i < list.Len() && invalid == nil; i++ {
					invalid = validateRawFields(list.Get(i).Message())
				}
			} else {
				invalid = validateRawFields(v.Message())
			}
		}
		return invalid == nil
	})
	return invalid
}

func rawMessageType(s *server, msg *waE2E.Message) string {
	msg = (&events.Message{RawMessage: msg}).UnwrapRaw().Message
	if action, ok := outgoingAction(msg); ok {
		return action.messageType
	}
	kind := s.getMessageType(msg)
	if kind == "text" && msg.Conversation == nil && msg.ExtendedTextMessage == nil {
		return "raw"
	}
	return kind
}

func (s *server) recordRawMessage(info Values, chat types.JID, id string, msg *waE2E.Message, resp whatsmeow.SendResponse) {
	if _, ok := outgoingAction(msg); ok {
		resp.ID = id
		s.saveOutgoingAction(info, chat, resp, msg)
		return
	}
	content := (&events.Message{RawMessage: msg}).UnwrapRaw().Message
	text := content.GetConversation()
	switch {
	case content.ExtendedTextMessage != nil:
		text = content.GetExtendedTextMessage().GetText()
	case content.ImageMessage != nil:
		text = content.GetImageMessage().GetCaption()
	case content.VideoMessage != nil:
		text = content.GetVideoMessage().GetCaption()
	case content.DocumentMessage != nil:
		text = content.GetDocumentMessage().GetCaption()
	case content.ContactMessage != nil:
		text = content.GetContactMessage().GetDisplayName()
	case content.LocationMessage != nil:
		text = content.GetLocationMessage().GetName()
	}
	limit, _ := strconv.Atoi(info.Get("History"))
	s.saveOutgoingMessageToHistory(info.Get("Id"), chat.String(), id, rawMessageType(s, msg), text, "", resp.Timestamp, limit, content)
}

// A wrapper or nested empty object alone is not message content.
func rawHasFields(m protoreflect.Message) bool {
	found := false
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Message() != nil && !fd.IsList() && !fd.IsMap() {
			found = rawHasFields(v.Message())
		} else {
			found = true
		}
		return !found
	})
	return found
}
