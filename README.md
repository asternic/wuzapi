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

## Atualização de dependências

Este projeto utiliza a biblioteca whatsmeow para comunicação com o WhatsApp. Para atualizar a biblioteca para a versão mais recente, execute:

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

* -address  : sets the IP address to bind the server to (default 0.0.0.0)
* -port  : sets the port number (default 8080)
* -logtype : format for logs, either console (default) or json
* -wadebug : enable whatsmeow debug, either INFO or DEBUG levels are suported
* -sslcertificate : SSL Certificate File
* -sslprivatekey : SSL Private Key File
* -admintoken : your admin token to create, get, or delete users from database
* --logtype=console --color=true
* --logtype json

Example:

Para ter logs coloridos:
```
Depois:
```
./wuzapi --logtype=console --color=true
```
 (ou -color no Docker, etc.)

Para logs em JSON:
Rode:
```
./wuzapi --logtype json Nesse caso, color é irrelevante.
```

Com fuso horário:
Defina TZ=America/Sao_Paulo ./wuzapi ... no seu shell, ou no Docker Compose environment: TZ=America/Sao_Paulo.

## Usage

Na primeira execução, o sistema cria automaticamente um usuário "admin" com o token administrativo definido na variável de ambiente `WUZAPI_ADMIN_TOKEN` ou no parâmetro `-admintoken`. Este usuário pode ser usado para autenticação e para gerenciar outros usuários.

Para interagir com a API, você deve incluir o cabeçalho `Token` nas requisições HTTP, contendo o token de autenticação do usuário. Você pode ter vários usuários (diferentes números de WhatsApp) no mesmo servidor.

O daemon também serve alguns arquivos web estáticos, úteis para desenvolvimento/teste, que você pode acessar com seu navegador:

* Uma referência da API Swagger em [/api](/api)
* Uma página web de exemplo para conectar e escanear códigos QR em [/login](/login) (onde você precisará passar ?token=seu_token_aqui)

## ADMIN Actions

Você pode listar, adicionar e excluir usuários usando os endpoints de administração. Para usar essa funcionalidade, você deve configurar o token de administrador de uma das seguintes formas:

1. Definir a variável de ambiente `WUZAPI_ADMIN_TOKEN` antes de iniciar o aplicativo
   ```shell
   export WUZAPI_ADMIN_TOKEN=seu_token_seguro
   ```

2. Ou passar o parâmetro `-admintoken` na linha de comando
   ```shell
   ./wuzapi -admintoken=seu_token_seguro
   ```

Então você pode usar o endpoint `/admin/users` com o cabeçalho `Authorization` contendo o token para:
- `GET /admin/users` - Listar todos os usuários
- `POST /admin/users` - Criar um novo usuário
- `DELETE /admin/users/{id}` - Remover um usuário

O corpo JSON para criar um novo usuário deve conter:

- `name` [string] : Nome do usuário
- `token` [string] : Token de segurança para autorizar/autenticar este usuário
- `webhook` [string] : URL para enviar eventos via POST (opcional)
- `events` [string] : Lista de eventos separados por vírgula a serem recebidos (opcional) - Eventos válidos são: "Message", "ReadReceipt", "Presence", "HistorySync", "ChatPresence", "All"
- `expiration` [int] : Timestamp de expiração (opcional, não é aplicado pelo sistema)

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


