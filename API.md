# API Reference

The API supports two authentication methods:

1. **User Token**: For regular endpoints, use the `Authorization` header with the user's token value.
2. **Admin Token**: For admin endpoints (/admin/**), use the `Authorization` header with the admin token value (set in WUZAPI_ADMIN_TOKEN).

### Request Requirements

* Content-Type: application/json (JSON-encoded body)
* Authentication: Include the `Authorization` header in all requests.

---

## Admin Endpoints (User Management)

The following admin-only endpoints are used to manage users in the system. All require the Authorization header with the admin token (WUZAPI_ADMIN_TOKEN).


## List All Users

*GET /admin/users*

Returns a list of registered users.

Example Request:
```
curl -s -X GET -H 'Authorization: {{WUZAPI_ADMIN_TOKEN}}' http://localhost:8080/admin/users
```

Response:

```json
[
  {
    "id": 1,
    "name": "admin",
    "token": "H4Zbhwr72PBrtKdTIgS",
    "webhook": "https://example.com/webhook",
    "jid": "5491155553934@s.whatsapp.net",
    "qrcode": "",
    "connected": true,
    "expiration": 0,
    "events": "Message,ReadReceipt"
  }
]
```

## Add User

*POST /admin/users*

Adds a new user

Example Request:
```
curl -s -X POST -H 'Authorization: {{WUZAPI_ADMIN_TOKEN}}' -H 'Content-Type: application/json' --data '{"name":"usuario2","token":"token2","webhook":"https://example.com/webhook2","events":"Message,ReadReceipt"}' http://localhost:8080/admin/users
```

Response:

```json
{
  "id": 2
}
```
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

## Delete User 

*DELETE /admin/users/{id}*

Deletes one user from the system by ID

Example Request:
```
curl -s -X DELETE -H 'Authorization: {{WUZAPI_ADMIN_TOKEN}}' http://localhost:8080/admin/users/2
```

Response:

```json
{
  "Details": "User deleted successfully"
}
```

---

## Webhook

The following _webhook_ endpoints are used to get or set the webhook that will be called whenever a message or event is received. Available event types are:

* Message
* ReadReceipt
* HistorySync
* ChatPresence


## Sets webhook

Configures the webhook to be called using POST whenever a subscribed event occurs.

Endpoint: _/webhook_

Method: **POST**


```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"webhookURL":"https://some.server/webhook"}' http://localhost:8080/webhook
```
Response:

```json
{ 
  "code": 200, 
  "data": { 
    "webhook": "https://example.net/webhook" 
  }, 
  "success": true 
}
```

---

## Gets webhook

Retrieves the configured webhook and subscribed events.

Endpoint: _/webhook_

Method: **GET**

```
curl -s -X GET -H 'Token: 1234ABCD' http://localhost:8080/webhook
```
Response:
```json
{ 
  "code": 200, 
  "data": { 
    "subscribe": [ "Message" ], 
    "webhook": "https://example.net/webhook" 
  }, 
  "success": true 
}
```

---

## HMAC Configuration

The following _HMAC_ endpoints are used to configure and manage HMAC keys for webhook security. HMAC signatures verify that webhooks are authentic and haven't been tampered with.

### Security Notes:

* HMAC keys must be at least 32 characters long for security
* Once saved, HMAC keys cannot be retrieved or viewed
* All webhooks will include an `x-hmac-signature` header when HMAC is configured
* Webhooks are signed using SHA-256 HMAC

### Signature Generation by Content-Type:

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

---

## Configure HMAC Key

Configures HMAC key for webhook signing to verify webhook authenticity.

Endpoint: _/session/hmac/config_

Method: **POST**

**Headers:**

* `Authorization: {user_token}`
* `Content-Type: application/json`

**Request Body:**

```json
{
  "hmac_key": "your_hmac_key_minimum_32_characters_long_here"
}
```

**Example Request:**

```
curl -s -X POST -H 'Authorization: 1234ABCD' -H 'Content-Type: application/json' --data '{"hmac_key":"your_hmac_key_minimum_32_characters_long_here"}' http://localhost:8080/session/hmac/config
```

**Response:**

```json
{
  "code": 200,
  "data": {
    "Details": "HMAC configuration saved successfully"
  },
  "success": true
}
```

**Error Responses:**

* `400 Bad Request`: HMAC key less than 32 characters
* `500 Internal Server Error`: Failed to save configuration

---

## Get HMAC Configuration Status

Retrieves HMAC configuration status (does not return the actual key for security reasons).

Endpoint: _/session/hmac/config_

Method: **GET**

**Headers:**

* `Authorization: {user_token}`

**Example Request:**

```
curl -s -X GET -H 'Authorization: 1234ABCD' http://localhost:8080/session/hmac/config
```

**Response:**

```json
{
  "hmac_key": "***"
}
```

---

## Delete HMAC Configuration

Removes HMAC configuration and disables webhook signing.

Endpoint: _/session/hmac/config_

Method: **DELETE**

**Headers:**

* `Authorization: {user_token}`

**Example Request:**

```
curl -s -X DELETE -H 'Authorization: 1234ABCD' http://localhost:8080/session/hmac/config
```

**Response:**

```json
{
  "code": 200,
  "data": {
    "Details": "HMAC configuration deleted successfully"
  },
  "success": true
}
```

---

## Session

The following _session_ endpoints are used to start a session to Whatsapp servers in order to send and receive messages

## Connect  

Connects to Whatsapp servers. If is there no existing session it will initiate a QR scan that can be retrieved via the [/session/qr](#user-content-gets-qr-code) endpoint. 
You can subscribe to different types of messages so they are POSTED to your configured webhook. 
Available message types to subscribe to are: 

* Message
* ReadReceipt
* HistorySync
* ChatPresence

If you set Immediate to false, the action will wait 10 seconds to verify a successful login. If Immediate is not set or set to true, it will return immedialty, but you will have to check shortly after the /session/status as your session might be disconnected shortly after started if the session was terminated previously via the phone/device.

Endpoint: _/session/connect_

Method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Subscribe":["Message"],"Immediate":false}' http://localhost:8080/session/connect 
```

Response:

```json
{
  "code": 200,
  "data": {
    "details": "Connected!",
    "events": "Message",
    "jid": "5491155554444.0:52@s.whatsapp.net",
    "webhook": "http://some.site/webhook?token=123456"
  },
  "success": true
}
```

---

## Disconnect

Disconnects from Whatsapp servers, keeping the session active. This means that if you /session/connect again, it will
reuse the session and won't require a QR code rescan.

Event subscriptions are **preserved by default**. Pass `clear=true` to also reset them on disconnect.

Endpoint: _/session/disconnect_

Method: **POST**

Query parameters:

* `clear` (optional, boolean): if `true`, clears the user's event subscriptions on disconnect. Defaults to `false` (subscriptions preserved).


```
curl -s -X POST -H 'Token: 1234ABCD' http://localhost:8080/session/disconnect 
```

Response: 

```json
{
  "code": 200,
  "data": {
    "Details": "Disconnected"
  },
  "success": true
}
```

---

## Logout

Disconnects from whatsapp websocket *and* finishes the session (so it will be required to scan a  QR code the next time a connection is initiated)

Endpoint: _/session/logout_

Method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' http://localhost:8080/session/logout 
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Logged out"
  },
  "success": true
}

```

---

## Status

Retrieve status (IsConnected means websocket connection is initiated, IsLoggedIn means QR code was scanned and session is ready to receive/send messages)

If its not logged in, you can use the [/session/qr](#user-content-gets-qr-code) endpoint to get the QR code to scan

Endpoint: _/session/status_

Method: **GET**

```
curl -s -H 'Token: 1234ABCD' http://localhost:8080/session/status 
```

Response:

```json
{
  "code": 200,
  "data": {
    "Connected": true,
    "LoggedIn": true
  },
  "success": true
}

```

---

## Gets QR code  

Retrieves QR code, session must be connected to Whatsapp servers and logged in must be false in order for the QR code to be generated. The generated code
will be returned encoded in base64 embedded format.

Endpoint: _/session/qr_

Method: **GET**

```
curl -s -H 'Token: 1234ABCD' http://localhost:8080/session/qr
```
Response:
```json
{ 
  "code": 200, 
  "data": { 
    "QRCode": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAQAAAAEAAQMAAABmvDolAAAABlBMVEX///8AAABVwtN+AAAEw0lEQVR42uyZ..." 
  }, 
  "success": true 
}
```

---

## User

The following _user_ endpoints are used to gather information about Whatsapp users.

## Gets user details

Gets information for users on Whatsapp

Endpoint: _/user/info_

Method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":["5491155554445","5491155554444"]}' http://localhost:8080/user/info 
```

Response:

```json
{
  "code": 200,
  "data": {
    "Users": {
      "5491155554445@s.whatsapp.net": {
        "Devices": [],
        "PictureID": "",
        "Status": "",
        "VerifiedName": null
      },
      "5491155554444@s.whatsapp.net": {
        "Devices": [
          "5491155554444.0:0@s.whatsapp.net",
          "5491155554444.0:11@s.whatsapp.net"
        ],
        "PictureID": "",
        "Status": "",
        "VerifiedName": {
          "Certificate": {
            "details": "CP7t782FIRIGc21iOndeshIghUcml4b2NvbQ==",
            "signature": "e35Fd320dccNmaBdNw+Yqtz1Q5545XpT9PpSlntqwaXpj1boOrQUnq9TNhYzGtgPWznTjRl7kHEBQ=="
          },
          "Details": {
            "issuer": "smb:wa",
            "serial": 23810327841439764000,
            "verifiedName": "Great Company"
          }
        }
      }
    }
  },
  "success": true
}
```

---

## Checks Users

Checks if phone numbers are registered as Whatsapp users

Endpoint: _/user/check_

Method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":["5491155554445","5491155554444"]}' http://localhost:8080/user/check
```

Response:

```json
{
  "code": 200,
  "data": {
    "Users": [
      {
        "IsInWhatsapp": true,
        "JID": "5491155554445@s.whatsapp.net",
        "Query": "5491155554445",
        "VerifiedName": "Company Name"
      },
      {
        "IsInWhatsapp": false,
        "JID": "5491155554444@s.whatsapp.net",
        "Query": "5491155554444",
        "VerifiedName": ""
      }
    ]
  },
  "success": true
}
```

---

## Gets Avatar

Gets information about users profile pictures on WhatsApp, either a thumbnail (Preview=true) or full picture.

Endpoint: _/user/avatar_

Method: **GET**

```
curl -s -X GET -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554445","Preview":true]}' http://localhost:8080/user/avatar
```

Response:

```json
{
  "URL": "https://pps.whatsapp.net/v/t61.24694-24/227295214_112447507729487_4643695328050510566_n.jpg?stp=dst-jpg_s96x96&ccb=11-4&oh=ja432434a91e8f41d86d341bx889c217&oe=543222A4",
  "ID": "1645308319",
  "Type": "preview",
  "DirectPath": "/v/t61.24694-24/227295214_112447507729487_4643695328050510566_n.jpg?stp=dst-jpg_s96x96&ccb=11-4&oh=ja432434a91e8f41d86d341ba889c217&oe=543222A4"
}
```

---

## Gets all contacts

Gets all contacts for the account.

Endpoint: _/user/contacts_

Method: **GET**

```
curl -s -X GET -H 'Token: 1234ABCD' http://localhost:8080/user/contacts
```

Response:

```json
{
  "code": 200,
  "data": {
    "5491122223333@s.whatsapp.net": {
      "BusinessName": "",
      "FirstName": "",
      "Found": true,
      "FullName": "",
      "PushName": "FOP2"
    },
    "549113334444@s.whatsapp.net": {
      "BusinessName": "",
      "FirstName": "",
      "Found": true,
      "FullName": "",
      "PushName": "Asternic"
    }
  }
}
```

---

## Get Privacy Settings

Returns the account's current privacy settings.

Endpoint: _/user/privacy_

Method: **GET**

```
curl -s -X GET -H 'Token: 1234ABCD' http://localhost:8080/user/privacy
```

Response:

```json
{
  "code": 200,
  "data": {
    "GroupAdd": "contacts",
    "LastSeen": "all",
    "Status": "contacts",
    "Profile": "all",
    "ReadReceipts": "all",
    "CallAdd": "all",
    "Online": "all",
    "Messages": "all",
    "Defense": "off",
    "Stickers": "contacts"
  },
  "success": true
}
```

---

## Set Privacy Setting

Updates a single privacy setting.

Endpoint: _/user/privacy_

Method: **POST**

Valid `Name` / `Value` combinations:

| Name | Valid values |
|------|--------------|
| `groupadd`, `last`, `status`, `profile` | `all`, `contacts`, `contact_blacklist`, `none` |
| `readreceipts` | `all`, `none` |
| `online` | `all`, `match_last_seen` |
| `calladd` | `all`, `known` |

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Name":"last","Value":"contacts"}' http://localhost:8080/user/privacy
```

Response:

```json
{
  "code": 200,
  "data": {
    "LastSeen": "contacts"
  },
  "success": true
}
```

---

## Block User

Blocks a WhatsApp user and returns the updated blocklist.

Endpoint: _/user/block_

Method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554445"}' http://localhost:8080/user/block
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "User blocked",
    "JID": "5491155554445@s.whatsapp.net",
    "Blocklist": [
      "5491155554445@s.whatsapp.net"
    ],
    "DHash": "1234567890"
  },
  "success": true
}
```

---

## Unblock User

Unblocks a WhatsApp user and returns the updated blocklist.

Endpoint: _/user/unblock_

Method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554445"}' http://localhost:8080/user/unblock
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "User unblocked",
    "JID": "5491155554445@s.whatsapp.net",
    "Blocklist": [],
    "DHash": "1234567891"
  },
  "success": true
}
```

---

## Get Blocklist

Returns the list of WhatsApp users currently blocked by the session.

Endpoint: _/user/blocklist_

Method: **GET**

```
curl -s -X GET -H 'Token: 1234ABCD' http://localhost:8080/user/blocklist
```

Response:

```json
{
  "code": 200,
  "data": {
    "Blocklist": [
      "5491155554445@s.whatsapp.net"
    ],
    "DHash": "1234567890"
  },
  "success": true
}
```

---


# Chat

The following _chat_ endpoints are used to send messages or mark them as read or indicating composing/not composing presence. The sample response is listed only once, as it is the
same for all message types.

## Send Text Message

Sends a text message or reply. For replies, ContextInfo data should be completed with the StanzaID (ID of the message we are replying to), and Participant (user JID we are replying to). If ID is 
ommited, a random message ID will be generated.

Endpoint: _/chat/send/text_

Method: **POST**

Example sending a new message:

```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Body":"Hellow Meow", "Id": "90B2F8B13FAC8A9CF6B06E99C7834DC5"}' http://localhost:8080/chat/send/text
```

Example sending a new message with link preview
```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Body":"Check my site? https://example.com", "Id": "90B2F8B13FAC8A9CF6B06E99C7834DC5","LinkPreview": true}' http://localhost:8080/chat/send/text
```

Example replying to some message:

```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Body":"Ditto","ContextInfo":{"StanzaId":"AA3DSE28UDJES3","Participant":"5491155553935@s.whatsapp.net"}}' http://localhost:8080/chat/send/text
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Sent",
    "Id": "90B2F8B13FAC8A9CF6B06E99C7834DC5",
    "Timestamp": "2022-04-20T12:49:08-03:00"
  },
  "success": true
}
```

---

## Send Template Message

Sends a template message or reply. Template messages can contain call to action buttons: up to three quick replies, call button, and link button.

Endpoint: _/chat/send/template_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Content":"Template content","Footer":"Some footer text","Buttons":[{"DisplayText":"Yes","Type":"quickreply"},{"DisplayText":"No","Type":"quickreply"},{"DisplayText":"Visit Site","Type":"url","Url":"https://www.fop2.com"},{"DisplayText":"Llamame","Type":"call","PhoneNumber":"1155554444"}]}' http://localhost:8080/chat/send/template
```

---

## Send Audio Message

Sends an Audio message. Audio must be in Opus format and base64 encoded in embedded format.

Endpoint: _/chat/send/audio_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Audio":"data:audio/ogg;base64,T2dnUw..."}' http://localhost:8080/chat/send/audio
```

## Send Image Message

Sends an Image message. Image must be in png or jpeg and base64 encoded in embedded format. You can optionally specify a text Caption 

Endpoint: _/chat/send/image_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Caption":"Look at this", "Image":"data:image/jpeg;base64,iVBORw0KGgoAAAANSU..."}' http://localhost:8080/chat/send/image
```

---

## Send Document Message

Sends a Document message. Any mime type can be attached. A FileName must be supplied in the request body. The Document must be passed as octet-stream in base64 embedded format.

Endpoint: _/chat/send/document_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","FileName":"hola.txt","Document":"data:application/octet-stream;base64,aG9sYSBxdWUgdGFsCg=="}' http://localhost:8080/chat/send/document
```

---

## Send Video Message

Sends a Video message. Video must be in mp4 or 3gpp and base64 encoded in embedded format. You can optionally specify a text Caption and a JpegThumbnail

Endpoint: _/chat/send/video_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Caption":"Look at this", "Video":"data:image/jpeg;base64,iVBORw0KGgoAAAANSU..."}' http://localhost:8080/chat/send/video
```


---

## Send Sticker Message

Sends a Sticker message. The API accepts:
- **Static stickers**: `image/webp`
- **Animated stickers**: `video/mp4`

The sticker data must be base64 encoded in data URI format (e.g., `data:image/webp;base64,...`).

Endpoint: _/chat/send/sticker_

Method: **POST**


Static WebP sticker:
```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Sticker":"data:image/webp;base64,iVBORw0KGgoAAAANSU..."}' http://localhost:8080/chat/send/sticker
```

Static WebP sticker with pack metadata:
```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{
  "Phone":"5491155554444",
  "Sticker":"data:image/webp;base64,iVBORw0KGgoAAAANSU...",
  "PackId":"com.example.my.pack",
  "PackName":"My Pack",
  "PackPublisher":"Wuzapi",
  "Emojis":["😂","😍","👍","🎉"],
  "PngThumbnail":"data:image/png;base64,iVBORw0KGgoAAAANSU..."
}' http://localhost:8080/chat/send/sticker
```

Animated sticker:
```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Sticker":"data:video/mp4;base64,AAAAIGZ0eXBpc29t..."}' http://localhost:8080/chat/send/sticker
```


---

## Send Location Message

Sends a Location message. Latitude and Longitude must be passed, with an optional Name

Endpoint: _/chat/send/location_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Latitude":48.858370,"Longitude":2.294481,"Phone":"5491155554444","Name":"Paris"}' http://localhost:8080/chat/send/location
```

---

## Send Contact Message

Sends a Contact message. Both Vcard and Name body parameters are mandatory.

Endpoint: _/chat/send/contact_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Name":"Casa","Vcard":"BEGIN:VCARD\nVERSION:3.0\nN:Doe;John;;;\nFN:John Doe\nORG:Example.com Inc.;\nTITLE:Imaginary test person\nEMAIL;type=INTERNET;type=WORK;type=pref:johnDoe@example.org\nTEL;type=WORK;type=pref:+1 617 555 1212\nTEL;type=WORK:+1 (617) 555-1234\nTEL;type=CELL:+1 781 555 1212\nTEL;type=HOME:+1 202 555 1212\nitem1.ADR;type=WORK:;;2 Enterprise Avenue;Worktown;NY;01111;USA\nitem1.X-ABADR:us\nitem2.ADR;type=HOME;type=pref:;;3 Acacia Avenue;Hoitem2.X-ABADR:us\nEND:VCARD"}' http://localhost:8080/chat/send/contact
```

---

## Chat Presence Indication

Sends indication if you are writing/composing a text or audio message to the other party. possible states are "composing" and "paused". if media is set to "audio" it will indicate an audio message is being recorded.

endpoint: _/chat/presence_

method: **POST**

```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","State":"composing","Media":""}' http://localhost:8080/chat/presence
```

---

## Subscribe to Contact Presence

Subscribes to a contact's presence updates (online/offline and last seen). After
subscribing, your configured webhook receives `Presence` events for that contact. You
should be online yourself to receive presence (wuzapi sends an available presence on
connect). Whether `last_seen` is available depends on the contact's privacy settings.

endpoint: _/user/presence/subscribe_

method: **POST**

```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444"}' http://localhost:8080/user/presence/subscribe
```

The resulting webhook `Presence` event looks like:

```json
{
  "type": "Presence",
  "state": "offline",
  "from": "5491155554444@s.whatsapp.net",
  "last_seen": 1750000000
}
```

`last_seen` is a Unix timestamp, present only when the contact is offline and shares
their last seen. When the contact is online, only `type`, `from` and `state` are sent.

---

## Mark message(s) as read

Indicates that one or more messages were read. Id is an array of messages Ids.
The endpoint now supports two methods for chat identification to ensure backward compatibility:

1. New Standard: Use ChatPhone and SenderPhone (recommended).
2. Legacy: Use Chat and Sender (accepts full JID format).

### Priority Logic

The API processes the IDs using the following priority: `ChatPhone` > `Chat` (and `SenderPhone` > `Sender`).

endpoint: _/chat/markread_

method: **POST**

```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Id":["AABBCCDD112233", "IIOOPPLL43332"], "ChatPhone":"5491155553934", "SenderPhone":"5491155553935"}' http://localhost:8080/chat/markread
```

---

## React to messages

Sends a reaction for an existing message. Id is the message Id to react to, if its your own message, prefix the Id with the string 'me:'

endpoint: _/chat/react_

method: **POST**

```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","Body":"❤️","Id":"me:069EDE53E81CB5A4773587FB96CB3ED3"}' http://localhost:8080/chat/react
```

---

## Download Image

Downloads an Image from a message and retrieves it Base64 media encoded. Required request parameters are: Url, MediaKey, Mimetype, FileSHA256 and FileLength

endpoint: _/chat/downloadimage_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Url":"https://mmg.whatsapp.net/d/f/Apah954sUug5I9GnQsmXKPUdUn3ZPKGYFnscJU02dpuD.enc","Mimetype":"image/jpeg", "FileSHA256":"nMthnfkUWQiMfNJpA6K9+ft+Dx9Mb1STs+9wMHjeo/M=","FileLength":2039,"MediaKey":"vq0RR0nYGkxm2HrpwUp3sK8A7Nr1KUcOiBHrT1hg+PU=","FileEncSHA256":"6bMVZ5dRf9JKxJSUgg4w1h3iSYA3dM8gEQxaMPwoONc="}' http://localhost:8080/chat/downloadimage
```

---

## Download Video

Downloads a Video from a message and retrieves it Base64 media encoded. Required request parameters are: Url, MediaKey, Mimetype, FileSHA256 and FileLength

endpoint: _/chat/downloadvideo_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Url":"https://mmg.whatsapp.net/d/f/Apah954sUug5I9GnQsmXKPUdUn3ZPKGYFnscJU02dpuD.enc","Mimetype":"video/mp4", "FileSHA256":"nMthnfkUWQiMfNJpA6K9+ft+Dx9Mb1STs+9wMHjeo/M=","FileLength":2039,"MediaKey":"vq0RR0nYGkxm2HrpwUp3sK8A7Nr1KUcOiBHrT1hg+PU=","FileEncSHA256":"6bMVZ5dRf9JKxJSUgg4w1h3iSYA3dM8gEQxaMPwoONc="}' http://localhost:8080/chat/downloadvideo
```

---

## Download Audio

Downloads an Audio from a message and retrieves it Base64 media encoded. Required request parameters are: Url, MediaKey, Mimetype, FileSHA256 and FileLength

endpoint: _/chat/downloadaudio_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Url":"https://mmg.whatsapp.net/d/f/Apah954sUug5I9GnQsmXKPUdUn3ZPKGYFnscJU02dpuD.enc","Mimetype":"audio/ogg; codecs=opus", "FileSHA256":"nMthnfkUWQiMfNJpA6K9+ft+Dx9Mb1STs+9wMHjeo/M=","FileLength":2039,"MediaKey":"vq0RR0nYGkxm2HrpwUp3sK8A7Nr1KUcOiBHrT1hg+PU=","FileEncSHA256":"6bMVZ5dRf9JKxJSUgg4w1h3iSYA3dM8gEQxaMPwoONc="}' http://localhost:8080/chat/downloadaudio
```

---

## Download Document

Downloads a Document from a message and retrieves it Base64 media encoded. Required request parameters are: Url, MediaKey, Mimetype, FileSHA256 and FileLength

endpoint: _/chat/downloaddocument_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Url":"https://mmg.whatsapp.net/d/f/Apah954sUug5I9GnQsmXKPUdUn3ZPKGYFnscJU02dpuD.enc","Mimetype":"application/pdf", "FileSHA256":"nMthnfkUWQiMfNJpA6K9+ft+Dx9Mb1STs+9wMHjeo/M=","FileLength":2039,"MediaKey":"vq0RR0nYGkxm2HrpwUp3sK8A7Nr1KUcOiBHrT1hg+PU=","FileEncSHA256":"6bMVZ5dRf9JKxJSUgg4w1h3iSYA3dM8gEQxaMPwoONc="}' http://localhost:8080/chat/downloaddocument
```

---

## Group

The following _group_ endpoints are used to gather information or perfrom actions in chat groups.

## List subscribed groups

Returns complete list of subscribed groups

endpoint: _/group/list_

method: **GET**


```
curl -s -X GET -H 'Token: 1234ABCD' http://localhost:8080/group/list 
```

Response:
```json
{
  "code": 200,
  "data": {
    "Groups": [
      {
        "AnnounceVersionID": "1650572126123738",
        "DisappearingTimer": 0,
        "GroupCreated": "2022-04-21T17:15:26-03:00",
        "IsAnnounce": false,
        "IsEphemeral": false,
        "IsLocked": false,
        "JID": "120362023605733675@g.us",
        "Name": "Super Group",
        "NameSetAt": "2022-04-21T17:15:26-03:00",
        "NameSetBy": "5491155554444@s.whatsapp.net",
        "OwnerJID": "5491155554444@s.whatsapp.net",
        "ParticipantVersionID": "1650234126145738",
        "Participants": [
          {
            "IsAdmin": true,
            "IsSuperAdmin": true,
            "JID": "5491155554444@s.whatsapp.net"
          },
          {
            "IsAdmin": false,
            "IsSuperAdmin": false,
            "JID": "5491155553333@s.whatsapp.net"
          },
          {
            "IsAdmin": false,
            "IsSuperAdmin": false,
            "JID": "5491155552222@s.whatsapp.net"
          }
        ],
        "Topic": "",
        "TopicID": "",
        "TopicSetAt": "0001-01-01T00:00:00Z",
        "TopicSetBy": ""
      }
    ]
  },
  "success": true
}
```

---

## Get group invite link

Gets the invite link for a group

endpoint: _/group/invitelink_

method: **GET**


```
curl -s -X GET -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"GroupJID":"120362023605733675@g.us"}' http://localhost:8080/group/invitelink 
```

Response: 

```json
{
  "code": 200,
  "data": {
    "InviteLink": "https://chat.whatsapp.com/HffXhYmzzyJGec61oqMXiz"
  },
  "success": true
}
```

---

## Gets group information

Retrieves information about a specific group

endpoint: _/group/info_

method: **GET**


```
curl -s -X GET -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"GroupJID":"120362023605733675@g.us"}' http://localhost:8080/group/info
```

Response: 

```json
{
  "code": 200,
  "data": {
    "AnnounceVersionID": "1650572126123738",
    "DisappearingTimer": 0,
    "GroupCreated": "2022-04-21T17:15:26-03:00",
    "IsAnnounce": false,
    "IsEphemeral": false,
    "IsLocked": false,
    "JID": "120362023605733675@g.us",
    "Name": "Super Group",
    "NameSetAt": "2022-04-21T17:15:26-03:00",
    "NameSetBy": "5491155554444@s.whatsapp.net",
    "OwnerJID": "5491155554444@s.whatsapp.net",
    "ParticipantVersionID": "1650234126145738",
    "Participants": [
      {
        "IsAdmin": true,
        "IsSuperAdmin": true,
        "JID": "5491155554444@s.whatsapp.net"
      },
      {
        "IsAdmin": false,
        "IsSuperAdmin": false,
        "JID": "5491155553333@s.whatsapp.net"
      },
      {
        "IsAdmin": false,
        "IsSuperAdmin": false,
        "JID": "5491155552222@s.whatsapp.net"
      }
    ],
    "Topic": "",
    "TopicID": "",
    "TopicSetAt": "0001-01-01T00:00:00Z",
    "TopicSetBy": ""
  },
  "success": true
}
```

---

## Changes group photo

Allows you to change a group photo/image. **WhatsApp only accepts JPEG format for group photos.**

endpoint: _/group/photo_

method: **POST**

**Parameters:**
- `GroupJID`: The JID of the group
- `Image`: Base64 encoded JPEG image data with data URL format (must be "data:image/jpeg;base64,...")

**Important Notes:**
- Only JPEG format is supported (WhatsApp requirement)
- Image will be automatically resized if too large
- Transparent images will be converted to JPEG with white background

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"GroupJID":"120362023605733675@g.us","Image":"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD..."}' http://localhost:8080/group/photo 
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Group Photo set successfully",
    "PictureID": "122233212312"
  },
  "success": true
}
```


---

## Changes group name

Allows you to change a group name

endpoint: _/group/name_

method: **POST**



```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"GroupJID":"120362023605733675@g.us","Name":"New Group Name"}' http://localhost:8080/group/name 
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Group Name set successfully"
  },
  "success": true
}
```

---

## Create group

Creates a new WhatsApp group with specified name and participants

endpoint: _/group/create_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"name":"My New Group","participants":["5491155553934","5491155553935"]}' http://localhost:8080/group/create 
```

Response:

```json
{
  "code": 200,
  "data": {
    "JID": "120363123456789@g.us",
    "Name": "My New Group",
    "OwnerJID": "5491155554444@s.whatsapp.net",
    "GroupCreated": "2023-12-01T10:00:00Z",
    "Participants": [
      {
        "IsAdmin": true,
        "IsSuperAdmin": true,
        "JID": "5491155554444@s.whatsapp.net"
      },
      {
        "IsAdmin": false,
        "IsSuperAdmin": false,
        "JID": "5491155553934@s.whatsapp.net"
      }
    ]
  },
  "success": true
}
```

---

## Set group locked status

Configures whether only admins can modify group info (locked) or all participants can modify (unlocked)

endpoint: _/group/locked_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"groupjid":"120362023605733675@g.us","locked":true}' http://localhost:8080/group/locked 
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Group locked setting updated successfully"
  },
  "success": true
}
```

---

## Set disappearing timer

Configures ephemeral/disappearing messages for the group. Messages will automatically disappear after the specified duration.

endpoint: _/group/ephemeral_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"groupjid":"120362023605733675@g.us","duration":"24h"}' http://localhost:8080/group/ephemeral 
```

Valid duration values:
- `"24h"` - 24 hours
- `"7d"` - 7 days  
- `"90d"` - 90 days
- `"off"` - Disable disappearing messages

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Disappearing timer set successfully"
  },
  "success": true
}
```

---

## Remove group photo

Removes the current photo/image from the specified WhatsApp group

endpoint: _/group/photo/remove_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"groupjid":"120362023605733675@g.us"}' http://localhost:8080/group/photo/remove 
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Group photo removed successfully"
  },
  "success": true
}
```

---

## List pending join requests

Lists participants who have requested to join the group. Requires join approval mode to be enabled on the group and the session to be a group admin.

endpoint: _/group/requestparticipants_

method: **GET**

```
curl -s -X GET -H 'Token: 1234ABCD' 'http://localhost:8080/group/requestparticipants?groupJID=120362023605733675@g.us'
```

Response:

```json
{
  "code": 200,
  "data": [
    {
      "JID": "70072425046185@lid",
      "RequestedAt": "2026-05-27T00:34:40-03:00"
    }
  ],
  "success": true
}
```

---

## Approve or reject join requests

Approves or rejects participants who requested to join the group. Use the `JID` values returned by `/group/requestparticipants` in the `Phone` array.

endpoint: _/group/updaterequestparticipants_

method: **POST**

`Action` must be `"approve"` or `"reject"`.

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"GroupJID":"120362023605733675@g.us","Phone":["70072425046185@lid"],"Action":"approve"}' http://localhost:8080/group/updaterequestparticipants
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Group request participants updated successfully"
  },
  "success": true
}
```

---

## Set group join approval mode

Enables or disables the requirement that new members be approved by an admin before joining the group.

endpoint: _/group/joinapprovalmode_

method: **POST**

```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"groupjid":"120362023605733675@g.us","mode":true}' http://localhost:8080/group/joinapprovalmode
```

Response:

```json
{
  "code": 200,
  "data": {
    "Details": "Group join approval mode updated successfully"
  },
  "success": true
}
```

# S3 Storage Integration for WuzAPI

## Overview

WuzAPI now supports S3-compatible storage for media files, allowing you to store WhatsApp media (images, videos, audio, and documents) in cloud storage services instead of or in addition to base64 encoding in webhooks.

## Features

- **Multi-tenant Support**: Each user can configure their own S3 storage
- **Multiple Providers**: Support for AWS S3, MinIO, Backblaze B2, and other S3-compatible services
- **Flexible Delivery**: Choose between base64, S3 URL, or both in webhooks
- **Automatic Organization**: Files are organized by user, contact, date, and media type
- **Public Access**: Media files are stored with public-read permissions for easy preview
- **Retention Management**: Configurable retention period with automatic expiration
- **CDN Support**: Option to use custom public URLs for CDN integration

## API Endpoints

### Configure S3 Storage
```
POST /session/s3/config
```

Configure S3 storage settings for the authenticated user.

**Request Body:**
```json
{
  "enabled": true,
  "endpoint": "https://s3.amazonaws.com",
  "region": "us-east-1", 
  "bucket": "my-whatsapp-media",
  "access_key": "AKIAIOSFODNN7EXAMPLE",
  "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  "path_style": false,
  "public_url": "https://cdn.example.com",
  "media_delivery": "both",
  "retention_days": 30
}
```

**Parameters:**
- `enabled`: Enable/disable S3 storage
- `endpoint`: S3 endpoint URL (leave empty for AWS S3)
- `region`: S3 region
- `bucket`: S3 bucket name
- `access_key`: S3 access key ID
- `secret_key`: S3 secret access key
- `path_style`: Use path-style URLs (required for MinIO)
- `public_url`: Custom public URL for accessing files (optional)
- `media_delivery`: Delivery method - "base64", "s3", or "both"
- `retention_days`: Days to retain files (0 for no expiration)

### Get S3 Configuration
```
GET /session/s3/config
```

Retrieve current S3 configuration (access key is masked).

**Response:**
```json
{
  "code": 200,
  "data": {
    "enabled": true,
    "endpoint": "https://s3.amazonaws.com",
    "region": "us-east-1",
    "bucket": "my-whatsapp-media",
    "access_key": "***",
    "path_style": false,
    "public_url": "",
    "media_delivery": "both",
    "retention_days": 30
  },
  "success": true
}
```

### Test S3 Connection
```
POST /session/s3/test
```

Test S3 connection with current configuration.

**Response:**
```json
{
  "code": 200,
  "data": {
    "Details": "S3 connection test successful",
    "Bucket": "my-whatsapp-media",
    "Region": "us-east-1"
  },
  "success": true
}
```

### Delete S3 Configuration
```
DELETE /session/s3/config
```

Remove S3 configuration and revert to base64-only delivery.

## S3 Provider Examples

### AWS S3
```json
{
  "enabled": true,
  "endpoint": "",
  "region": "us-east-1",
  "bucket": "my-bucket",
  "access_key": "AKIAIOSFODNN7EXAMPLE",
  "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  "path_style": false,
  "media_delivery": "s3",
  "retention_days": 30
}
```

### MinIO
```json
{
  "enabled": true,
  "endpoint": "https://minio.example.com",
  "region": "us-east-1",
  "bucket": "whatsapp-media",
  "access_key": "minioadmin",
  "secret_key": "minioadmin",
  "path_style": true,
  "media_delivery": "both",
  "retention_days": 0
}
```

### Backblaze B2
```json
{
  "enabled": true,
  "endpoint": "https://s3.us-west-004.backblazeb2.com",
  "region": "us-west-004",
  "bucket": "my-b2-bucket",
  "access_key": "0004b0000000000000000000001",
  "secret_key": "K004XXXXXXXXXXXXXXXXXXXXXXXX",
  "path_style": false,
  "media_delivery": "s3",
  "retention_days": 90
}
```

## File Organization

Media files are stored in S3 with the following structure:

```
users/{user_id}/{inbox|outbox}/{contact_jid}/{year}/{month}/{day}/{media_type}/{message_id}.{ext}
```

Example:
```
users/abc123/inbox/5491155553934_s.whatsapp.net/2024/12/25/images/3EB06F9067F80BAB89FF.jpg
```

## Webhook Payload

When S3 is enabled, webhook payloads will include S3 information based on the `media_delivery` setting:

### S3 Only (`media_delivery: "s3"`)
```json
{
  "event": {
    "Info": { ... },
    "Message": { ... }
  },
  "s3": {
    "url": "https://my-bucket.s3.us-east-1.amazonaws.com/users/abc123/inbox/...",
    "key": "users/abc123/inbox/5491155553934/2024/12/25/images/3EB06F9067F80BAB89FF.jpg",
    "bucket": "my-bucket",
    "size": 245632,
    "mimeType": "image/jpeg",
    "fileName": "3EB06F9067F80BAB89FF.jpg"
  }
}
```

### Both S3 and Base64 (`media_delivery: "both"`)
```json
{
  "event": { ... },
  "base64": "/9j/4AAQSkZJRgABAQAAAQ...",
  "mimeType": "image/jpeg",
  "fileName": "3EB06F9067F80BAB89FF.jpg",
  "s3": {
    "url": "https://my-bucket.s3.us-east-1.amazonaws.com/users/abc123/inbox/...",
    "key": "users/abc123/inbox/5491155553934/2024/12/25/images/3EB06F9067F80BAB89FF.jpg",
    "bucket": "my-bucket",
    "size": 245632,
    "mimeType": "image/jpeg",
    "fileName": "3EB06F9067F80BAB89FF.jpg"
  }
}
```

## Bucket Policy

Ensure your S3 bucket has the appropriate policy for public read access:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "PublicReadGetObject",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::my-bucket/*"
    }
  ]
}
```

## Important Notes

1. **Security**: Store S3 credentials securely. They are encrypted in the database but should still be treated as sensitive.

2. **Costs**: S3 storage and bandwidth costs apply based on your provider's pricing.

3. **Performance**: S3 upload is synchronous. Large files may slightly delay webhook delivery.

4. **Fallback**: If S3 upload fails, the webhook is still sent (without S3 data if `media_delivery` is "s3" only).

5. **Retention**: Files are automatically deleted after the retention period if set. Use 0 for permanent storage.

6. **Public Access**: Files are stored with public-read permissions. Do not use this for sensitive data without additional security measures.

## Migration Guide

To migrate from base64-only to S3 storage:

1. Configure S3 with `media_delivery: "both"` initially
2. Update your webhook handler to support both formats
3. Test thoroughly with various media types
4. Switch to `media_delivery: "s3"` once confirmed working
5. Update webhook handler to use S3 URLs exclusively

## Troubleshooting

### Connection Test Fails
- Verify credentials are correct
- Check bucket exists and is accessible
- Ensure region is correct
- For MinIO, ensure `path_style: true` is set

### Files Not Accessible
- Check bucket policy allows public read
- Verify CORS settings if accessing from browser
- Ensure `public_url` is correct if using CDN

### Webhook Missing S3 Data
- Check `media_delivery` setting
- Verify S3 is enabled
- Check logs for upload errors
- Test S3 connection


## Webhook format configuration

Starting from version X.X.X, you can choose the format for sending webhook data using the `WEBHOOK_FORMAT` environment variable.

### Available options
- `form` (default): Sends data as `application/x-www-form-urlencoded`, with the JSON in the `jsonData` field and the token in the `token` field.
- `json`: Sends data as `application/json`, with the full event JSON as the body and the `token` field included.

### How to configure

In your terminal, set the variable before starting the service:

```bash
export WEBHOOK_FORMAT=json # or "form" for the default
```

### Payload examples

**Form mode (default):**

```
POST /webhook
Content-Type: application/x-www-form-urlencoded

jsonData={...json...}&token=YOUR_TOKEN
```

**JSON mode:**

```
POST /webhook
Content-Type: application/json

{
  "event": {...},
  "type": "ReadReceipt",
  ...
  "token": "YOUR_TOKEN"
}
```

### Notes
- The `form` mode ensures compatibility with legacy or older webhook systems.
- The `json` mode is recommended for modern integrations and easier backend parsing.
- If you do not set the variable, the system will use `form` mode by default.
