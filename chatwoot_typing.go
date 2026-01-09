package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

var sendChatwootTypingPresence = sendWhatsAppChatPresence

func (s *server) handleChatwootTypingEvent(ctx context.Context, cfg *ChatwootConfig, payload *chatwootCallbackPayload, presence types.ChatPresence) (bool, error) {
	if cfg == nil || payload == nil {
		return false, errors.New("missing chatwoot context")
	}
	if payload.IsPrivate || payload.Private {
		return false, nil
	}

	contactIdentifier := payload.contactIdentifier()
	if contactIdentifier == "" {
		return false, errors.New("missing contact identifier")
	}
	if contactIdentifier == cfg.SystemContactIdentifier {
		return false, nil
	}

	mapping, err := s.GetChatwootMapByContactIdentifier(cfg.WuzapiUserID, contactIdentifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(mapping.WaJID) == "" {
		return false, errors.New("missing whatsapp jid")
	}

	phone := mapping.WaPhone
	if phone == "" {
		phone = normalizePhone(mapping.WaJID)
	}
	if phone != "" && isIgnoredNumber(phone, cfg.IgnoredNumbers) {
		return false, nil
	}

	if err := sendChatwootTypingPresence(ctx, cfg.WuzapiUserID, mapping.WaJID, presence); err != nil {
		return false, err
	}
	return true, nil
}

func sendWhatsAppChatPresence(ctx context.Context, userID, waJID string, presence types.ChatPresence) error {
	client := clientManager.GetWhatsmeowClient(userID)
	if client == nil {
		return errChatwootNoSession
	}
	if !client.IsConnected() {
		return errChatwootNotConnected
	}

	if strings.TrimSpace(waJID) == "" {
		return errors.New("missing whatsapp jid")
	}
	jid, ok := parseJID(waJID)
	if !ok {
		return errors.New("invalid whatsapp jid")
	}
	return client.SendChatPresence(ctx, jid, presence, types.ChatPresenceMediaText)
}
