package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

const chatwootFoundationSQL = `
CREATE TABLE IF NOT EXISTS chatwoot_config (
    instance_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    url TEXT NOT NULL DEFAULT '',
    account_id BIGINT NOT NULL DEFAULT 0,
    token BYTEA,
    inbox_id BIGINT NOT NULL DEFAULT 0,
    inbox_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS chatwoot_outbox (
    id BIGSERIAL PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    direction TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending',
    lock_key TEXT NOT NULL DEFAULT '',
    source_message_id TEXT NOT NULL DEFAULT '',
    payload BYTEA NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    locked_at TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_chatwoot_outbox_pending
    ON chatwoot_outbox (state, next_attempt_at, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_chatwoot_outbox_source
    ON chatwoot_outbox (instance_id, direction, source_message_id);
CREATE TABLE IF NOT EXISTS chatwoot_outbox_locks (
    lock_key TEXT PRIMARY KEY,
    outbox_id BIGINT NOT NULL UNIQUE REFERENCES chatwoot_outbox(id) ON DELETE CASCADE,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS chatwoot_message_map (
    instance_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    wa_message_id TEXT NOT NULL DEFAULT '',
    chatwoot_message_id BIGINT,
    conversation_id BIGINT,
    direction TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (instance_id, wa_message_id),
    UNIQUE (instance_id, chatwoot_message_id)
);
CREATE TABLE IF NOT EXISTS chatwoot_contact_map (
    instance_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_jid TEXT NOT NULL,
    contact_id BIGINT NOT NULL,
    source_id TEXT NOT NULL,
    conversation_id BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (instance_id, chat_jid)
);
`

func initializeChatwootSchema(db *sqlx.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chatwoot_schema_migrations (
        id INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	var applied bool
	if err := db.Get(&applied, db.Rebind(`SELECT EXISTS(SELECT 1 FROM chatwoot_schema_migrations WHERE id=?)`), 1); err != nil {
		return err
	}
	if applied {
		return nil
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if db.DriverName() == "sqlite" {
		err = applyChatwootSQLiteMigration(tx)
	} else {
		_, err = tx.Exec(chatwootFoundationSQL)
	}
	if err != nil {
		return fmt.Errorf("apply foundation: %w", err)
	}
	if _, err := tx.Exec(db.Rebind(`INSERT INTO chatwoot_schema_migrations (id, name) VALUES (?, ?)`), 1, "foundation"); err != nil {
		return err
	}
	return tx.Commit()
}

func applyChatwootSQLiteMigration(tx *sqlx.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS chatwoot_config (
            instance_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
            enabled BOOLEAN NOT NULL DEFAULT 0, url TEXT NOT NULL DEFAULT '',
            account_id INTEGER NOT NULL DEFAULT 0, token BLOB, inbox_id INTEGER NOT NULL DEFAULT 0,
            inbox_name TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS chatwoot_outbox (
            id INTEGER PRIMARY KEY AUTOINCREMENT, instance_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            direction TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'pending', lock_key TEXT NOT NULL DEFAULT '',
            source_message_id TEXT NOT NULL DEFAULT '',
            payload BLOB NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
            locked_at DATETIME,
            next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, last_error TEXT NOT NULL DEFAULT '',
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS idx_chatwoot_outbox_pending ON chatwoot_outbox (state, next_attempt_at, id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_chatwoot_outbox_source ON chatwoot_outbox (instance_id, direction, source_message_id)`,
		`CREATE TABLE IF NOT EXISTS chatwoot_outbox_locks (
            lock_key TEXT PRIMARY KEY, outbox_id INTEGER NOT NULL UNIQUE REFERENCES chatwoot_outbox(id) ON DELETE CASCADE,
            acquired_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS chatwoot_message_map (
            instance_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, wa_message_id TEXT NOT NULL DEFAULT '',
            chatwoot_message_id INTEGER, conversation_id INTEGER,
            direction TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (instance_id, wa_message_id), UNIQUE (instance_id, chatwoot_message_id))`,
		`CREATE TABLE IF NOT EXISTS chatwoot_contact_map (
            instance_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, chat_jid TEXT NOT NULL,
            contact_id INTEGER NOT NULL, source_id TEXT NOT NULL, conversation_id INTEGER NOT NULL DEFAULT 0,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (instance_id, chat_jid))`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}
