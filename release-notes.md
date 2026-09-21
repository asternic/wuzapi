## WuzAPI 1.0.9

### Added

- Added `/chat/pin` to pin and unpin WhatsApp messages. (#336)
- Completed pairing history sync configuration with SQLite and PostgreSQL support, API and dashboard controls, and per-user history requests. Supersedes #361.

### Fixed

- Updated Whatsmeow to resolve "Client outdated" (405) login failures. (#368)
- Fixed empty JIDs in `/session/status` after QR pairing and improved device lookup when reconnecting. (#333)
- Limited full webhook payload logging to small payloads to avoid excessive log output. (#352)
- Replaced the incomplete history sync loop with Whatsmeow's native history-window request during pairing, keeping sync days separate from message retention.

### Release automation

- GitHub releases now trigger Docker image publishing for Linux amd64 and arm64, with versioned and `latest` tags.

### Upgrade notes

- Database migration 13 runs automatically on startup and adds `days_to_sync_history`. Existing values are preserved; new settings default to 0 (WhatsApp's normal sync behavior).
- Configure `days_to_sync_history` (0–365) before starting a new pairing through the dashboard, admin user API, or `POST /session/history`. Changing it does not trigger backfill on an already linked account or an ordinary reconnect. WhatsApp and the phone determine the history available.
- `history` remains the separate local message retention count per chat.
- Building from source now requires Go 1.26.0 or newer.

**Full changelog:** https://github.com/asternic/wuzapi/compare/v1.0.8...v1.0.9
