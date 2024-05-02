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

## Building

```
go build .
```

## Run

By default it will start a REST service in port 8080. These are the parameters
you can use to alter behaviour

* -address  : sets the IP address to bind the server to (default 0.0.0.0)
* -port  : sets the port number (default 8080)
* -logtype : format for logs, either console (default) or json
* -wadebug : enable whatsmeow debug, either INFO or DEBUG levels are suported
* -sslcertificate : SSL Certificate File
* -sslprivatekey : SSL Private Key File
* -admintoken : your admin token to create, get, or delete users from database

Example:

```
./wuzapi -logtype json
```

## Usage

In order to open up sessions, you first need to create a user and set an
authentication token for it. You can do so by updating the SQLite _users.db_
database:

``` 
sqlite3 dbdata/users.db "insert into users ('name','token') values ('John','1234ABCD')" 
```

Once you have some users created, you can talk to the API passing the **Token**
header as a simple means of authentication. You can have several users
(different numbers), on the same server.

The daemon also serves some static web files, useful for development/testing
that you can load with your browser:

* An API swagger reference in [/api](/api) A sample web page to connect and
* scan QR codes in [/login](/login) (where you will need to pass
?token=1234ABCD)

## ADMIN Actions

You can also list, add and delete users using an admin enpoint. In order to
use it you must either pass the -admintoken parameter on the command line when
starting wuzapi, or set the enviornment variable WUZAPI\_ADMIN\_TOKEN

Then you can use the /admin/users endpoint to GET the list of users, you can
POST to /admin/users to create a new user, or you can DELETE to /admin/users/{id}
to remove one. You need to set the header Authorization and pass the token
defined either via environment or command line.

The JSON body to create a new user must contain:

- name [string] : User name
- token [string] : Security token for authorizing/authenticating this user
- webhook [string] : URL to send events via POST
- events [string] : comma separated list of events to receive, valid events are: "Message", "ReadReceipt", "Presence", "HistorySync", "ChatPresence", "All"
- expiration [int] : Some expiration timestamp, it is not enforced not used by the daemon

## API reference 

API calls should be made with content type json, and parameters sent into the
request body, always passing the Token header for authenticating the request.

Check the [API Reference](https://github.com/asternic/wuzapi/blob/main/API.md)

## License

Copyright &copy; 2022 Nicolás Gudiño

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


