-- migrations/0001_create_users_table.up.sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    token TEXT NOT NULL,
    webhook TEXT NOT NULL DEFAULT '',
    jid TEXT NOT NULL DEFAULT '',
    qrcode TEXT NOT NULL DEFAULT '',
    connected INTEGER,
    expiration INTEGER,
    events TEXT NOT NULL DEFAULT 'All'
);
