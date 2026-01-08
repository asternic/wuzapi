package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"wuzapi/internal/chatwoot"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types/events"
)

type chatwootInboundClient interface {
	CreateContact(ctx context.Context, inboxIdentifier string, payload chatwoot.CreateContactRequest) (string, error)
	CreateConversation(ctx context.Context, inboxIdentifier, contactIdentifier string, payload chatwoot.CreateConversationRequest) (int, error)
	CreateMessage(ctx context.Context, inboxIdentifier, contactIdentifier string, conversationID int, content string) (string, error)
	ToggleConversationStatus(ctx context.Context, conversationID int, status string) error
}

func (s *server) handleChatwootInbound(ctx context.Context, evt *events.Message, userID string) error {
	cfg, err := s.GetChatwootConfigByUserID(userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !cfg.Enabled {
		return nil
	}

	client, err := chatwoot.NewClient(cfg.ChatwootBaseURL, cfg.AccountID, cfg.APIToken)
	if err != nil {
		return err
	}

	return s.handleChatwootInboundWithClient(ctx, cfg, evt, userID, client)
}

func (s *server) handleChatwootInboundWithClient(ctx context.Context, cfg *ChatwootConfig, evt *events.Message, userID string, client chatwootInboundClient) error {
	if cfg == nil {
		return errors.New("chatwoot config is nil")
	}
	if evt == nil {
		return errors.New("event is nil")
	}
	if evt.Info.IsFromMe {
		return nil
	}
	if evt.Info.IsGroup {
		if cfg.IgnoreGroups {
			return nil
		}
		log.Info().Str("message_id", evt.Info.ID).Msg("Chatwoot inbound: group message ignored")
		return nil
	}

	contactJID := evt.Info.Sender
	if contactJID.User == "" {
		contactJID = evt.Info.Chat
	}
	if contactJID.User == "" {
		return errors.New("missing contact JID")
	}

	phone := normalizePhone(contactJID.User)
	if phone == "" {
		return errors.New("missing contact phone")
	}
	if isIgnoredNumber(phone, cfg.IgnoredNumbers) {
		return nil
	}

	text := extractTextMessage(evt)
	if text == "" {
		return nil
	}

	if cfg.InboxIdentifier == "" {
		return errors.New("chatwoot inbox identifier is required")
	}

	mapping, err := s.GetChatwootMapByJID(userID, contactJID.String())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	contactIdentifier := ""
	conversationID := 0
	conversationStatus := ""

	if mapping != nil {
		contactIdentifier = mapping.ChatwootContactIdentifier
		if mapping.ChatwootConversationID.Valid {
			conversationID = int(mapping.ChatwootConversationID.Int64)
		}
		if mapping.ConversationStatus.Valid {
			conversationStatus = mapping.ConversationStatus.String
		}
	}

	if contactIdentifier == "" {
		createdID, err := client.CreateContact(ctx, cfg.InboxIdentifier, chatwoot.CreateContactRequest{
			Identifier:  phone,
			PhoneNumber: "+" + phone,
			Name:        strings.TrimSpace(evt.Info.PushName),
		})
		if err != nil {
			return fmt.Errorf("create chatwoot contact: %w", err)
		}
		contactIdentifier = createdID
	}

	if conversationID != 0 && conversationStatus == "resolved" {
		if cfg.ReopenConversations {
			if err := client.ToggleConversationStatus(ctx, conversationID, "open"); err != nil {
				log.Warn().Err(err).Int("conversation_id", conversationID).Msg("Chatwoot inbound: failed to reopen conversation")
				conversationID = 0
			} else {
				conversationStatus = "open"
			}
		} else {
			conversationID = 0
		}
	}

	if conversationID == 0 {
		createdConvID, err := client.CreateConversation(ctx, cfg.InboxIdentifier, contactIdentifier, chatwoot.CreateConversationRequest{})
		if err != nil {
			return fmt.Errorf("create chatwoot conversation: %w", err)
		}
		conversationID = createdConvID
		conversationStatus = "open"
	}

	if err := s.UpsertChatwootMap(&ChatwootMap{
		WuzapiUserID:              userID,
		WaJID:                     contactJID.String(),
		WaPhone:                   phone,
		ChatwootContactIdentifier: contactIdentifier,
		ChatwootConversationID:    sql.NullInt64{Int64: int64(conversationID), Valid: conversationID > 0},
		ConversationStatus:        sql.NullString{String: conversationStatus, Valid: conversationStatus != ""},
	}); err != nil {
		return fmt.Errorf("upsert chatwoot map: %w", err)
	}

	messageID, err := client.CreateMessage(ctx, cfg.InboxIdentifier, contactIdentifier, conversationID, text)
	if err != nil {
		return fmt.Errorf("create chatwoot message: %w", err)
	}
	log.Info().Str("wa_message_id", evt.Info.ID).Str("chatwoot_message_id", messageID).Msg("Chatwoot inbound message created")

	if cfg.SetConversationsPending {
		if err := client.ToggleConversationStatus(ctx, conversationID, "pending"); err != nil {
			log.Warn().Err(err).Int("conversation_id", conversationID).Msg("Chatwoot inbound: failed to set conversation pending")
		} else {
			_ = s.UpdateChatwootMapConversation(userID, contactJID.String(), sql.NullInt64{Int64: int64(conversationID), Valid: true}, sql.NullString{String: "pending", Valid: true})
		}
	}

	return nil
}

func extractTextMessage(evt *events.Message) string {
	if evt == nil || evt.Message == nil {
		return ""
	}
	if conv := evt.Message.GetConversation(); conv != "" {
		return conv
	}
	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return ""
}

func normalizePhone(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isIgnoredNumber(phone, raw string) bool {
	ignored := parseIgnoredNumbers(raw)
	if len(ignored) == 0 {
		return false
	}
	_, ok := ignored[phone]
	return ok
}

func parseIgnoredNumbers(raw string) map[string]struct{} {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		values = strings.FieldsFunc(trimmed, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
		})
	}

	result := make(map[string]struct{})
	for _, value := range values {
		clean := normalizePhone(value)
		if clean == "" {
			continue
		}
		result[clean] = struct{}{}
	}
	return result
}
