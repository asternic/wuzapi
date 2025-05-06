# WUZAPI

<img src="static/favicon.ico" width="30"> WuzAPI is an implementation 
of [@tulir/whatsmeow](https://github.com/tulir/whatsmeow) library as a 
simple RESTful API service with multiple device support and concurrent 
sessions.

Whatsmeow does not use Puppeteer on headless Chrome, nor an Android emulator. 
It talks directly to WhatsApp websocket servers, thus is quite fast and uses 
much less memory and CPU than those solutions. The drawback is that a change 
in the WhatsApp protocol could break connections and will require a library 
update.

## :warning: Warning

**Using this software violating WhatsApp ToS can get your number banned**: 
Be very careful, do not use this to send SPAM or anything like it. Use at
your own risk. If you need to develop something with commercial interest 
you should contact a WhatsApp global solution provider and sign up for the
Whatsapp Business API service instead.

## Available endpoints

* Session: connect, disconnect and logout from WhatsApp. Retrieve 
connection status. Retrieve QR code for scanning.
* Messages: send text, image, audio, document, template, video, sticker, 
location and contact messages.
* Users: check if phones have whatsapp, get user information, get user avatar, 
retrieve full contact list.
* Chat: set presence (typing/paused,recording media), mark messages as read, 
download images from messages, send reactions.
* Groups: list subscribed, get info, get invite links, change photo and name.
* Webhooks: set and get webhook that will be called whenever events/messages 
are received.

## Prerequisites

Packages:

* Go (Go Programming Language)

Optional:

* Docker (Containerization)

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

WuzAPI uses a <code>.env</code> file for configuration. Here are the required settings:

### For PostgreSQL
```
WUZAPI_ADMIN_TOKEN=your_admin_token_here
DB_USER=wuzapi
DB_PASSWORD=wuzapi
DB_NAME=wuzapi
DB_HOST=localhost
DB_PORT=5432
TZ=America/New_York
```

### For SQLite
```
WUZAPI_ADMIN_TOKEN=your_admin_token_here
TZ=America/New_York
```

Key configuration options:

* WUZAPI_ADMIN_TOKEN: Required - Authentication token for admin endpoints
* TZ: Optional - Timezone for server operations (default: UTC)
* PostgreSQL-specific options: Only required when using PostgreSQL backend


## Usage

To interact with the API, you must include the `Authorization` header in HTTP requests, containing the user's authentication token. You can have multiple users (different WhatsApp numbers) on the same server.  

* Uma referência da API Swagger em [/api](/api)
* Uma página web de exemplo para conectar e escanear códigos QR em [/login](/login) (onde você precisará passar ?token=seu_token_aqui)

A Swagger API reference at [/api](/api)

A sample web page to connect and scan QR codes at [/login](/login)

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

## API reference 

API calls should be made with content type json, and parameters sent into the
request body, always passing the Token header for authenticating the request.

Check the [API Reference](https://github.com/asternic/wuzapi/blob/main/API.md)

## Contributors
Thanks to these amazing people:
<!-- ALL-CONTRIBUTORS-LIST:START - Do not remove -->
<!-- ALL-CONTRIBUTORS-LIST:END -->

## License

Copyright &copy; 2025 Nicolás Gudiño

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


