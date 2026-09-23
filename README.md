# WUZAPI

<img src="static/favicon.ico" width="30"> WuzAPI is an implementation 
of the [@tulir/whatsmeow](https://github.com/tulir/whatsmeow) library as a 
simple RESTful API service with multiple device support and concurrent 
sessions.

Whatsmeow does not use Puppeteer on headless Chrome, nor an Android emulator. It communicates directly with WhatsApp’s WebSocket servers, making it significantly faster and much less demanding on memory and CPU than those solutions. The drawback is that any changes to the WhatsApp protocol could break connections, requiring a library update.

## :warning: Warning

**Using this software in violation of WhatsApp’s Terms of Service can get your number banned**:  
Be very careful—do not use this to send SPAM or anything similar. Use at your own risk. If you need to develop something for commercial purposes, contact a WhatsApp global solution provider and sign up for the WhatsApp Business API service instead.

## Available endpoints

* **Session:** Connect, disconnect, and log out from WhatsApp. Retrieve connection status and QR codes for scanning.
* **Messages:** Send text, image, audio, document, template, video, sticker, location, contact, and poll messages.
* **Users:** Check if phone numbers have WhatsApp, get user information and avatars, and retrieve the full contact list.
* **Chat:** Set presence (typing/paused, recording media), mark messages as read, download images from messages, send reactions.
* **Groups:** Create, delete and list groups, get info, get invite links, set participants, change group photos and names.
* **Webhooks:** Set and get webhooks that will be called whenever events or messages are received.
* **HMAC Configuration:** Configure HMAC keys for webhook security and signature verification.

### Webhook HMAC Signing

When HMAC is configured, all webhooks include an `x-hmac-signature` header with SHA-256 HMAC signature.

#### Signature Generation by Content-Type:

**`application/json`**
* Signed data: Raw JSON request body
* Verification: Use the exact JSON received

**`application/x-www-form-urlencoded`**
* Signed data: URL-encoded form string (`key=value&key2=value2`)
* Verification: Reconstruct the form string from received parameters

**`multipart/form-data`** (file uploads)
* Signed data: JSON representation of form fields (excluding files)
* Verification: Create JSON from non-file form fields

* Always verify signatures before processing webhooks

## Prerequisites

**Required:**
* Go (Go Programming Language)

**Optional:**
* Docker (for containerization)

## Building

```
go build .
```

## Homebrew installation

To install `wuzapi` via [Homebrew](https://brew.sh) use:

```sh
brew install asternic/wuzapi/wuzapi
```

## Run

By default it will start a REST service in port 8080. These are the parameters
you can use to alter behaviour

* -admintoken  : sets authentication token for admin endpoints. If not specified it will be read from .env
* -address  : sets the IP address to bind the server to (default 0.0.0.0)
* -port  : sets the port number (default 8080)
* -logtype : format for logs, either console (default) or json
* -color : enable colored output for console logs
* -osname : Connection OS Name in Whatsapp
* -autopresence : automatic presence after connecting, either available (default) or unavailable
* -skipmedia : Skip downloading media from messages
* -wadebug : enable whatsmeow debug, either INFO or DEBUG levels are suported

* -sslcertificate : SSL Certificate File
* -sslprivatekey : SSL Private Key File

Example:

To have colored logs:

```
./wuzapi -logtype=console -color=true
```

For JSON logs:

```
./wuzapi -logtype json 
```

With time zone: 

Set `TZ=America/New_York ./wuzapi ...` in your shell or in your .env file or Docker Compose environment: `TZ=America/New_York`.  

## Configuration

WuzAPI uses a `.env` file for configuration. You can use the provided `.env.sample` as a template:

```bash
cp .env.sample .env
```

### Environment Variables

#### Required Settings
```
WUZAPI_ADMIN_TOKEN=your_admin_token_here
```

#### Security Settings

```
WUZAPI_GLOBAL_ENCRYPTION_KEY=your_32_byte_encryption_key_here
WUZAPI_GLOBAL_HMAC_KEY=your_global_hmac_key_here
```

#### Optional Settings

```
TZ=America/New_York
WEBHOOK_FORMAT=json
SESSION_DEVICE_NAME=WuzAPI
WUZAPI_AUTO_PRESENCE=available
WUZAPI_PORT=8080
WUZAPI_GLOBAL_WEBHOOK=https://your-global-webhook.url
WEBHOOK_RETRY_ENABLED=true
WEBHOOK_RETRY_COUNT=2
WEBHOOK_RETRY_DELAY_SECONDS=30
WEBHOOK_ERROR_QUEUE_NAME=wuzapi_dead_letter_webhooks
WUZAPI_MEDIA_CONCURRENCY=2
# WUZAPI_MEDIA_TMPDIR=/path/to/disk-backed/temp
```

### Attachment temporary storage

Incoming event attachments and outgoing document, audio, image, video, sticker,
and optional button-image uploads use private temporary files automatically.
`WUZAPI_MEDIA_TMPDIR` defaults to the operating system temporary directory.
`WUZAPI_MEDIA_CONCURRENCY` defaults to `2` and limits simultaneous media downloads,
uploads, decoding, conversions, and delivery-body preparation. Waiting operations
honor cancellation; webhook retry backoff does not hold a media slot.

Use a **disk-backed** writable directory or container volume for memory savings.
A tmpfs mount still consumes RAM. Set the path inside the container and mount the
backing storage there; setting a host path in `.env` alone does not mount it.
The directory must support file locks. Allow space for plaintext, encryption or
conversion scratch files, and base64 delivery bodies (approximately 4/3 of the
attachment size each). Slow receivers and pending retries retain delivery files.

Files are removed when their consumers finish. Each process owns a locked private
directory; later starts reclaim abandoned directories without touching another
running instance. Temporary files are not a durable queue and do not recover
in-flight messages after a crash. Existing API inputs, delivery formats, and URL
size limits are unchanged. JSON request decoding, image pixel decoding, and
RabbitMQ's final payload buffer still have size-dependent memory costs.

See [attachment memory validation](media-memory.md) for measurements and test limits.

### Log verbosity

Set `LOG_LEVEL=warn` in `.env` or the process environment to show warnings and
more severe WuzAPI logs. Accepted values are `trace`, `debug`, `info`, `warn`,
`error`, `fatal`, and `panic`, ignoring case and surrounding whitespace. An unset,
empty, or invalid value preserves the existing verbosity; numeric values and
`disabled` are not accepted. Existing process environment values take precedence
over `.env`, including an explicitly empty value.

The filter is applied before startup messages and works with console and JSON
output. It controls WuzAPI's zerolog logger, not Whatsmeow's `-wadebug` output.
Compose and Swarm forward `LOG_LEVEL`; for Swarm, export it before deploying since
`docker stack deploy` does not automatically read `.env` for substitution.

### Important Notes

#### Auto-Generated Credentials
If the following settings are not provided, they will be auto-generated:
* `WUZAPI_ADMIN_TOKEN`: Random 32-character token
* `WUZAPI_GLOBAL_ENCRYPTION_KEY`: Random 32-byte key for AES-256 encryption

**Important**: Save auto-generated credentials to your `.env` file or you will lose access to encrypted data and admin functions on restart!

#### Webhook Security
* `WUZAPI_GLOBAL_HMAC_KEY`: Global HMAC key for webhook signing (minimum 32 characters)

#### Database Configuration

**For PostgreSQL:**
```
DB_USER=wuzapi
DB_PASSWORD=wuzapi
DB_NAME=wuzapi
DB_HOST=db  # Use 'db' when running with Docker Compose, or 'localhost' for native execution
DB_PORT=5432
DB_SSLMODE=false
```

**For SQLite (default):**
No database configuration needed - SQLite is used by default if no PostgreSQL settings are provided.

#### Optional Settings
```
TZ=America/New_York
WEBHOOK_FORMAT=json # or "form" for the default
SESSION_DEVICE_NAME=WuzAPI
WUZAPI_PORT=8080 # Port for the WuzAPI server
WUZAPI_GLOBAL_WEBHOOK= # Global webhook URL for all instances
WUZAPI_AUTO_PRESENCE=available # use unavailable to preserve primary-phone push notifications
```

`WUZAPI_AUTO_PRESENCE` controls the presence announced after a session connects or
its push name changes. The default `available` value preserves the existing behavior
and enables contact presence updates. Set it to `unavailable` to keep the linked
client offline so WhatsApp continues sending push notifications to the primary phone.

### RabbitMQ Integration
WuzAPI supports sending WhatsApp events to a RabbitMQ queue for global event distribution. When enabled, all WhatsApp events will be published to the specified queue regardless of individual user webhook configurations.

Set these environment variables to enable RabbitMQ integration:

```
RABBITMQ_URL=amqp://guest:guest@localhost:5672
RABBITMQ_QUEUE=whatsapp  # Optional (default: whatsapp_events)
```

When enabled:

* All WhatsApp events (messages, presence updates, etc.) will be published to the configured queue regardless of event subscritions for regular webhooks
* Events will include the userId and instanceName
* This works alongside webhook configurations - events will be sent to both RabbitMQ and any configured webhooks
* The integration is global and affects all instances

### Webhook Security with HMAC

WuzAPI supports HMAC signatures for webhook verification:

* **Per-instance HMAC**: Configure unique HMAC keys for each user instance
* **Global HMAC**: Set a global HMAC key via `WUZAPI_GLOBAL_HMAC_KEY` environment variable
* **Signature Header**: All signed webhooks include `x-hmac-signature` header
* **Key Security**: HMAC keys are never exposed after configuration

**Priority**: Instance HMAC > Global HMAC > No signature

Configure HMAC keys via the Dashboard or using the `/session/hmac/config` API endpoints.

#### Key configuration options:

* WUZAPI_ADMIN_TOKEN: Required - Authentication token for admin endpoints
* TZ: Optional - Timezone for server operations (default: UTC)
* PostgreSQL-specific options: Only required when using PostgreSQL backend
* RabbitMQ options: Optional, only required if you want to publish events to RabbitMQ

### Docker Configuration

When using Docker Compose, `docker-compose.yml` automatically loads environment variables from a `.env` file when available. However, `docker-compose-swarm.yaml` uses `docker stack deploy`, which does not automatically load from `.env` files. Variables in the swarm file will only be substituted if they are exported in the shell environment where the deploy command is run. For managing secrets in Swarm, consider using Docker secrets.

The Docker configuration will:
1. First load variables from the `.env` file (if present and supported)
2. Use default values as fallback if variables are not defined
3. Override with any variables explicitly set in the `environment` section of the compose file

**Key differences for Docker deployment:**
- Set `DB_HOST=db` instead of `localhost` to connect to the PostgreSQL container
- The `WUZAPI_PORT` variable controls the external port mapping in `docker-compose.yml`
- In swarm mode, `WUZAPI_PORT` configures the Traefik load balancer port

**Note:** The `.env` file is already included in `.gitignore` to avoid committing sensitive information to your repository.

## Usage

To interact with the API, you must include the `Authorization` header in HTTP requests, containing the user's authentication token. You can have multiple users (different WhatsApp numbers) on the same server.  

* A Swagger API reference at [/api](/api)
* A sample web page to connect and scan QR codes at [/login](/login)
* A fully featured Dashboard to create, manage and test instances at [/dashboard](dashboard)

## ADMIN Actions

You can list, add and remove users using the admin endpoints. For that you must use the WUZAPI_ADMIN_TOKEN in the Authorization header

Then you can use the /admin/users endpoint with the Authorization header containing the token to:

- `GET /admin/users` - List all users
- `POST /admin/users` - Create a new user
- `DELETE /admin/users/{id}` - Remove a user

The JSON body for creating a new user must contain:

- `name` [string] : User's name 
- `token` [string] : Security token to authorize/authenticate this user
- `webhook` [string] : URL to send events via POST (optional)
- `events` [string] : Comma-separated list of events to receive (required) - Valid events are: "Message", "ReadReceipt", "Presence", "HistorySync", "ChatPresence", "All"
- `expiration` [int] : Expiration timestamp (optional, not enforced by the system)

## User Creation with Optional Proxy and S3 Configuration

You can create a user with optional proxy and S3 storage configuration. All fields are optional and backward compatible. If you do not provide these fields, the user will be created with default settings.

### Example Payload

```json
{
  "name": "test_user",
  "token": "user_token",
  "proxyConfig": {
    "enabled": true,
    "proxyURL": "socks5://user:pass@host:port"
  },
  "s3Config": {
    "enabled": true,
    "endpoint": "https://s3.amazonaws.com",
    "region": "us-east-1",
    "bucket": "my-bucket",
    "accessKey": "AKIAIOSFODNN7EXAMPLE",
    "secretKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
    "pathStyle": false,
    "publicURL": "https://cdn.yoursite.com",
    "mediaDelivery": "both",
    "retentionDays": 30
  }
}
```

- `proxyConfig` (object, optional):
  - `enabled` (boolean): Enable proxy for this user.
  - `proxyURL` (string): Proxy URL (e.g., `socks5://user:pass@host:port`).
- `s3Config` (object, optional):
  - `enabled` (boolean): Enable S3 storage for this user.
  - `endpoint` (string): S3 endpoint URL.
  - `region` (string): S3 region.
  - `bucket` (string): S3 bucket name.
  - `accessKey` (string): S3 access key.
  - `secretKey` (string): S3 secret key.
  - `pathStyle` (boolean): Use path style addressing.
  - `publicURL` (string): Public URL for accessing files.
  - `mediaDelivery` (string): Media delivery type (`base64`, `s3`, or `both`).
  - `retentionDays` (integer): Number of days to retain files.

If you omit `proxyConfig` or `s3Config`, the user will be created without proxy or S3 integration, maintaining full backward compatibility.

## API reference 

API calls should be made with content type json, and parameters sent into the
request body, always passing the Token header for authenticating the request.

Check the [API Reference](https://github.com/asternic/wuzapi/blob/main/API.md)

## Updating the upstream whatsmeow library

> [!CAUTION]
> This section is intended for maintainers and developers. Regular users should use the whatsmeow version pinned in `go.mod` and should not upgrade it as part of the normal installation or build process.

WuzAPI uses [whatsmeow](https://github.com/tulir/whatsmeow) to communicate with WhatsApp. Its upstream API can introduce breaking changes, so upgrading to the latest version may cause WuzAPI to stop compiling or working correctly until its code is adapted.

Perform upgrades in a development branch, review the resulting `go.mod` and `go.sum` changes, and verify that WuzAPI builds and its tests pass before deploying the update:

```bash
go get -u go.mau.fi/whatsmeow@latest
go mod tidy
go test ./...
go build .
```

## Contributors

<!-- CONTRIBUTORS:START -->

<table><tr>
<td align="center">
    <a href="https://github.com/asternic">
      <img src="https://avatars.githubusercontent.com/u/25182694?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>asternic</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/cleitonme">
      <img src="https://avatars.githubusercontent.com/u/12551230?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>cleitonme</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/guilhermejansen">
      <img src="https://avatars.githubusercontent.com/u/52773109?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>guilhermejansen</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/LuizFelipeNeves">
      <img src="https://avatars.githubusercontent.com/u/14094719?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>LuizFelipeNeves</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/WellingtonFonseca">
      <img src="https://avatars.githubusercontent.com/u/25608175?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>WellingtonFonseca</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/xenodium">
      <img src="https://avatars.githubusercontent.com/u/8107219?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>xenodium</b></sub>
    </a>
  </td>
</tr><tr>
<td align="center">
    <a href="https://github.com/ThiagoBauken">
      <img src="https://avatars.githubusercontent.com/u/107090829?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>ThiagoBauken</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/ramon-victor">
      <img src="https://avatars.githubusercontent.com/u/13617054?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>ramon-victor</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/AntonKun">
      <img src="https://avatars.githubusercontent.com/u/59668952?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>AntonKun</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/vitorsilvalima">
      <img src="https://avatars.githubusercontent.com/u/9752658?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>vitorsilvalima</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/Piahn">
      <img src="https://avatars.githubusercontent.com/u/132025108?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>Piahn</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/netrixken">
      <img src="https://avatars.githubusercontent.com/u/9066682?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>netrixken</b></sub>
    </a>
  </td>
</tr><tr>
<td align="center">
    <a href="https://github.com/luizrgf2">
      <img src="https://avatars.githubusercontent.com/u/71092163?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>luizrgf2</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/andreydruz">
      <img src="https://avatars.githubusercontent.com/u/976438?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>andreydruz</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/devLucasMoraes">
      <img src="https://avatars.githubusercontent.com/u/104109951?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>devLucasMoraes</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/RuanAyram">
      <img src="https://avatars.githubusercontent.com/u/16547662?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>RuanAyram</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/pedroafonso18">
      <img src="https://avatars.githubusercontent.com/u/157052926?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>pedroafonso18</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/Alg0rix">
      <img src="https://avatars.githubusercontent.com/u/53804949?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>Alg0rix</b></sub>
    </a>
  </td>
</tr><tr>
<td align="center">
    <a href="https://github.com/igortrinidad">
      <img src="https://avatars.githubusercontent.com/u/13478652?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>igortrinidad</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/chrsmendes">
      <img src="https://avatars.githubusercontent.com/u/77082167?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>chrsmendes</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/claytim">
      <img src="https://avatars.githubusercontent.com/u/47343472?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>claytim</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/eliasmeireles">
      <img src="https://avatars.githubusercontent.com/u/13203692?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>eliasmeireles</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/jeffersonfelixdev">
      <img src="https://avatars.githubusercontent.com/u/3003222?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>jeffersonfelixdev</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/My-con">
      <img src="https://avatars.githubusercontent.com/u/123265027?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>My-con</b></sub>
    </a>
  </td>
</tr><tr>
<td align="center">
    <a href="https://github.com/paul-lestyo">
      <img src="https://avatars.githubusercontent.com/u/51690314?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>paul-lestyo</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/luiis716">
      <img src="https://avatars.githubusercontent.com/u/97978347?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>luiis716</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/dgattupalli696">
      <img src="https://avatars.githubusercontent.com/u/219828309?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>dgattupalli696</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/joaosouz4dev">
      <img src="https://avatars.githubusercontent.com/u/47183663?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>joaosouz4dev</b></sub>
    </a>
  </td>
<td align="center">
    <a href="https://github.com/gusnips">
      <img src="https://avatars.githubusercontent.com/u/981265?v=4" width="100px;" style="border-radius:50%;"/><br />
      <sub><b>gusnips</b></sub>
    </a>
  </td>
</tr></table>

<!-- CONTRIBUTORS:END -->

## Clients

- [wuzapi TypeScript / Node Client](https://github.com/gusnips/wuzapi-node)

## Pairing history sync

`days_to_sync_history` requests a full history window from WhatsApp when linking
an account. Set it through `POST /admin/users`, `PUT /admin/users/{id}`, or
`POST /session/history` **before starting pairing**:

```json
{"history": 1000, "days_to_sync_history": 30}
```

The dashboard exposes the same setting when creating a user and in History
Configuration. The session configuration endpoint works without an active WhatsApp
client. Its omitted fields are preserved; sending `days_to_sync_history: 0`
restores WhatsApp's default sync behavior. Values from 0 through 365 are accepted.
Admin user responses and `GET /session/status` expose the saved value.

Sync days and `history` are separate settings: `history` is the local per-chat
message retention count, not a number of days. The requested window is sent in
that user's pairing payload; WhatsApp and the phone determine which messages are
available. Incoming batches use the existing `HistorySync` processing and webhooks.
Zero sync days does not suppress WhatsApp's normal history events.

Changes apply on the next **new pairing**, not an ordinary reconnect. If a QR has
already been issued, restart the pairing flow after saving. For an already linked
account, the explicit `GET /session/history` endpoint remains available for
message-based history requests; changing sync days alone does not backfill it.
Both SQLite and PostgreSQL are supported, with existing accounts defaulting to 0.

Regression tests run on SQLite with `go test ./...`. To run the same migration,
API, and pairing-payload checks on PostgreSQL, set `WUZAPI_TEST_POSTGRES_DSN` to a
test database connection string and run `go test -run TestHistorySync ./...`.
The database role must be able to create schemas; tests create and remove their
own schemas.

## Star History

<a href="https://www.star-history.com/?type=date&repos=asternic%2Fwuzapi">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=asternic/wuzapi&type=date&theme=dark&legend=top-left&sealed_token=btZMq-H0d-DBRgXRdFTBx24bZ3x6oVGnSTwAk6DEM19J5wiWYhsN20SekiMRIbFaEkIhmwM5_SyQKT1QTvNVYF9QAaFLdvvPPEq7Y7dvZ34MoKnKNXyXlQgerN1ag_hYzp9RGYAywggEXDxTESW-asFZnacNcBq7LvO4XhspFm-KflmgBomjG_czi8vR" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=asternic/wuzapi&type=date&legend=top-left&sealed_token=btZMq-H0d-DBRgXRdFTBx24bZ3x6oVGnSTwAk6DEM19J5wiWYhsN20SekiMRIbFaEkIhmwM5_SyQKT1QTvNVYF9QAaFLdvvPPEq7Y7dvZ34MoKnKNXyXlQgerN1ag_hYzp9RGYAywggEXDxTESW-asFZnacNcBq7LvO4XhspFm-KflmgBomjG_czi8vR" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=asternic/wuzapi&type=date&legend=top-left&sealed_token=btZMq-H0d-DBRgXRdFTBx24bZ3x6oVGnSTwAk6DEM19J5wiWYhsN20SekiMRIbFaEkIhmwM5_SyQKT1QTvNVYF9QAaFLdvvPPEq7Y7dvZ34MoKnKNXyXlQgerN1ag_hYzp9RGYAywggEXDxTESW-asFZnacNcBq7LvO4XhspFm-KflmgBomjG_czi8vR" />
 </picture>
</a>

## License

Copyright &copy; 2025 Nicolás Gudiño and contributors

[MIT](https://choosealicense.com/licenses/mit/)

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Icon Attribution

[Communication icons created by Vectors Market -
Flaticon](https://www.flaticon.com/free-icons/communication)

## Legal

This code is in no way affiliated with, authorized, maintained, sponsored or
endorsed by WhatsApp or any of its affiliates or subsidiaries. This is an
independent and unofficial software. Use at your own risk.

## Cryptography Notice

This distribution includes cryptographic software. The country in which you
currently reside may have restrictions on the import, possession, use, and/or
re-export to another country, of encryption software. BEFORE using any
encryption software, please check your country's laws, regulations and policies
concerning the import, possession, or use, and re-export of encryption
software, to see if this is permitted. See
[http://www.wassenaar.org/](http://www.wassenaar.org/) for more information.

The U.S. Government Department of Commerce, Bureau of Industry and Security
(BIS), has classified this software as Export Commodity Control Number (ECCN)
5D002.C.1, which includes information security software using or performing
cryptographic functions with asymmetric algorithms. The form and manner of this
distribution makes it eligible for export under the License Exception ENC
Technology Software Unrestricted (TSU) exception (see the BIS Export
Administration Regulations, Section 740.13) for both object code and source
code.

### Message history edits

`/chat/history` returns stored events, newest first, rather than the current state of
each message. Both live and HistorySync edits are stored as separate `edit` rows:
`message_id` identifies the edit, `quoted_message_id` identifies its target, and
`text_content` contains the replacement text or caption, including an empty caption.
The original message stays unchanged. Consumers must apply edits to their targets;
use the protocol message's `timestampMS` in `datajson` to order successive edits when
available, since database timestamps record insertion time, including HistorySync.
A limited response can include an edit without the original message.

Previously imported `unknown` edits are repaired when redelivered. This does not
backfill existing history or recover events that WhatsApp does not redeliver.
