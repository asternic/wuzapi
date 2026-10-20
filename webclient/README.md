# WuzAPI Web Client

A self-hosted Svelte 5 + Vite client for the WuzAPI instance in this repository.
The interface is in Spanish and adapts to desktop and mobile screens.

## Open it

Start WuzAPI normally and visit **`/client/`** on the same server. The built
assets are included in `static/client/`, so running the Go service or its Docker
image does not require Node.js. Use the token of an existing **user**, not the
admin token. The client can also point to another WuzAPI server that permits the
browser origin through CORS. Use HTTPS for remote deployments.

The landing page includes an explicitly labeled, read-only demo. Demo messages
are synthetic and do not connect to WhatsApp.

## Features

- Contact directory, joined groups, new conversations, and group creation.
- Profile pictures for visible contacts, groups, and participants, using the existing
  `/user/avatar` endpoint with `Preview: true`. The active chat has queue priority.
  Requests are shared across views, limited to two concurrent queries with at
  least 200 ms between starts. A per-session in-memory cache holds up to 256
  results: pictures for 15 minutes, missing/private/broken pictures for 5 minutes.
  Initials remain visible when no picture is available. Cache is cleared on
  disconnecting the client, switching accounts, or closing the page; photos are
  not queried on every message poll.
- Conversation summaries and stable, cursor-paginated message history.
- Text sending, multiline drafts per conversation, images, videos, audio files,
  documents, and on-demand downloads of historical media.
- Voice recording with preview/cancel, up to five minutes and 16 MiB. The server
  converts browser recordings to OGG/Opus through **FFmpeg with libopus** before
  using the existing audio sender. Microphone access requires HTTPS or localhost.
- Contact/group search, search within loaded messages, group participants,
  explicitly requested read receipts, and played receipts when incoming audio is
  played. The check on a sent message means sent, not delivered or read.
- QR connection and history configuration. If WhatsApp requires a passkey, finish
  pairing in the existing WuzAPI dashboard.

## History and incoming messages

Enable history in client settings before pairing if you want initial history
sync. `history` is the number of retained messages **per conversation**;
`days_to_sync_history` is the requested window for the **next pairing**. Setting
retention to zero disables history. The client never enables retention or logs
out your WhatsApp session automatically.

Incoming messages are read from the local history database by polling every
3.5 seconds while the tab is visible. There is no WebSocket/SSE connection or
webhook replacement. Existing webhooks and RabbitMQ settings are untouched.
The “Nuevos” filter tracks changes observed during this browser session; it is
not a synchronized WhatsApp unread count.

WhatsApp status broadcasts (`status@broadcast`) are excluded from chats, contacts,
and new-message badges. They remain in backend history; this client does not yet
provide a dedicated status viewer. Other broadcast-list JIDs are unaffected.

“Recuperar historial del teléfono” uses the oldest loaded message as an anchor
and requests 50 earlier messages. Requests are asynchronous; WhatsApp decides
what is available. Reopen/refresh the conversation to see recovered messages
that are older than the currently loaded pages. “Cargar mensajes anteriores”
reads already-stored messages from WuzAPI instead of requesting the phone.

Media is downloaded only on demand. Historical media can expire on WhatsApp's
servers; old outgoing rows without download metadata cannot be reconstructed.
New outgoing audio/image/video/document/sticker messages retain their encrypted
media download metadata when history is enabled, without storing full plaintext
attachments in the database. The existing retention policy continues to apply.
Browser playback depends on supported codecs (especially OGG/Opus on older Safari).

Identifiers remain strings. Preserve full JIDs, including `@lid` and `@g.us`.
Usernames and Cloud API BSUIDs are not resolved by the client's destination field.

## Credentials and privacy

The user token is sent only in the `token` header to the configured API. It is
kept in memory by default. The opt-in “Recordar en esta pestaña” uses
`sessionStorage`; closing the client clears it. Messages, media, and drafts are
not written to browser storage. “Cerrar el cliente” clears browser state and
aborts requests without disconnecting or unlinking WhatsApp.

The client performs authenticated history queries scoped to the current user;
it does not call the legacy `chat_jid=index` endpoint. New history responses use
`Cache-Control: no-store`. No analytics, CDN fonts, or third-party scripts load.

## Development

Use Node.js 22.12+ or a compatible newer LTS release.

```sh
cd webclient
npm ci
npm run dev
```

Vite serves `http://127.0.0.1:5173/client/` and proxies API paths to
`http://127.0.0.1:8080`. Override the target with `WUZAPI_DEV_TARGET` if needed.
The URL field defaults to the page origin, which works with this proxy.

```sh
npm run check
npm test
npm run build
```

Build output goes to `../static/client/` with the `/client/` asset base. Commit
both source/lockfile and rebuilt static assets when changing the interface.
Do not edit generated JavaScript or CSS directly.

## Added API routes

- `GET /chat/conversations?limit=100&offset=0`: tenant-scoped latest-message
  summaries with `chats`, `has_more`, and `next_offset`; max limit 200.
- `GET /chat/messages?chat_jid=...&limit=50&before=ROW_ID`: tenant-scoped history
  page with `messages`, `has_more`, and `next_before`; max limit 100. A cursor is
  a row ID owned by the same user and chat. Ordering is timestamp descending,
  then row ID descending. A missing/pruned cursor returns 404.
- `POST /chat/send/voice`: multipart `Phone`, `Audio` recording, optional `Seconds` (0–300), and optional `Id`.
  Maximum input recording: 16 MiB; conversion is limited to five minutes and a
  45-second conversion timeout. Uses the normal audio response format. Invalid
  uploads return 400, conversion failures return 422, and session/send failures
  retain the existing audio handler behavior.

The read routes require enabled history (501 otherwise) and are also mapped in
stdio as `chat.conversations` and `chat.messages`. Voice upload is HTTP-only
because it accepts multipart files; stdio clients can continue to use
`chat.send.audio` with an OGG/Opus data URL.

## Verification boundaries

Automated tests cover user isolation, cursor ownership, pagination with equal
timestamps and newly arrived messages, legacy transport compatibility, metadata
persistence, frontend history normalization, safe media URLs, and OGG/Opus
conversion when FFmpeg/libopus is available. The history contract also runs on
PostgreSQL when `WUZAPI_TEST_POSTGRES_DSN` is set. Browser QA uses synthetic data;
actual WhatsApp delivery, QR/passkey completion, microphone capture, and phone
history availability need a smoke test with a linked account.
