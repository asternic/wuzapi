# API Reference

API calls should be made with content type json, and parameters sent into the request body, always passing the Token header for authenticating the request.

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

Endpoint: _/session/disconnect_

Method: **POST**


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

Sends a Sticker message. Sticker must be in image/webp format and base64 encoded in embedded format. You can optionally specify a PngThumbnail

Endpoint: _/chat/send/sticker_

Method: **POST**


```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Phone":"5491155554444","PngThumbnail":"VBORgoAANSU=", "Sticker":"data:image/jpeg;base64,iVBORw0KGgoAAAANSU..."}' http://localhost:8080/chat/send/sticker
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

## Mark message(s) as read

Indicates that one or more messages were read. Id is an array of messages Ids. 
Chat must always be set to the chat ID (user ID in DMs and group ID in group chats).
Sender must be set in group chats and must be the user ID who sent the message.

endpoint: _/chat/markread_

method: **POST**

```
curl -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' --data '{"Id":["AABBCCDD112233","IIOOPPLL43332"]","Chat":"5491155553934.0:1@s.whatsapp.net"}' http://localhost:8080/chat/markread
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
````

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

Allows you to change a group photo/image

endpoint: _/group/photo_

method: **POST**


```
curl -s -X POST -H 'Token: 1234ABCD' -H 'Content-Type: application/json' -d '{"GroupJID":"120362023605733675@g.us","Image":"data:image/jpeg;base64,AABB00DD-"}' http://localhost:8080/group/photo 
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

