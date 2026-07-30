# Novas funcionalidades disponíveis após o merge do upstream

Este documento lista o que passou a existir no fork depois do merge de 139 commits
do [asternic/wuzapi](https://github.com/asternic/wuzapi) (branch `merge-upstream`).

Tudo aqui **já está implementado e compilando** — o objetivo do documento é servir de
referência para consumir essas capacidades na nossa aplicação. Nenhuma rota ou payload
pré-existente do fork foi alterado: o contrato atual da API continua válido.

Índice:

- [1. Rotas novas](#1-rotas-novas)
- [2. Mudanças em rotas existentes](#2-mudanças-em-rotas-existentes)
- [3. Payloads de webhook enriquecidos](#3-payloads-de-webhook-enriquecidos)
- [4. Correções relevantes para o consumidor](#4-correções-relevantes-para-o-consumidor)
- [5. Configuração nova](#5-configuração-nova)
- [6. Sugestão de priorização](#6-sugestão-de-priorização)

---

## 1. Rotas novas

Todas usam o mesmo esquema de autenticação das demais (header `token`) e o mesmo
envelope de resposta (`{ "code": 200, "data": { ... }, "success": true }`).

### 1.1 Bloqueio de contatos

| Método | Rota              | Descrição                        |
| ------ | ----------------- | -------------------------------- |
| POST   | `/user/block`     | Bloqueia um contato              |
| POST   | `/user/unblock`   | Desbloqueia um contato           |
| GET    | `/user/blocklist` | Lista os contatos bloqueados     |

**POST /user/block** e **/user/unblock** — aceitam `Phone` **ou** `JID` (o que vier
preenchido; `JID` tem precedência):

```json
{ "Phone": "5511999999999" }
```

Resposta:

```json
{
  "Details": "User blocked",
  "JID": "5511999999999@s.whatsapp.net",
  "Blocklist": ["5511999999999@s.whatsapp.net"],
  "DHash": "1234567890"
}
```

Detalhes úteis:

- Aceita LID (`...@lid`) como entrada — é resolvido internamente para o número real
  antes de bloquear. Quando isso acontece, a resposta traz também `RequestedJID` com
  o valor original enviado.
- `Blocklist` nunca vem `null` — lista vazia quando não há bloqueados.
- A lista retornada já é a lista **atualizada** após a operação, então não é preciso
  chamar `/user/blocklist` em seguida.

**GET /user/blocklist** retorna `{ "Blocklist": [...], "DHash": "..." }`.

### 1.2 Configurações de privacidade

| Método | Rota            | Descrição                            |
| ------ | --------------- | ------------------------------------ |
| GET    | `/user/privacy` | Lê as configurações de privacidade   |
| POST   | `/user/privacy` | Altera **uma** configuração por vez  |

**POST /user/privacy**:

```json
{ "Name": "last", "Value": "contacts" }
```

Valores aceitos por configuração (validados antes de ir ao servidor; valor inválido
retorna `400`):

| `Name`      | Valores aceitos                                       |
| ----------- | ----------------------------------------------------- |
| `groupadd`  | `all`, `contacts`, `contact_blacklist`, `none`        |
| `last`      | `all`, `contacts`, `contact_blacklist`, `none`        |
| `status`    | `all`, `contacts`, `contact_blacklist`, `none`        |
| `profile`   | `all`, `contacts`, `contact_blacklist`, `none`        |
| `readreceipts` | `all`, `none`                                      |
| `online`    | `all`, `match_last_seen`                              |
| `calladd`   | `all`, `known`                                        |

A resposta (tanto GET quanto POST) é o objeto completo de configurações já atualizado.

> Nota: o protocolo do WhatsApp também define `messages`, `defense` e `stickers`, mas o
> whatsmeow não atualiza o cache para esses nomes, então eles foram deliberadamente
> deixados de fora para não retornar estado inconsistente.

### 1.3 Presença / último visto

| Método | Rota                       | Descrição                              |
| ------ | -------------------------- | -------------------------------------- |
| POST   | `/user/presence/subscribe` | Assina eventos de presença de um contato |

```json
{ "Phone": "5511999999999" }
```

Depois de assinar, o webhook passa a receber eventos `Presence` daquele contato.
Requer sessão conectada. Ver [seção 3.1](#31-presence-com-last_seen) para o payload.

### 1.4 Pedidos de entrada em grupo (fila de aprovação)

| Método | Rota                                  | Descrição                                  |
| ------ | ------------------------------------- | ------------------------------------------ |
| GET    | `/group/requestparticipants`          | Lista solicitações pendentes                |
| POST   | `/group/updaterequestparticipants`    | Aprova ou rejeita solicitações              |
| POST   | `/group/joinapprovalmode`             | Liga/desliga a exigência de aprovação       |

**GET /group/requestparticipants?groupJID=123456789@g.us** — retorna a lista de
participantes que pediram para entrar.

**POST /group/updaterequestparticipants**:

```json
{
  "GroupJID": "123456789@g.us",
  "Phone": ["5511999999999", "5511888888888"],
  "Action": "approve"
}
```

`Action` deve ser `approve` ou `reject` (qualquer outro valor retorna `400`).

**POST /group/joinapprovalmode** — atenção: este usa chaves **minúsculas**, diferente
dos outros dois:

```json
{ "groupjid": "123456789@g.us", "mode": true }
```

---

## 2. Mudanças em rotas existentes

### 2.1 Envio de mídia por URL HTTP

Os endpoints de envio de mídia agora aceitam, além de data URL base64, uma **URL HTTP
comum** no mesmo campo:

| Endpoint             | Campo      | Limite de download |
| -------------------- | ---------- | ------------------ |
| `/chat/send/image`   | `Image`    | 16 MB              |
| `/chat/send/video`   | `Video`    | 100 MB             |
| `/chat/send/audio`   | `Audio`    | 16 MB              |
| `/chat/send/document`| `Document` | 100 MB             |
| `/chat/send/sticker` | `Sticker`  | 16 MB              |

```json
{ "Phone": "5511999999999", "Image": "https://exemplo.com/foto.jpg" }
```

O MIME type é detectado do `Content-Type` da resposta (ou do conteúdo, se ausente) e
pode ser sobrescrito pelo campo `MimeType`. Isso elimina a necessidade de baixar e
converter para base64 do lado do cliente.

### 2.2 `POST /session/disconnect?clear=true`

Antes, desconectar **apagava** as assinaturas de eventos do usuário, e era preciso
reconfigurá-las a cada reconexão. Agora as assinaturas são **preservadas por padrão**.
Para restaurar o comportamento antigo (limpar tudo), passe `?clear=true`.

> Se a nossa aplicação hoje reconfigura os eventos após cada disconnect, essa chamada
> passou a ser desnecessária.

### 2.3 `POST /session/proxy` — bypass de proxy no webhook

O payload ganhou o campo opcional `webhook_use_proxy`:

```json
{
  "enable": true,
  "proxy_url": "socks5://user:pass@host:1080",
  "webhook_use_proxy": false
}
```

Com `false`, o proxy é usado apenas para a conexão com o WhatsApp; as entregas de
webhook saem pela rede direta. Útil quando o webhook é um serviço interno que o proxy
não alcança. Omitir o campo mantém o padrão global (ver [seção 5](#5-configuração-nova)).

As respostas de sessão/usuário passaram a incluir o objeto:

```json
{ "enabled": true, "proxy_url": "socks5://...", "webhook_use_proxy": false }
```

### 2.4 Link preview de melhor qualidade

`/chat/send/message` com `LinkPreview: true` agora envia `User-Agent` na busca do
Open Graph (sites que bloqueavam requests sem UA voltam a funcionar) e faz upload de
uma thumbnail em alta resolução, o que renderiza o **card grande** de preview em vez do
thumbnail pequeno inline. Não há mudança de payload — é só qualidade do resultado.

---

## 3. Payloads de webhook enriquecidos

### 3.1 Presence com `last_seen`

Evento `Presence` (após assinar via `/user/presence/subscribe`):

```json
{
  "type": "Presence",
  "from": "5511999999999@s.whatsapp.net",
  "state": "offline",
  "last_seen": 1753900000
}
```

`last_seen` é um timestamp Unix e só aparece quando `state` é `offline` **e** o contato
expõe o último visto (respeitando a privacidade dele).

### 3.2 Votos de enquete em texto claro

Eventos de voto agora trazem um objeto `pollVote` com as opções **já descriptografadas
e resolvidas** para o texto original:

```json
{
  "pollVote": {
    "pollCreationMsgID": "3EB0ABCD1234",
    "selectedOptions": ["Opção A", "Opção C"],
    "selectedHashesB64": ["h1...", "h2..."]
  }
}
```

Antes só chegavam os hashes — era necessário guardar as opções e refazer o SHA-256 do
lado do consumidor para saber em que votaram. Agora `selectedOptions` já vem pronto
(`selectedHashesB64` fica disponível para conciliação, se útil).

---

## 4. Correções relevantes para o consumidor

Não exigem mudança de código na nossa aplicação, mas alteram o comportamento observado:

- **Crash do processo por webhook** — um panic durante a entrega de webhook (por
  exemplo, para um usuário já deletado) derrubava o wuzapi inteiro, desconectando
  **todas** as sessões. Agora o panic é contido e logado; perde-se no máximo uma entrega.
- **Data races** — acesso concorrente ao mapa de sessões, ao cache de usuários e à
  config de S3 foi serializado. Elimina uma classe de travamentos e crashes aleatórios
  sob carga.
- **Spam de erro no HistorySync** — o insert de histórico virou idempotente
  (`ON CONFLICT DO NOTHING`), acabando com a enxurrada de erros de chave duplicada.
- **Vazamento de arquivos temporários** — cada envio de imagem deixava um arquivo em
  `/tmp`; agora são removidos.
- **Token admin** — a comparação passou a ser por hash em tempo constante (não é mais
  possível inferir o tamanho/conteúdo do token medindo tempo de resposta).
- **Deleção no S3** — objetos agora são realmente removidos na rota `/full`.
- **Edição de mensagem** — compatível com o modelo novo do WhatsApp.
- **PostgreSQL** — migração de índice que falhava foi corrigida, mais um índice novo em
  `whatsmeow_message_secrets` (melhora a performance de descriptografia).
- **SQLite** — modo WAL e busy timeout maior (menos erros de "database is locked").

---

## 5. Configuração nova

| Variável                    | Padrão | Descrição                                                         |
| --------------------------- | ------ | ----------------------------------------------------------------- |
| `WUZAPI_WEBHOOK_USE_PROXY`  | `true` | Padrão global de uso do proxy nas entregas de webhook. Pode ser sobrescrito por usuário via `/session/proxy`. |

---

## 6. Sugestão de priorização

Ordenado por relação valor/esforço para a nossa aplicação:

1. **`?clear=true` no disconnect** — se hoje reconfiguramos eventos após reconectar,
   dá para simplesmente remover esse passo.
2. **Envio de mídia por URL** — remove o download + base64 do nosso lado; menos memória
   e menos latência em cada envio.
3. **`pollVote.selectedOptions`** — se consumimos enquetes, permite jogar fora a lógica
   de hash que mantemos hoje.
4. **Bloqueio de contatos** — capacidade nova, sem equivalente anterior.
5. **Presença com `last_seen`** — capacidade nova, útil para indicadores de atividade.
6. **Pedidos de entrada em grupo** — só relevante se administramos grupos com aprovação.
7. **Configurações de privacidade** — nicho, mas trivial de expor.

---

## Referências

- Rotas: [routes.go](routes.go)
- Handlers de grupo/aprovação: [handlers_grouprequests.go](handlers_grouprequests.go)
- Processamento de mídia recebida: [media.go](media.go)
- Spec OpenAPI atualizada: [static/api/spec.yml](static/api/spec.yml)
- Coleção Postman atualizada: [wuzapi_postman.json](wuzapi_postman.json)
