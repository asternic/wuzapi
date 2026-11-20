package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type chatwootMigration struct {
	ID   int
	Name string
}

var chatwootMigrations = []chatwootMigration{
	{ID: 1, Name: "foundation"},
	{ID: 2, Name: "webhook_credentials"},
	{ID: 3, Name: "attachment_hosts"},
	{ID: 4, Name: "attachment_message_map"},
}

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
	var applied map[int]bool = make(map[int]bool)
	var rows []struct {
		ID int `db:"id"`
	}
	if err := db.Select(&rows, `SELECT id FROM chatwoot_schema_migrations`); err != nil {
		return err
	}
	for _, row := range rows {
		applied[row.ID] = true
	}
	for _, migration := range chatwootMigrations {
		if applied[migration.ID] {
			continue
		}
		if err := applyChatwootMigration(db, migration); err != nil {
			return fmt.Errorf("apply Chatwoot migration %d: %w", migration.ID, err)
		}
	}
	return nil
}

func applyChatwootMigration(db *sqlx.DB, migration chatwootMigration) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	switch migration.ID {
	case 1:
		if db.DriverName() == "sqlite" {
			err = applyChatwootSQLiteMigration(tx)
		} else {
			_, err = tx.Exec(chatwootFoundationSQL)
		}
	case 2:
		err = applyChatwootWebhookCredentialsMigration(db.DriverName(), tx)
	case 3:
		err = applyChatwootAttachmentHostsMigration(db.DriverName(), tx)
	case 4:
		err = applyChatwootAttachmentMessageMapMigration(db.DriverName(), tx)
	default:
		return fmt.Errorf("unknown migration")
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(db.Rebind(`INSERT INTO chatwoot_schema_migrations (id, name) VALUES (?, ?)`), migration.ID, migration.Name); err != nil {
		return err
	}
	return tx.Commit()
}

func applyChatwootAttachmentMessageMapMigration(driver string, tx *sqlx.Tx) error {
	if driver == "sqlite" {
		if _, err := tx.Exec(`ALTER TABLE chatwoot_message_map RENAME TO chatwoot_message_map_old;
            CREATE TABLE chatwoot_message_map (
                instance_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                wa_message_id TEXT NOT NULL DEFAULT '', chatwoot_message_id INTEGER,
                chatwoot_attachment_index INTEGER NOT NULL DEFAULT 0, conversation_id INTEGER,
                direction TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                PRIMARY KEY (instance_id, wa_message_id),
                UNIQUE (instance_id, chatwoot_message_id, chatwoot_attachment_index));
            INSERT INTO chatwoot_message_map (instance_id, wa_message_id, chatwoot_message_id, chatwoot_attachment_index, conversation_id, direction, created_at)
                SELECT instance_id, wa_message_id, chatwoot_message_id, 0, conversation_id, direction, created_at FROM chatwoot_message_map_old;
            DROP TABLE chatwoot_message_map_old;`); err != nil {
			return err
		}
		return nil
	}
	_, err := tx.Exec(`ALTER TABLE chatwoot_message_map ADD COLUMN IF NOT EXISTS chatwoot_attachment_index INTEGER NOT NULL DEFAULT 0;
        ALTER TABLE chatwoot_message_map DROP CONSTRAINT IF EXISTS chatwoot_message_map_instance_id_chatwoot_message_id_key;
        CREATE UNIQUE INDEX IF NOT EXISTS idx_chatwoot_message_map_chatwoot_attachment
            ON chatwoot_message_map (instance_id, chatwoot_message_id, chatwoot_attachment_index);`)
	return err
}

func applyChatwootAttachmentHostsMigration(driver string, tx *sqlx.Tx) error {
	if driver == "sqlite" {
		return addColumnIfNotExistsSQLite(tx, "chatwoot_config", "attachment_hosts", "TEXT NOT NULL DEFAULT '[]'")
	}
	_, err := tx.Exec(`ALTER TABLE chatwoot_config ADD COLUMN IF NOT EXISTS attachment_hosts TEXT NOT NULL DEFAULT '[]'`)
	return err
}

func applyChatwootWebhookCredentialsMigration(driver string, tx *sqlx.Tx) error {
	if driver == "sqlite" {
		if err := addColumnIfNotExistsSQLite(tx, "chatwoot_config", "webhook_key", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
		if err := addColumnIfNotExistsSQLite(tx, "chatwoot_config", "webhook_secret", "BLOB"); err != nil {
			return err
		}
		_, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_chatwoot_config_webhook_key ON chatwoot_config (webhook_key) WHERE webhook_key <> ''`)
		return err
	}
	_, err := tx.Exec(`ALTER TABLE chatwoot_config ADD COLUMN IF NOT EXISTS webhook_key TEXT NOT NULL DEFAULT '';
        ALTER TABLE chatwoot_config ADD COLUMN IF NOT EXISTS webhook_secret BYTEA;
        CREATE UNIQUE INDEX IF NOT EXISTS idx_chatwoot_config_webhook_key ON chatwoot_config (webhook_key) WHERE webhook_key <> '';`)
	return err
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
