## WuzAPI 1.1.1

### Added

- Added `POST /chat/deletechat` and stdio `chat.deletechat` to delete a conversation across the account's linked devices. This does not delete WuzAPI's stored message history. (#406)
- Added `selectableCount` to `/chat/send/poll` for multiple-answer polls. Omission keeps the single-answer default; 0 allows any number of answers. (#403)
- Added `sent_at` to message history to expose the send time reported by WhatsApp, separately from the local storage `timestamp`. (#395)
- Added automatic recovery for app state collections that fail to sync, using a full resync followed by a request to the primary phone when necessary. Enabled by default; disable with `WUZAPI_APPSTATE_RECOVERY=false`. (#396)
- Added `DELETE /admin/users/{id}/preserve-devices`, its stdio equivalent, and a dashboard recovery action for ambiguous device ownership. The action removes the instance, history and files while preserving WhatsApp device credentials without remote logout.
- Added opt-in `WUZAPI_IGNORE_GROUP_EVENTS=true` to discard inbound group events before media downloads, local history persistence and event delivery. Disabled by default. (#408)
- Added `WUZAPI_HISTORY_TRIM_INTERVAL` to control how often retention is enforced per user and chat. The default of 1 trims on every saved message.

### Fixed

- Consolidated SQLite application and WhatsApp storage in `dbdata/main.db`, with automatic migration of legacy `users.db` tables, fixing history persistence. (#392)
- Scoped the history index to the authenticated user, preventing exposure of other users' chat identifiers and last-message times. (#407)
- Fixed PostgreSQL history save lock ordering and allowed saves for separate chats of the same user to run concurrently. (#401)
- Persisted outgoing polls, reactions, edits and deletes in local history when history is enabled, including corrected newsletter action IDs. (#400, #402)
- Fixed PostgreSQL `sent_at` timezone handling so send times preserve the same instant across database session time zones.
- Canceled pending QR waits during session shutdown to prevent stale pairing attempts. (#405)
- Fixed login session status key casing and added chat and entry-kind context to HistorySync logs for messages without a sender. (#399)

### Release automation

- Docker publishing now verifies that the release tag's commit belongs to the history of `main` before logging in to Docker Hub or building images. Tags pointing to commits exclusive to other branches are rejected.

### Upgrade notes

- Stop older instances and back up `dbdata` before upgrading SQLite deployments. Legacy `users.db` tables migrate once into `main.db`; `users.db` remains as a pre-migration copy and is no longer kept in sync. Migration aborts rather than merging objects with conflicting names.
- Database migration 16 adds `message_history.sent_at`; migration 17 converts that column to `TIMESTAMPTZ` on PostgreSQL. Both run automatically on startup; the timezone conversion is not needed on SQLite.
- `sent_at` is omitted when unknown, including for previously stored rows. History results remain ordered by storage `timestamp`. Existing timezone-less PostgreSQL values are interpreted as UTC; offsets already lost by older writes outside UTC cannot be recovered automatically.
- With `WUZAPI_HISTORY_TRIM_INTERVAL` greater than 1, chats can temporarily exceed their retention limit until a later save triggers trimming. Counters reset on restart. Keep the default of 1 for enforcement on every save.
- App state recovery may require opening WhatsApp on the primary phone before settings operations succeed. Device-preserving deletion leaves devices linked; create and pair a new instance to record exact ownership when reusing the number.

**Full changelog:** https://github.com/asternic/wuzapi/compare/v1.1.0...v1.1.1

## WuzAPI 1.1.0

### Added

- Added the Svelte messaging client at `/client/`, with chats, contacts, groups, paginated history, attachments, avatars, and voice recording and playback. Added conversation listing, message pagination and voice-upload API endpoints.
- Added an admin proxy pool with per-proxy device capacity, automatic session assignment, dashboard management and explicit release of assignments.
- Added native image carousel messages through `/chat/send/carousel`. (#387)
- Added WhatsApp label editing and chat/message associations, conversation mute and unread settings, conversation pinning, message stars, played receipts, encrypted poll voting, and business profile lookup.
- Added automatic mentions from `@number` text, group mention-all support, and dashboard controls for mentions and copying group IDs. (#389, #390)
- Added group member-add mode controls on group creation and through `/group/memberaddmode`. (#385)
- Added newsletter join, leave and mute endpoints.
- Added `WUZAPI_AUTO_PRESENCE` to configure automatic presence after connection, `LOG_LEVEL` for application log filtering, and closed-socket session recovery controlled by `WUZAPI_SOCKET_RECOVERY`.
- Reworked attachment processing to use temporary files and bounded concurrency, reducing memory use. Added `WUZAPI_MEDIA_TMPDIR` and `WUZAPI_MEDIA_CONCURRENCY`.

### Fixed

- Updated the pinned Whatsmeow dependency and adapted blocklist handling.
- Fixed device ownership, stale-session cleanup and startup reconnect behavior. (#386, #388)
- Enforced per-chat history retention across live, outgoing and HistorySync messages, and improved complete user deletion cleanup.
- Persisted HistorySync edits, repaired legacy edit events when redelivered, and corrected group HistorySync sender resolution. (#370, #371, #381)
- Prevented credentials from leaking into logs and serialized shared RabbitMQ channel access during concurrent publishing. (#363)
- Improved document MIME detection from data URLs and filenames, automatic mention boundaries, audio caption handling and destination validation. (#374)
- Preserved handler failure messages in API error responses and validated sender identifiers when revoking group messages.
- Moved routine per-event traffic and expected webhook skip notices to debug logging, and fixed log-level loading after `.env` configuration. (#359, #377)

### Upgrade notes

- Database migrations 14 and 15 run automatically on startup, adding the proxy pool, user proxy assignments and explicit device ownership (`device_jid`) for SQLite and PostgreSQL.
- The web client requires a user token and enabled message history for polling. Browser voice recording requires HTTPS or localhost, and server-side FFmpeg with libopus. Built client assets are included; Node.js is only needed to rebuild them.
- Store media temporary files on a writable disk-backed directory or mounted volume that supports file locks. A tmpfs still consumes RAM. Media concurrency defaults to 2.
- Automatic presence defaults to `available`. Set `WUZAPI_AUTO_PRESENCE=unavailable` to preserve primary-phone push notifications; this also affects contact presence updates.
- When all enabled pool proxies are full, `WUZAPI_PROXY_POOL_FALLBACK=block` refuses connection with HTTP 503 by default. Use `direct` only if connecting without a proxy is intended.
- Closed-socket recovery is enabled by default and reconnects the session without retrying the failed message. Clients still receive the original send error.
- A history setting of 0 disables local persistence without suppressing HistorySync webhooks. Repair of legacy edits depends on WhatsApp redelivery and does not backfill existing history.

**Full changelog:** https://github.com/asternic/wuzapi/compare/v1.0.9...v1.1.0

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
