package main

import (
	"database/sql"
	"time"
)

type ChatwootConfig struct {
	ID                      int           `db:"id"`
	WuzapiUserID            string        `db:"wuzapi_user_id"`
	ChatwootBaseURL         string        `db:"chatwoot_base_url"`
	AccountID               int           `db:"account_id"`
	APIToken                string        `db:"api_token"`
	InboxIdentifier         string        `db:"inbox_identifier"`
	InboxName               string        `db:"inbox_name"`
	InboxID                 int           `db:"inbox_id"`
	CallbackSecret          string        `db:"callback_secret"`
	HMACSecret              string        `db:"hmac_secret"`
	Enabled                 bool          `db:"enabled"`
	SignMessages            bool          `db:"sign_messages"`
	SignatureText           string        `db:"signature_text"`
	ReopenConversations     bool          `db:"reopen_conversations"`
	SetConversationsPending bool          `db:"set_conversations_pending"`
	IgnoreGroups            bool          `db:"ignore_groups"`
	EnableTypingIndicator   bool          `db:"enable_typing_indicator"`
	IgnoredNumbers          string        `db:"ignored_numbers"`
	SystemContactIdentifier string        `db:"system_contact_identifier"`
	SystemConversationID    sql.NullInt64 `db:"system_conversation_id"`
	CreatedAt               time.Time     `db:"created_at"`
	UpdatedAt               time.Time     `db:"updated_at"`
}

type ChatwootMap struct {
	ID                        int            `db:"id"`
	WuzapiUserID              string         `db:"wuzapi_user_id"`
	WaJID                     string         `db:"wa_jid"`
	WaPhone                   string         `db:"wa_phone"`
	ChatwootContactIdentifier string         `db:"chatwoot_contact_identifier"`
	ChatwootConversationID    sql.NullInt64  `db:"chatwoot_conversation_id"`
	ConversationStatus        sql.NullString `db:"conversation_status"`
	LastSyncAt                sql.NullTime   `db:"last_sync_at"`
	CreatedAt                 time.Time      `db:"created_at"`
	UpdatedAt                 time.Time      `db:"updated_at"`
}
