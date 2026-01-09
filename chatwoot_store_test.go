package main

import (
	"database/sql"
	"errors"
	"testing"
)

func TestChatwootConfigUpsertAndGet(t *testing.T) {
	s := makeTestServer(t)

	cfg := &ChatwootConfig{
		WuzapiUserID:            "user-1",
		ChatwootBaseURL:         "https://chat.example",
		AccountID:               12,
		APIToken:                "token-abc",
		InboxIdentifier:         "inbox-identifier",
		InboxName:               "Inbox A",
		InboxID:                 99,
		CallbackSecret:          "secret-123",
		HMACSecret:              "hmac-secret",
		Enabled:                 true,
		SignMessages:            true,
		SignatureText:           "--sig",
		ReopenConversations:     true,
		SetConversationsPending: true,
		IgnoreGroups:            true,
		EnableTypingIndicator:   true,
		IgnoredNumbers:          "[\"5511999999999\"]",
		SystemContactIdentifier: "system-contact",
		SystemConversationID:    sql.NullInt64{Int64: 123, Valid: true},
	}

	if err := s.UpsertChatwootConfig(cfg); err != nil {
		t.Fatalf("upsert chatwoot config: %v", err)
	}

	got, err := s.GetChatwootConfigByUserID(cfg.WuzapiUserID)
	if err != nil {
		t.Fatalf("get chatwoot config: %v", err)
	}

	if got.ChatwootBaseURL != cfg.ChatwootBaseURL {
		t.Fatalf("expected base url %q, got %q", cfg.ChatwootBaseURL, got.ChatwootBaseURL)
	}
	if got.InboxIdentifier != cfg.InboxIdentifier {
		t.Fatalf("expected inbox identifier %q, got %q", cfg.InboxIdentifier, got.InboxIdentifier)
	}
	if got.SignMessages != cfg.SignMessages {
		t.Fatalf("expected sign_messages %v, got %v", cfg.SignMessages, got.SignMessages)
	}
	if got.SystemConversationID.Int64 != cfg.SystemConversationID.Int64 || got.SystemConversationID.Valid != cfg.SystemConversationID.Valid {
		t.Fatalf("expected system conversation id %v, got %v", cfg.SystemConversationID, got.SystemConversationID)
	}

	cfg.SignMessages = false
	cfg.SignatureText = "updated"
	cfg.InboxName = "Inbox B"
	if err := s.UpsertChatwootConfig(cfg); err != nil {
		t.Fatalf("upsert chatwoot config (update): %v", err)
	}

	updated, err := s.GetChatwootConfigByUserID(cfg.WuzapiUserID)
	if err != nil {
		t.Fatalf("get chatwoot config after update: %v", err)
	}
	if updated.SignMessages != cfg.SignMessages {
		t.Fatalf("expected sign_messages %v, got %v", cfg.SignMessages, updated.SignMessages)
	}
	if updated.SignatureText != cfg.SignatureText {
		t.Fatalf("expected signature_text %q, got %q", cfg.SignatureText, updated.SignatureText)
	}
	if updated.InboxName != cfg.InboxName {
		t.Fatalf("expected inbox name %q, got %q", cfg.InboxName, updated.InboxName)
	}
}

func TestChatwootConfigLookupBySecretAndInboxID(t *testing.T) {
	s := makeTestServer(t)

	cfg := &ChatwootConfig{
		WuzapiUserID:    "user-2",
		ChatwootBaseURL: "https://chat.example",
		AccountID:       44,
		APIToken:        "token-def",
		InboxIdentifier: "inbox-identifier-2",
		InboxName:       "Inbox C",
		InboxID:         123,
		CallbackSecret:  "secret-xyz",
		Enabled:         true,
	}

	if err := s.UpsertChatwootConfig(cfg); err != nil {
		t.Fatalf("upsert chatwoot config: %v", err)
	}

	bySecret, err := s.GetChatwootConfigByCallbackSecret(cfg.CallbackSecret)
	if err != nil {
		t.Fatalf("get chatwoot config by callback secret: %v", err)
	}
	if bySecret.WuzapiUserID != cfg.WuzapiUserID {
		t.Fatalf("expected user id %q, got %q", cfg.WuzapiUserID, bySecret.WuzapiUserID)
	}

	byInbox, err := s.GetChatwootConfigByInboxID(cfg.InboxID)
	if err != nil {
		t.Fatalf("get chatwoot config by inbox id: %v", err)
	}
	if byInbox.CallbackSecret != cfg.CallbackSecret {
		t.Fatalf("expected callback secret %q, got %q", cfg.CallbackSecret, byInbox.CallbackSecret)
	}

	if _, err := s.GetChatwootConfigByCallbackSecret("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
	if _, err := s.GetChatwootConfigByInboxID(999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestChatwootMapUpsertAndGet(t *testing.T) {
	s := makeTestServer(t)

	entry := &ChatwootMap{
		WuzapiUserID:              "user-1",
		WaJID:                     "5511999999999@s.whatsapp.net",
		WaPhone:                   "5511999999999",
		ChatwootContactIdentifier: "contact-1",
	}

	if err := s.UpsertChatwootMap(entry); err != nil {
		t.Fatalf("upsert chatwoot map: %v", err)
	}

	got, err := s.GetChatwootMapByJID(entry.WuzapiUserID, entry.WaJID)
	if err != nil {
		t.Fatalf("get chatwoot map by jid: %v", err)
	}
	if got.ChatwootContactIdentifier != entry.ChatwootContactIdentifier {
		t.Fatalf("expected contact identifier %q, got %q", entry.ChatwootContactIdentifier, got.ChatwootContactIdentifier)
	}
	if got.WaPhone != entry.WaPhone {
		t.Fatalf("expected wa_phone %q, got %q", entry.WaPhone, got.WaPhone)
	}

	entry.ChatwootContactIdentifier = "contact-2"
	entry.ChatwootConversationID = sql.NullInt64{Int64: 456, Valid: true}
	entry.ConversationStatus = sql.NullString{String: "open", Valid: true}
	if err := s.UpsertChatwootMap(entry); err != nil {
		t.Fatalf("upsert chatwoot map (update): %v", err)
	}

	byContact, err := s.GetChatwootMapByContactIdentifier(entry.WuzapiUserID, entry.ChatwootContactIdentifier)
	if err != nil {
		t.Fatalf("get chatwoot map by contact identifier: %v", err)
	}
	if byContact.ChatwootConversationID.Int64 != entry.ChatwootConversationID.Int64 || !byContact.ChatwootConversationID.Valid {
		t.Fatalf("expected conversation id %v, got %v", entry.ChatwootConversationID, byContact.ChatwootConversationID)
	}
	if byContact.ConversationStatus.String != entry.ConversationStatus.String {
		t.Fatalf("expected conversation status %q, got %q", entry.ConversationStatus.String, byContact.ConversationStatus.String)
	}
}

func TestChatwootMapUpdateConversation(t *testing.T) {
	s := makeTestServer(t)

	entry := &ChatwootMap{
		WuzapiUserID:              "user-1",
		WaJID:                     "5511888888888@s.whatsapp.net",
		WaPhone:                   "5511888888888",
		ChatwootContactIdentifier: "contact-3",
	}

	if err := s.UpsertChatwootMap(entry); err != nil {
		t.Fatalf("upsert chatwoot map: %v", err)
	}

	conversationID := sql.NullInt64{Int64: 789, Valid: true}
	status := sql.NullString{String: "resolved", Valid: true}
	if err := s.UpdateChatwootMapConversation(entry.WuzapiUserID, entry.WaJID, conversationID, status); err != nil {
		t.Fatalf("update chatwoot map conversation: %v", err)
	}

	updated, err := s.GetChatwootMapByJID(entry.WuzapiUserID, entry.WaJID)
	if err != nil {
		t.Fatalf("get chatwoot map after update: %v", err)
	}
	if updated.ChatwootConversationID.Int64 != conversationID.Int64 || !updated.ChatwootConversationID.Valid {
		t.Fatalf("expected conversation id %v, got %v", conversationID, updated.ChatwootConversationID)
	}
	if updated.ConversationStatus.String != status.String {
		t.Fatalf("expected conversation status %q, got %q", status.String, updated.ConversationStatus.String)
	}

	if err := s.UpdateChatwootMapConversation("missing-user", "missing-jid", conversationID, status); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
