package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type chatwootSendFunc func(ctx context.Context, userID, waJID, content string) (string, error)

var (
	errChatwootMissingToken = errors.New("missing chatwoot callback token")
	errChatwootInvalidToken = errors.New("invalid chatwoot callback token")
)

func (s *server) ChatwootCallback() http.HandlerFunc {
	return s.chatwootCallbackHandler(s.sendChatwootWhatsAppText)
}

func (s *server) chatwootCallbackHandler(send chatwootSendFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		if token == "" {
			token = strings.TrimSpace(r.Header.Get("X-Chatwoot-Token"))
		}
		if token == "" {
			s.Respond(w, r, http.StatusUnauthorized, errChatwootMissingToken)
			return
		}

		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			s.Respond(w, r, http.StatusUnsupportedMediaType, errors.New("content-type must be application/json"))
			return
		}

		var payload chatwootCallbackPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid json payload"))
			return
		}

		cfg, err := s.resolveChatwootCallbackConfig(token, &payload)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, errChatwootMissingToken):
				status = http.StatusUnauthorized
			case errors.Is(err, errChatwootInvalidToken):
				status = http.StatusForbidden
			case errors.Is(err, sql.ErrNoRows):
				status = http.StatusNotFound
			}
			s.Respond(w, r, status, err)
			return
		}

		if !cfg.Enabled {
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}

		event := strings.TrimSpace(payload.Event)
		if event == "conversation_typing_on" || event == "conversation_typing_off" {
			if !cfg.EnableTypingIndicator {
				s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
				return
			}

			presence := types.ChatPresenceComposing
			if event == "conversation_typing_off" {
				presence = types.ChatPresencePaused
			}

			sent, err := s.handleChatwootTypingEvent(r.Context(), cfg, &payload, presence)
			if err != nil {
				log.Warn().Err(err).Msg("Chatwoot typing: failed to send presence")
				s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "typing_failed"})
				return
			}

			status := "typing_ignored"
			if sent {
				status = "typing_sent"
			}
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": status})
			return
		}

		if event != "message_created" {
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}

		msg := payload.effectiveMessage()
		if msg.Private {
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		if !msg.MessageType.IsOutgoing() {
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		if strings.TrimSpace(msg.Content) == "" {
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}

		contactIdentifier := payload.contactIdentifier()
		if contactIdentifier == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing contact identifier"))
			return
		}

		if cmd, ok := chatwootCommandFromContent(msg.Content); ok {
			if contactIdentifier == cfg.SystemContactIdentifier || cmd == "#attid" {
				if err := s.handleChatwootCommand(r.Context(), cfg, &payload, cmd); err != nil {
					log.Error().Err(err).Str("command", cmd).Msg("Chatwoot callback: command handling failed")
					s.Respond(w, r, http.StatusInternalServerError, err)
					return
				}
				s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "command_sent"})
				return
			}
		}

		if cfg.SystemContactIdentifier != "" && contactIdentifier == cfg.SystemContactIdentifier {
			log.Info().Msg("Chatwoot callback: system contact message ignored")
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}

		mapping, err := s.GetChatwootMapByContactIdentifier(cfg.WuzapiUserID, contactIdentifier)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				s.Respond(w, r, http.StatusNotFound, errors.New("chatwoot contact mapping not found"))
				return
			}
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		if mapping.WaJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing whatsapp jid"))
			return
		}

		phone := mapping.WaPhone
		if phone == "" {
			phone = normalizePhone(mapping.WaJID)
		}
		if phone != "" && isIgnoredNumber(phone, cfg.IgnoredNumbers) {
			s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}

		content := applyChatwootSignature(msg.Content, cfg)
		messageID, err := send(r.Context(), cfg.WuzapiUserID, mapping.WaJID, content)
		if err != nil {
			log.Error().Err(err).Str("wa_jid", mapping.WaJID).Msg("Chatwoot callback: failed to send WhatsApp message")
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		s.respondChatwootCallback(w, r, http.StatusOK, map[string]string{"status": "sent", "message_id": messageID})
	}
}

func (s *server) resolveChatwootCallbackConfig(token string, payload *chatwootCallbackPayload) (*ChatwootConfig, error) {
	if token == "" {
		return nil, errChatwootMissingToken
	}

	cfg, err := s.GetChatwootConfigByCallbackSecret(token)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	inboxID := payload.inboxID()
	if inboxID == 0 {
		return nil, errChatwootInvalidToken
	}

	cfg, err = s.GetChatwootConfigByInboxID(inboxID)
	if err != nil {
		return nil, err
	}
	if cfg.CallbackSecret != token {
		return nil, errChatwootInvalidToken
	}
	return cfg, nil
}

func (s *server) respondChatwootCallback(w http.ResponseWriter, r *http.Request, status int, payload map[string]string) {
	body, err := json.Marshal(payload)
	if err != nil {
		s.Respond(w, r, http.StatusInternalServerError, err)
		return
	}
	s.Respond(w, r, status, string(body))
}

func (s *server) sendChatwootWhatsAppText(ctx context.Context, userID, waJID, content string) (string, error) {
	client := clientManager.GetWhatsmeowClient(userID)
	if client == nil {
		return "", errors.New("no session")
	}

	recipient, err := validateMessageFields(waJID, nil, nil)
	if err != nil {
		return "", err
	}

	msgID := client.GenerateMessageID()
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(content),
		},
	}

	resp, err := client.SendMessage(ctx, recipient, msg, whatsmeow.SendRequestExtra{ID: msgID})
	if err != nil {
		return "", err
	}

	historyLimit := s.getUserHistoryLimit(userID)
	s.saveOutgoingMessageToHistory(userID, recipient.String(), msgID, "text", content, "", historyLimit)

	log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgID).Msg("Chatwoot outbound message sent")
	return msgID, nil
}

func (s *server) getUserHistoryLimit(userID string) int {
	var history sql.NullInt64
	query := `SELECT COALESCE(history, 0) FROM users WHERE id = ?`
	if err := s.db.Get(&history, s.db.Rebind(query), userID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Warn().Err(err).Str("user_id", userID).Msg("Chatwoot callback: failed to load history limit")
		}
		return 0
	}
	if !history.Valid {
		return 0
	}
	return int(history.Int64)
}

func applyChatwootSignature(content string, cfg *ChatwootConfig) string {
	if cfg == nil || !cfg.SignMessages {
		return content
	}
	signature := strings.TrimSpace(cfg.SignatureText)
	if signature == "" {
		return content
	}
	if strings.HasSuffix(content, "\n") {
		return content + signature
	}
	return content + "\n" + signature
}

func chatwootCommandFromContent(content string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", false
	}
	command := strings.ToLower(fields[0])
	switch command {
	case "#qrcode", "#help", "#status", "#disconnect", "#attid", "#updateavatar":
		return command, true
	default:
		return "", false
	}
}

type chatwootCallbackPayload struct {
	Event        string                       `json:"event"`
	MessageType  chatwootMessageType          `json:"message_type"`
	Private      bool                         `json:"private"`
	IsPrivate    bool                         `json:"is_private"`
	Content      string                       `json:"content"`
	Conversation chatwootCallbackConversation `json:"conversation"`
	Inbox        chatwootCallbackInbox        `json:"inbox"`
	Message      *chatwootCallbackMessage     `json:"message"`
}

func (p *chatwootCallbackPayload) inboxID() int {
	if p == nil {
		return 0
	}
	if p.Inbox.ID != 0 {
		return p.Inbox.ID
	}
	if p.Conversation.InboxID != 0 {
		return p.Conversation.InboxID
	}
	return 0
}

func (p *chatwootCallbackPayload) contactIdentifier() string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.Conversation.ContactInbox.SourceID)
}

func (p *chatwootCallbackPayload) effectiveMessage() chatwootCallbackMessage {
	msg := chatwootCallbackMessage{
		MessageType: p.MessageType,
		Private:     p.Private,
		Content:     p.Content,
	}
	if p.Message == nil {
		return msg
	}
	if p.Message.Content != "" {
		msg.Content = p.Message.Content
	}
	if !p.Message.MessageType.IsZero() {
		msg.MessageType = p.Message.MessageType
	}
	if p.Message.Private {
		msg.Private = true
	}
	return msg
}

type chatwootCallbackMessage struct {
	MessageType chatwootMessageType `json:"message_type"`
	Private     bool                `json:"private"`
	Content     string              `json:"content"`
}

type chatwootCallbackConversation struct {
	ID           int                          `json:"id"`
	InboxID      int                          `json:"inbox_id"`
	ContactInbox chatwootCallbackContactInbox `json:"contact_inbox"`
}

type chatwootCallbackContactInbox struct {
	SourceID string `json:"source_id"`
}

type chatwootCallbackInbox struct {
	ID int `json:"id"`
}

type chatwootMessageType struct {
	rawString string
	rawInt    *int
}

func (t *chatwootMessageType) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		t.rawString = str
		return nil
	}
	var num int
	if err := json.Unmarshal(data, &num); err == nil {
		t.rawInt = &num
		return nil
	}
	return fmt.Errorf("invalid message_type")
}

func (t chatwootMessageType) IsOutgoing() bool {
	if t.rawString != "" {
		return strings.EqualFold(strings.TrimSpace(t.rawString), "outgoing")
	}
	if t.rawInt != nil {
		return *t.rawInt == 1
	}
	return false
}

func (t chatwootMessageType) IsZero() bool {
	return t.rawString == "" && t.rawInt == nil
}
