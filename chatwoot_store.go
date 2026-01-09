package main

import (
	"database/sql"
	"errors"
	"fmt"
)

func (s *server) UpsertChatwootConfig(cfg *ChatwootConfig) error {
	if cfg == nil {
		return errors.New("chatwoot config is nil")
	}

	query := `
		INSERT INTO chatwoot_config (
			wuzapi_user_id,
			chatwoot_base_url,
			account_id,
			api_token,
			inbox_identifier,
			inbox_name,
			inbox_id,
			callback_secret,
			hmac_secret,
			enabled,
			sign_messages,
			signature_text,
			reopen_conversations,
			set_conversations_pending,
			ignore_groups,
			enable_typing_indicator,
			ignored_numbers,
			system_contact_identifier,
			system_conversation_id,
			created_at,
			updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
		ON CONFLICT (wuzapi_user_id) DO UPDATE SET
			chatwoot_base_url = excluded.chatwoot_base_url,
			account_id = excluded.account_id,
			api_token = excluded.api_token,
			inbox_identifier = excluded.inbox_identifier,
			inbox_name = excluded.inbox_name,
			inbox_id = excluded.inbox_id,
			callback_secret = excluded.callback_secret,
			hmac_secret = excluded.hmac_secret,
			enabled = excluded.enabled,
			sign_messages = excluded.sign_messages,
			signature_text = excluded.signature_text,
			reopen_conversations = excluded.reopen_conversations,
			set_conversations_pending = excluded.set_conversations_pending,
			ignore_groups = excluded.ignore_groups,
			enable_typing_indicator = excluded.enable_typing_indicator,
			ignored_numbers = excluded.ignored_numbers,
			system_contact_identifier = excluded.system_contact_identifier,
			system_conversation_id = excluded.system_conversation_id,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(
		s.db.Rebind(query),
		cfg.WuzapiUserID,
		cfg.ChatwootBaseURL,
		cfg.AccountID,
		cfg.APIToken,
		cfg.InboxIdentifier,
		cfg.InboxName,
		cfg.InboxID,
		cfg.CallbackSecret,
		cfg.HMACSecret,
		cfg.Enabled,
		cfg.SignMessages,
		cfg.SignatureText,
		cfg.ReopenConversations,
		cfg.SetConversationsPending,
		cfg.IgnoreGroups,
		cfg.EnableTypingIndicator,
		cfg.IgnoredNumbers,
		cfg.SystemContactIdentifier,
		cfg.SystemConversationID,
	)
	if err != nil {
		return fmt.Errorf("upsert chatwoot config: %w", err)
	}

	return nil
}

func (s *server) GetChatwootConfigByUserID(userID string) (*ChatwootConfig, error) {
	var cfg ChatwootConfig
	query := `SELECT * FROM chatwoot_config WHERE wuzapi_user_id = ? LIMIT 1`
	if err := s.db.Get(&cfg, s.db.Rebind(query), userID); err != nil {
		return nil, fmt.Errorf("get chatwoot config: %w", err)
	}
	return &cfg, nil
}

func (s *server) GetChatwootConfigByCallbackSecret(secret string) (*ChatwootConfig, error) {
	var cfg ChatwootConfig
	query := `SELECT * FROM chatwoot_config WHERE callback_secret = ? LIMIT 1`
	if err := s.db.Get(&cfg, s.db.Rebind(query), secret); err != nil {
		return nil, fmt.Errorf("get chatwoot config by callback secret: %w", err)
	}
	return &cfg, nil
}

func (s *server) GetChatwootConfigByInboxID(inboxID int) (*ChatwootConfig, error) {
	var cfg ChatwootConfig
	query := `SELECT * FROM chatwoot_config WHERE inbox_id = ? LIMIT 1`
	if err := s.db.Get(&cfg, s.db.Rebind(query), inboxID); err != nil {
		return nil, fmt.Errorf("get chatwoot config by inbox id: %w", err)
	}
	return &cfg, nil
}

func (s *server) UpsertChatwootMap(entry *ChatwootMap) error {
	if entry == nil {
		return errors.New("chatwoot map is nil")
	}

	query := `
		INSERT INTO chatwoot_map (
			wuzapi_user_id,
			wa_jid,
			wa_phone,
			chatwoot_contact_identifier,
			chatwoot_conversation_id,
			conversation_status,
			last_sync_at,
			created_at,
			updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP),
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
		ON CONFLICT (wuzapi_user_id, wa_jid) DO UPDATE SET
			wa_phone = excluded.wa_phone,
			chatwoot_contact_identifier = excluded.chatwoot_contact_identifier,
			chatwoot_conversation_id = excluded.chatwoot_conversation_id,
			conversation_status = excluded.conversation_status,
			last_sync_at = COALESCE(excluded.last_sync_at, CURRENT_TIMESTAMP),
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(
		s.db.Rebind(query),
		entry.WuzapiUserID,
		entry.WaJID,
		entry.WaPhone,
		entry.ChatwootContactIdentifier,
		entry.ChatwootConversationID,
		entry.ConversationStatus,
		entry.LastSyncAt,
	)
	if err != nil {
		return fmt.Errorf("upsert chatwoot map: %w", err)
	}

	return nil
}

func (s *server) GetChatwootMapByJID(userID, waJID string) (*ChatwootMap, error) {
	var entry ChatwootMap
	query := `SELECT * FROM chatwoot_map WHERE wuzapi_user_id = ? AND wa_jid = ? LIMIT 1`
	if err := s.db.Get(&entry, s.db.Rebind(query), userID, waJID); err != nil {
		return nil, fmt.Errorf("get chatwoot map by jid: %w", err)
	}
	return &entry, nil
}

func (s *server) GetChatwootMapByContactIdentifier(userID, contactIdentifier string) (*ChatwootMap, error) {
	var entry ChatwootMap
	query := `SELECT * FROM chatwoot_map WHERE wuzapi_user_id = ? AND chatwoot_contact_identifier = ? LIMIT 1`
	if err := s.db.Get(&entry, s.db.Rebind(query), userID, contactIdentifier); err != nil {
		return nil, fmt.Errorf("get chatwoot map by contact identifier: %w", err)
	}
	return &entry, nil
}

func (s *server) UpdateChatwootMapConversation(userID, waJID string, conversationID sql.NullInt64, status sql.NullString) error {
	query := `
		UPDATE chatwoot_map
		SET
			chatwoot_conversation_id = ?,
			conversation_status = ?,
			last_sync_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE wuzapi_user_id = ? AND wa_jid = ?
	`

	result, err := s.db.Exec(s.db.Rebind(query), conversationID, status, userID, waJID)
	if err != nil {
		return fmt.Errorf("update chatwoot conversation: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update chatwoot conversation rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("chatwoot map not found: %w", sql.ErrNoRows)
	}

	return nil
}
