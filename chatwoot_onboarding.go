package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"wuzapi/internal/chatwoot"
)

const (
	chatwootSystemContactName  = "Flownix"
	chatwootSystemContactEmail = "contato@flownix.com.br"
	chatwootOnboardingMessage  = "Bem-vindo ao WuzAPI! Para conectar seu WhatsApp, envie #qrcode aqui."
)

func (s *server) ensureChatwootOnboarding(ctx context.Context, cfg *ChatwootConfig) error {
	if cfg == nil {
		return errors.New("chatwoot config is nil")
	}
	if strings.TrimSpace(cfg.ChatwootBaseURL) == "" {
		return errors.New("chatwoot base url is required")
	}
	if strings.TrimSpace(cfg.InboxIdentifier) == "" {
		return errors.New("chatwoot inbox identifier is required")
	}

	client, err := newChatwootClient(cfg.ChatwootBaseURL, cfg.AccountID, cfg.APIToken)
	if err != nil {
		return fmt.Errorf("create chatwoot client: %w", err)
	}

	contactIdentifier := strings.TrimSpace(cfg.SystemContactIdentifier)
	if contactIdentifier == "" {
		identifier := buildChatwootSystemIdentifier(cfg)
		payload := chatwoot.CreateContactRequest{
			Identifier: identifier,
			Name:       chatwootSystemContactName,
			Email:      chatwootSystemContactEmail,
		}
		if cfg.HMACSecret != "" {
			payload.IdentifierHash = chatwoot.IdentifierHash(identifier, cfg.HMACSecret)
		}

		contactIdentifier, err = client.CreateContact(ctx, cfg.InboxIdentifier, payload)
		if err != nil {
			return fmt.Errorf("create chatwoot system contact: %w", err)
		}
		cfg.SystemContactIdentifier = contactIdentifier
	}

	conversationID := 0
	if cfg.SystemConversationID.Valid && cfg.SystemConversationID.Int64 > 0 {
		conversationID = int(cfg.SystemConversationID.Int64)
	}
	if conversationID == 0 {
		conversationID, err = client.CreateConversation(ctx, cfg.InboxIdentifier, contactIdentifier, chatwoot.CreateConversationRequest{})
		if err != nil {
			return fmt.Errorf("create chatwoot system conversation: %w", err)
		}
		cfg.SystemConversationID = sql.NullInt64{Int64: int64(conversationID), Valid: true}
	}

	if err := s.UpsertChatwootConfig(cfg); err != nil {
		return fmt.Errorf("update chatwoot system contact: %w", err)
	}

	if _, err := client.CreateMessage(ctx, cfg.InboxIdentifier, contactIdentifier, conversationID, chatwootOnboardingMessage); err != nil {
		return fmt.Errorf("create chatwoot onboarding message: %w", err)
	}

	return nil
}

func buildChatwootSystemIdentifier(cfg *ChatwootConfig) string {
	seed := strings.TrimSpace(cfg.InboxIdentifier)
	if seed == "" {
		seed = strings.TrimSpace(cfg.WuzapiUserID)
	}
	if seed == "" {
		seed = "default"
	}
	return fmt.Sprintf("flownix-%s", seed)
}
