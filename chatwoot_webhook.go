package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

const (
	chatwootWebhookMaxBody = 1 << 20
	chatwootWebhookMaxAge  = 5 * time.Minute
)

type chatwootWebhookPayload struct {
	Event             string         `json:"event"`
	ID                int64          `json:"id"`
	MessageType       string         `json:"message_type"`
	Private           bool           `json:"private"`
	Content           string         `json:"content"`
	ContentAttributes map[string]any `json:"content_attributes"`
	Inbox             struct {
		ID int64 `json:"id"`
	} `json:"inbox"`
	Conversation struct {
		ID int64 `json:"id"`
	} `json:"conversation"`
}

type chatwootWebhookConfig struct {
	InstanceID    string `db:"instance_id"`
	InboxID       int64  `db:"inbox_id"`
	WebhookSecret []byte `db:"webhook_secret"`
}

func (s *server) ChatwootWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		if key == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var config chatwootWebhookConfig
		if err := s.db.Get(&config, s.db.Rebind(`SELECT instance_id, inbox_id, webhook_secret FROM chatwoot_config WHERE webhook_key=? AND enabled=?`), key, true); err != nil || len(config.WebhookSecret) == 0 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, chatwootWebhookMaxBody)
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		secret, err := decryptHMACKey(config.WebhookSecret)
		if err != nil || !validChatwootWebhookSignature(secret, rawBody, r.Header.Get("X-Chatwoot-Timestamp"), r.Header.Get("X-Chatwoot-Signature"), time.Now()) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var payload chatwootWebhookPayload
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if payload.Event != "message_created" || payload.ID <= 0 || payload.MessageType != "outgoing" || payload.Private || payload.Inbox.ID != config.InboxID || payload.Conversation.ID <= 0 || payload.Content == "" || payload.ContentAttributes["wuzapi_source"] == true {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var mapped bool
		if err := s.db.Get(&mapped, s.db.Rebind(`SELECT EXISTS(SELECT 1 FROM chatwoot_message_map WHERE instance_id=? AND chatwoot_message_id=?)`), config.InstanceID, payload.ID); err != nil || mapped {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var chatJID string
		if err := s.db.Get(&chatJID, s.db.Rebind(`SELECT chat_jid FROM chatwoot_contact_map WHERE instance_id=? AND conversation_id=?`), config.InstanceID, payload.Conversation.ID); err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := s.enqueueChatwootInbound(r.Context(), chatwootInboundMessage{
			InstanceID: config.InstanceID, ChatwootMessageID: payload.ID, ConversationID: payload.Conversation.ID, ChatJID: chatJID, Text: payload.Content,
		}); err != nil {
			http.Error(w, "queue unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func validChatwootWebhookSignature(secret string, rawBody []byte, timestamp, received string, now time.Time) bool {
	sentAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || timestamp == "" || absDuration(now.Sub(time.Unix(sentAt, 0))) > chatwootWebhookMaxAge {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(rawBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return len(expected) == len(received) && hmac.Equal([]byte(expected), []byte(received))
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
