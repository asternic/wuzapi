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

## Updating dependencies

This project uses the whatsmeow library to communicate with WhatsApp. To update the library to the latest version, run:

```bash
go get -u go.mau.fi/whatsmeow@latest
go mod tidy
```

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
WUZAPI_PORT=8080
WUZAPI_GLOBAL_WEBHOOK=https://your-global-webhook.url
WEBHOOK_RETRY_ENABLED=true
WEBHOOK_RETRY_COUNT=2
WEBHOOK_RETRY_DELAY_SECONDS=30
WEBHOOK_ERROR_QUEUE_NAME=wuzapi_dead_letter_webhooks
```

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
```

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

## Contributors

<table>
<tr>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/asternic>
            <img src=https://avatars.githubusercontent.com/u/25182694?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Nicolas/>
            <br />
            <sub style="font-size:14px"><b>Nicolas</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/guilhermejansen>
            <img src=https://avatars.githubusercontent.com/u/52773109?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Guilherme Jansen/>
            <br />
            <sub style="font-size:14px"><b>Guilherme Jansen</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/LuizFelipeNeves>
            <img src=https://avatars.githubusercontent.com/u/14094719?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Luiz Felipe Neves/>
            <br />
            <sub style="font-size:14px"><b>Luiz Felipe Neves</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/cleitonme>
            <img src=https://avatars.githubusercontent.com/u/12551230?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=cleitonme/>
            <br />
            <sub style="font-size:14px"><b>cleitonme</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/WellingtonFonseca>
            <img src=https://avatars.githubusercontent.com/u/25608175?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Wellington Fonseca/>
            <br />
            <sub style="font-size:14px"><b>Wellington Fonseca</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/xenodium>
            <img src=https://avatars.githubusercontent.com/u/8107219?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=xenodium/>
            <br />
            <sub style="font-size:14px"><b>xenodium</b></sub>
        </a>
    </td>
</tr>
<tr>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/ramon-victor>
            <img src=https://avatars.githubusercontent.com/u/13617054?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=ramon-victor/>
            <br />
            <sub style="font-size:14px"><b>ramon-victor</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/netrixken>
            <img src=https://avatars.githubusercontent.com/u/9066682?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Netrix Ken/>
            <br />
            <sub style="font-size:14px"><b>Netrix Ken</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/luizrgf2>
            <img src=https://avatars.githubusercontent.com/u/71092163?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Luiz Ricardo Gonçalves Felipe/>
            <br />
            <sub style="font-size:14px"><b>Luiz Ricardo Gonçalves Felipe</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/andreydruz>
            <img src=https://avatars.githubusercontent.com/u/976438?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=andreydruz/>
            <br />
            <sub style="font-size:14px"><b>andreydruz</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/vitorsilvalima>
            <img src=https://avatars.githubusercontent.com/u/9752658?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Vitor Silva Lima/>
            <br />
            <sub style="font-size:14px"><b>Vitor Silva Lima</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/RuanAyram>
            <img src=https://avatars.githubusercontent.com/u/16547662?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Ruan Kaylo/>
            <br />
            <sub style="font-size:14px"><b>Ruan Kaylo</b></sub>
        </a>
    </td>
</tr>
<tr>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/pedroafonso18>
            <img src=https://avatars.githubusercontent.com/u/157052926?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Pedro Afonso/>
            <br />
            <sub style="font-size:14px"><b>Pedro Afonso</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/igortrinidad>
            <img src=https://avatars.githubusercontent.com/u/13478652?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Igor Trindade/>
            <br />
            <sub style="font-size:14px"><b>Igor Trindade</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/chrsmendes>
            <img src=https://avatars.githubusercontent.com/u/77082167?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Christopher Mendes/>
            <br />
            <sub style="font-size:14px"><b>Christopher Mendes</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/luiis716>
            <img src=https://avatars.githubusercontent.com/u/97978347?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=luiis716/>
            <br />
            <sub style="font-size:14px"><b>luiis716</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/joaosouz4dev>
            <img src=https://avatars.githubusercontent.com/u/47183663?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=João Victor Souza/>
            <br />
            <sub style="font-size:14px"><b>João Victor Souza</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/gusnips>
            <img src=https://avatars.githubusercontent.com/u/981265?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Gustavo Salomé />
            <br />
            <sub style="font-size:14px"><b>Gustavo Salomé </b></sub>
        </a>
    </td>
</tr>
<tr>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/AntonKun>
            <img src=https://avatars.githubusercontent.com/u/59668952?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Anton Kozyk/>
            <br />
            <sub style="font-size:14px"><b>Anton Kozyk</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/anilgulecha>
            <img src=https://avatars.githubusercontent.com/u/1016984?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Anil Gulecha/>
            <br />
            <sub style="font-size:14px"><b>Anil Gulecha</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/AlanMartines>
            <img src=https://avatars.githubusercontent.com/u/10979090?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Alan Martines/>
            <br />
            <sub style="font-size:14px"><b>Alan Martines</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/DwiRizqiH>
            <img src=https://avatars.githubusercontent.com/u/69355492?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Ahmad Dwi Rizqi Hidayatulloh/>
            <br />
            <sub style="font-size:14px"><b>Ahmad Dwi Rizqi Hidayatulloh</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/elohmeier>
            <img src=https://avatars.githubusercontent.com/u/2536303?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=elohmeier/>
            <br />
            <sub style="font-size:14px"><b>elohmeier</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/fadlee>
            <img src=https://avatars.githubusercontent.com/u/334797?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Fadlul Alim/>
            <br />
            <sub style="font-size:14px"><b>Fadlul Alim</b></sub>
        </a>
    </td>
</tr>
<tr>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/joaokopernico>
            <img src=https://avatars.githubusercontent.com/u/111400483?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=joaokopernico/>
            <br />
            <sub style="font-size:14px"><b>joaokopernico</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/JobasFernandes>
            <img src=https://avatars.githubusercontent.com/u/26033148?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Joseph Fernandes/>
            <br />
            <sub style="font-size:14px"><b>Joseph Fernandes</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/renancesarti-cyber>
            <img src=https://avatars.githubusercontent.com/u/235291917?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=renancesarti-cyber/>
            <br />
            <sub style="font-size:14px"><b>renancesarti-cyber</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/ruben18salazar3>
            <img src=https://avatars.githubusercontent.com/u/86245508?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Rubén Salazar/>
            <br />
            <sub style="font-size:14px"><b>Rubén Salazar</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/ryanachdiadsyah>
            <img src=https://avatars.githubusercontent.com/u/165612793?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Ryan Achdiadsyah/>
            <br />
            <sub style="font-size:14px"><b>Ryan Achdiadsyah</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/ViFigueiredo>
            <img src=https://avatars.githubusercontent.com/u/67883343?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=ViFigueiredo/>
            <br />
            <sub style="font-size:14px"><b>ViFigueiredo</b></sub>
        </a>
    </td>
</tr>
<tr>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/cadao7>
            <img src=https://avatars.githubusercontent.com/u/306330?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=Ricardo Maminhak/>
            <br />
            <sub style="font-size:14px"><b>Ricardo Maminhak</b></sub>
        </a>
    </td>
    <td align="center" style="word-wrap: break-word; width: 150.0; height: 150.0">
        <a href=https://github.com/zennnez>
            <img src=https://avatars.githubusercontent.com/u/3524740?v=4 width="100;"  style="border-radius:50%;align-items:center;justify-content:center;overflow:hidden;padding-top:10px" alt=zen/>
            <br />
            <sub style="font-size:14px"><b>zen</b></sub>
        </a>
    </td>
</tr>
</table>

## Clients

- [wuzapi TypeScript / Node Client](https://github.com/gusnips/wuzapi-node)

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=asternic/wuzapi&type=Date)](https://www.star-history.com/#asternic/wuzapi&Date)

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
