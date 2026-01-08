# WuzAPI <-> Chatwoot Native Integration (specs.md)

## 0) Guidelines

### Contexto
Este repositório é um fork do WuzAPI. Vamos adicionar uma integração nativa com Chatwoot, sem alterar o Chatwoot.

### Objetivo do guideline
Garantir que o Codex execute mudanças pequenas, testáveis, com rastreabilidade e progresso sempre visível neste arquivo.

### Regras de trabalho (obrigatórias)
1) Sempre começar lendo este `specs.md` e escolher a próxima tarefa NÃO marcada.
2) Executar no máximo 1 tarefa por vez (ou um pequeno conjunto coerente dentro da mesma milestone).
3) Antes de codar: escrever um plano curto (3 a 7 bullets) do que vai mudar e onde.
4) Depois de codar: rodar testes e checagens locais definidas na tarefa.
5) Só marcar a checkbox como concluída após:
   - testes passarem, e
   - build local (quando aplicável) estar OK, e
   - um pequeno resumo do que mudou estar refletido no `specs.md` (se a tarefa pedir).
6) Cada tarefa concluída deve virar pelo menos 1 commit, com mensagem clara e escopo pequeno.
7) Se surgir bloqueio por falta de informação:
   - não chutar,
   - registrar em “Pendências e decisões” no fim do arquivo,
   - propor a alternativa mais segura e seguir com o que não depende da pendência.

### Critérios de aceite do guideline
- O histórico de commits e as checkboxes permitem saber exatamente:
  - o que já foi implementado,
  - o que falta,
  - como reproduzir testes.
- Nenhuma tarefa é marcada sem evidência de teste/checagem descrita.

### Progress Tracker (Checkpoint)

- **Status (last completed milestone):** M0
- **Last review:** 2026-01-05 — avaliação de adequação do specs para integração Chatwoot (pendências registradas fora do tracker)
- **Last update:** 2026-01-07 — M1.4 CRUD chatwoot_config/chatwoot_map com testes
- **Next up:** **Milestone M1: Persistência (migrações) e modelo de config/mapeamento**

---

## 1) Visão geral do produto

### Problema
O WuzAPI não possui integração nativa com Chatwoot. Queremos usar o Chatwoot como central omnichannel (incluindo WhatsApp), mantendo o WuzAPI como camada WhatsApp.

### Solução
Adicionar no WuzAPI um conector nativo “Chatwoot”, com 2 fluxos:
1) Inbound: WhatsApp -> WuzAPI -> Chatwoot (cria contato/conversa/mensagem)
2) Outbound: Chatwoot -> WuzAPI (webhook/callback) -> WhatsApp (envio via whatsmeow)

### Escopo inicial (MVP)
- Mensagens de texto 1:1 (não-grupo)
- Criação e reuso de contato e conversa no Chatwoot
- Entrega de mensagens outbound do Chatwoot para WhatsApp
- Prevenção de loop em ambos os lados
- Configuração por usuário/token do WuzAPI (multi-tenant), se possível
- Configuração via dashboard do WuzAPI (modal) com teste de conexão e provisionamento do API Inbox
- Comandos operacionais via Chatwoot (onboarding + #qrcode, #help, #status, #disconnect, #attid, #updateavatar)
- Ajustes via toggles (assinar mensagens, reabrir conversas, set pending, ignorar grupos, typing indicator, números ignorados)

### Fora de escopo (primeira versão)
- Mídia (imagem, áudio, documento, vídeo, sticker)
- Templates WhatsApp
- Mensagens de grupo (@g.us)
- Sincronização de status de leitura e presence (typing apenas via toggle)
- Suporte a múltiplas contas Chatwoot por um mesmo usuário do WuzAPI (pode entrar depois)
- Anexos para Chatwoot (exceto o QR code, que deve seguir o formato do WuzAPI)

---

## 2) Arquitetura e fluxo

### 2.1 Inbound (WhatsApp -> Chatwoot)
Ponto de hook: `wmiau.go`, handler de eventos, `case *events.Message`.

Regras:
- Ignorar mensagens `IsFromMe == true` (evita eco do que o próprio WuzAPI enviou).
- Ignorar eventos de grupo (JID termina com `@g.us`) se `Ignore Groups` estiver habilitado.
- Ignorar números listados em `ignored_numbers`.
- Para mensagem válida:
  1) Determinar “contact key” (ex: phone E.164 ou número puro; decisão abaixo).
  2) Garantir que existe um Contact no Chatwoot:
     - se não houver mapeamento local, criar Contact via Client API e armazenar `contact_identifier` (source_id).
  3) Garantir que existe uma Conversation aberta:
     - se não houver mapeamento, criar nova Conversation e armazenar `conversation_id`.
     - se a conversa estiver resolvida:
       - se `Reopen Conversations` estiver habilitado, reabrir a conversa existente;
       - caso contrario, criar nova Conversation e armazenar `conversation_id`.
  4) Criar Message (incoming) na Conversation com o texto.
  5) Se `Set Conversations as Pending` estiver habilitado, atualizar status para `pending`.

### 2.2 Outbound (Chatwoot -> WhatsApp) via callback do API inbox
Ao criar o API Inbox no Chatwoot, será configurada uma Callback URL. Sempre que uma nova mensagem for criada nesse API Inbox, o Chatwoot fará um POST para essa Callback URL com o evento `message_created` e o campo `message_type` (incoming ou outgoing).

Regras:
- Validar segredo (token) no request (query param ou header). Sem isso vira relay aberto.
- Resolver `wuzapi_user_id`/config pelo `callback_secret` (token) OU pelo `inbox_id` presente no payload (`body.inbox.id` ou `body.conversation.inbox_id`).
  - `callback_secret` deve ser único por usuário para evitar colisões.
  - `inbox_id` deve ser único por config se usado como fallback.
- Se `Enable Chatwoot Integration` estiver desabilitado, ignorar o callback.
- Processar somente evento `message_created`.
- Processar somente `message_type == "outgoing"`.
- Ignorar `private == true` (notas internas), se presente.
- Ignorar mensagens de números listados em `ignored_numbers`.
- Resolver destinatário (WhatsApp JID) a partir do payload consultando o mapeamento salvo no banco:
  - usar `body.conversation.contact_inbox.source_id` como identificador do contato.
- Se a mensagem for um comando de sistema (ex.: `#qrcode`) na conversa do contato "Flownix", não enviar ao WhatsApp; processar o comando e responder no Chatwoot.
- Se `Sign Messages` estiver habilitado, acrescentar assinatura ao texto antes do envio ao WhatsApp.
- Enviar via pipeline existente do WuzAPI (o mesmo fluxo que o endpoint `/chat/send/text` usa internamente).

### 2.3 Prevenção de loop
- Lado WuzAPI -> Chatwoot: filtrar `IsFromMe == true` no hook inbound.
- Lado Chatwoot -> WuzAPI: processar apenas `message_type == outgoing`.

### 2.4 Comandos e onboarding via Chatwoot
- Ao salvar a configuração, criar/atualizar um contato "Flownix" (email `contato@flownix.com.br`) e garantir uma conversa de onboarding.
- Enviar mensagem de onboarding instruindo o usuario a digitar `#qrcode` para conectar o WhatsApp.
- Comandos suportados (processados a partir de mensagens outgoing do agente):
  - `#qrcode`: gerar QR code da instancia no mesmo formato do WuzAPI (`data:image/png;base64,...`) e responder no Chatwoot.
  - `#help`: listar comandos disponiveis.
  - `#status`: retornar status da sessao WhatsApp.
  - `#disconnect`: desconectar sessao WhatsApp.
  - `#attid`: resetar e atualizar identificadores de contato (detalhar em Pendencias).
  - `#updateavatar`: atualizar a foto de perfil do contato.

---

## 3) Persistência e dados

### 3.1 Tabelas novas (migração SQL)
Criar uma tabela para configuração e uma para mapeamento.

1) `chatwoot_config`
- id (PK)
- wuzapi_user_id (FK lógico para o usuário/token do WuzAPI)
- chatwoot_base_url (text)
- account_id (int)
- api_token (text, armazenar criptografado)
- inbox_identifier (text)
- inbox_name (text)
- inbox_id (int)
- callback_secret (text)
- hmac_secret (text, opcional, se formos usar identifier_hash)
- enabled (boolean)
- sign_messages (boolean)
- signature_text (text, opcional)
- reopen_conversations (boolean)
- set_conversations_pending (boolean)
- ignore_groups (boolean)
- enable_typing_indicator (boolean)
- ignored_numbers (text/json)
- system_contact_identifier (text, opcional)
- system_conversation_id (int, opcional)
- created_at, updated_at

2) `chatwoot_map`
- id (PK)
- wuzapi_user_id
- wa_jid (text)                  // ex: 5511999999999@s.whatsapp.net
- wa_phone (text)                // ex: 5511999999999
- chatwoot_contact_identifier (text)  // source_id retornado no create contact
- chatwoot_conversation_id (int)      // última conversa ativa
- conversation_status (text, opcional)
- last_sync_at
- created_at, updated_at

Observação: guardar `wa_phone` facilita reconstruir JID e facilita debug.
Índices recomendados:
- `chatwoot_config`: UNIQUE (`wuzapi_user_id`), UNIQUE (`callback_secret`), UNIQUE (`inbox_identifier`), UNIQUE (`inbox_id`)
- `chatwoot_map`: UNIQUE (`wuzapi_user_id`, `wa_jid`), UNIQUE (`wuzapi_user_id`, `chatwoot_contact_identifier`)
As migrações devem cobrir PostgreSQL e SQLite.

---

## 4) Configuração e segurança

### 4.1 Como configurar
No MVP, fornecer endpoints para o usuário do WuzAPI configurar a integração:

- GET  `/integrations/chatwoot/config`
- POST `/integrations/chatwoot/config`

Payload esperado:
- chatwoot_base_url
- account_id
- api_token
- inbox_name
- inbox_identifier
- inbox_id
- callback_secret
- (opcional) hmac_secret
- enabled
- sign_messages
- signature_text (opcional)
- reopen_conversations
- set_conversations_pending
- ignore_groups
- enable_typing_indicator
- ignored_numbers

### 4.2 Segurança do callback do API inbox
O callback do API inbox deve ser tratado como um endpoint sensível.

Requisitos de segurança (MVP):
- Exigir um `callback_secret` (token longo) e rejeitar qualquer request sem o segredo correto.
- Rejeitar se não houver config encontrada para o segredo ou `inbox_id`.
- O segredo deve ser validado via:
  - query param (ex.: `?token=...`) OU
  - header dedicado (ex.: `X-Chatwoot-Token: ...`).
- Logar tentativas inválidas com rate limit (quando aplicável).
- Nunca logar `api_token` em texto.

### 4.3 Provisionamento do inbox e teste de conexao
- Test Connection: validar `chatwoot_base_url`, `account_id` e `api_token` via Account API.
- Create Inbox: criar API Inbox com `inbox_name`, `callback_webhook_url` e capturar `inbox_id` e `inbox_identifier`.
- Update Inbox: atualizar o inbox existente quando configuracoes mudarem.

---

## 5) API mapping (alto nível)

### 5.1 Chatwoot Client APIs usadas (inbound) no API inbox
Autenticação das Client APIs usa `inbox_identifier` e `contact_identifier`.
- Create Contact
- Create Conversation (necessário para obter `conversation_id`)
- Create Message (exige `conversation_id`)

### 5.2 Chatwoot Account APIs usadas (provisionamento e status)
- Test Connection: `GET /api/v1/accounts/{account_id}`
- Create/Update Inbox: `POST/PUT /api/v1/accounts/{account_id}/inboxes` (Channel::Api)
- Update Conversation Status (reopen/pending): endpoint de conversas

### 5.3 WuzAPI endpoints usados (outbound)
- Internamente, reusar o mesmo fluxo de envio do `/chat/send/text`.
- Não chamar HTTP em loop dentro do próprio processo; extrair/reusar função interna de envio.

---

## 6) Testes

### 6.1 Testes unitários (obrigatórios)
- Chatwoot client: build URL, headers, parse de responses, retry/backoff (se existir)
- Parser de webhook do Chatwoot: aceita payload válido, rejeita sem segredo, ignora incoming/private
- Parser do callback: aceita `message_type` como string (top-level) e int (mensagens aninhadas)
- Resolver tenant/config: segredo/inbox inexistente => erro controlado; sucesso => retorna config correta
- Mapper: resolve wa_jid por contact_identifier e vice-versa
- Validador de config: base_url, account_id, api_token, inbox_name, inbox_id, toggles e ignored_numbers
- Command parser: processa #qrcode/#help/#status/#disconnect/#attid/#updateavatar
- Typing indicator: parser de eventos e chamadas de presence no WhatsApp

### 6.2 Teste de integração local (obrigatório antes de marcar milestone MVP)
- Subir WuzAPI local (docker compose) com banco
- Subir um mock HTTP server para Chatwoot (httptest) OU usar um Chatwoot local (opcional)
- Simular:
  - evento inbound WhatsApp -> cria contact/conversation/message (mock Chatwoot recebe chamadas)
  - webhook outbound Chatwoot -> WuzAPI -> chama SendMessage (mockar whatsmeow client)

---

## 7) Milestones e tarefas

### Milestone M0: Preparar fork, ambiente e harness de testes
- [x] Criar fork do repositório e adicionar `upstream` remoto (para comparar e puxar mudanças quando necessário)
- [x] Documentar no README (ou em `specs.md` seção “Como rodar local”) o fluxo básico: `go test ./...`, `docker compose up`, `go run`
- [x] Criar `docker-compose.dev.yml` (ou equivalente) para subir banco + WuzAPI local com variáveis mínimas
- [x] Adicionar um pacote de testes base (ex: `internal/testutil`) para facilitar mocks HTTP e fixtures JSON
- [x] Rodar `go test ./...` e corrigir qualquer falha existente (se houver) antes de iniciar features

Critério de aceite M0:
- `go test ./...` passa localmente
- existe um caminho claro e reproduzível para rodar WuzAPI local

---

### Milestone M1: Persistência (migrações) e modelo de config/mapeamento
- [x] Adicionar migração SQL (PostgreSQL + SQLite) para criar `chatwoot_config` e `chatwoot_map`
- [x] Incluir novos campos de config (account_id, api_token, inbox_name, toggles, ignored_numbers)
- [x] Adicionar índices únicos conforme seção 3.1
- [x] Implementar funções DB (sqlx) para:
  - [x] upsert de config por wuzapi_user_id
  - [x] get config por wuzapi_user_id
  - [x] upsert de map por wa_jid
  - [x] get map por wa_jid
  - [x] get map por chatwoot_contact_identifier
  - [x] update conversation_id/status

Critério de aceite M1:
- Migração aplica com banco limpo
- CRUD básico funciona com testes unitários

---

### Milestone M2: Chatwoot client (Client APIs)
- [ ] Criar pacote `internal/chatwoot` (ou similar) com client HTTP
- [ ] Confirmar endpoints e payloads do API Inbox (Chatwoot) antes de fixar assinatura das funções
- [ ] Implementar:
  - [ ] `CreateContact(inbox_identifier, payload) -> contact_identifier`
  - [ ] `CreateConversation(inbox_identifier, contact_identifier) -> conversation_id`
  - [ ] `CreateMessage(inbox_identifier, contact_identifier, conversation_id, content) -> message_id`
- [ ] Implementar Account API (test connection, create/update inbox)
- [ ] Definir política de timeout e retry (no mínimo timeout)
- [ ] Testes unitários com mock server (httptest)

Critério de aceite M2:
- Testes cobrem respostas 200 e erros comuns (401/404/500)
- Client não faz logs de segredo (inbox_identifier/callback_secret)

---

### Milestone M3: Inbound WhatsApp -> Chatwoot
- [ ] No hook `case *events.Message`:
  - [ ] Ignorar `IsFromMe == true`
  - [ ] Respeitar `Ignore Groups`
  - [ ] Extrair phone/JID e conteúdo de texto (primeiro suportar apenas texto simples)
  - [ ] Ignorar `ignored_numbers`
- [ ] Implementar pipeline:
  - [ ] Load config do usuário (wuzapi_user_id)
  - [ ] Resolver ou criar contact no Chatwoot e salvar mapping
  - [ ] Resolver ou criar conversation e salvar mapping
  - [ ] Reabrir conversa resolvida se `Reopen Conversations` estiver habilitado
  - [ ] Setar status `pending` se `Set Conversations as Pending` estiver habilitado
  - [ ] Criar message incoming no Chatwoot
- [ ] Log estruturado (info/warn/error) com correlation id (ex: message id do WhatsApp)

Testes:
- [ ] Unit test do “handler inbound” com:
  - [ ] evento IsFromMe => não chama Chatwoot
  - [ ] evento grupo => não chama Chatwoot
  - [ ] evento texto => chama CreateContact/Conversation/Message na ordem esperada

Critério de aceite M3:
- Um evento de texto gera uma conversa e mensagem no Chatwoot (via mock)
- Não há loop do que é enviado pelo próprio número (IsFromMe filtrado)

---

### Milestone M4: Outbound Chatwoot -> WhatsApp (callback do API inbox)
- [ ] Criar endpoint HTTP: `POST /integrations/chatwoot/callback`
  - [ ] Validar `callback_secret` (query ou header)
  - [ ] Aceitar apenas JSON
- [ ] Resolver config/tenant via `callback_secret` OU `inbox_id` do payload
- [ ] Implementar parser para payload do callback do API inbox:
  - [ ] aceitar apenas `event == "message_created"`
  - [ ] aceitar apenas `message_type == "outgoing"` (string top-level; int nas mensagens aninhadas)
  - [ ] ignorar `private == true` (se presente no payload)
- [ ] Resolver destinatário:
  - [ ] extrair identificador do contato do payload (`conversation.contact_inbox.source_id`)
  - [ ] buscar em `chatwoot_map` o `wa_jid`
- [ ] Processar comandos `#qrcode/#help/#status/#disconnect/#attid/#updateavatar` quando a conversa for do contato "Flownix"
- [ ] Aplicar `Sign Messages` e `ignored_numbers`
- [ ] Enviar mensagem via função interna equivalente ao `/chat/send/text`

Testes:
- [ ] Sem segredo => 401/403
- [ ] Incoming => ignorar
- [ ] Outgoing => chama função de envio WhatsApp
- [ ] Sem mapeamento => 404 controlado OU log + ignore (definir comportamento)

Critério de aceite M4:
- Mensagem enviada pelo agente no Chatwoot vira mensagem no WhatsApp (via mock)
- Nenhuma mensagem incoming do callback dispara envio (evita loop)

---

### Milestone M5: Configuração via dashboard e provisionamento
- [ ] Implementar GET/POST `/integrations/chatwoot/config`
- [ ] Implementar `POST /integrations/chatwoot/test` (test connection)
- [ ] Implementar `POST /integrations/chatwoot/inbox` (create/update inbox)
- [ ] Validar inputs:
  - [ ] base_url válido (http/https)
  - [ ] account_id válido
  - [ ] api_token não vazio
  - [ ] inbox_name não vazio
  - [ ] inbox_identifier não vazio (após create)
  - [ ] inbox_id válido (> 0, após create)
  - [ ] callback_secret >= 16 chars
- [ ] UI: adicionar "Chatwoot Integration" no dashboard usando modal existente
  - [ ] campos e toggles conforme seção 4.1
  - [ ] botão "Test Connection" com toast de sucesso/falha
  - [ ] botão "Create Inbox" visível após teste OK, vira "Update Inbox" após criação
  - [ ] botão "Save" persiste config no WuzAPI
- [ ] Documentar no `specs.md` (ou README) o passo a passo
- [ ] Atualizar `API.md` e `static/api/spec.yml` com os novos endpoints

Critério de aceite M5:
- Config persistida e recuperável
- Test connection e create/update inbox funcionam
- UI reproduzivel por alguém que não conhece o código

---

### Milestone M6: Onboarding e comandos operacionais
- [ ] Criar/atualizar contato "Flownix" e conversa de onboarding no Chatwoot
- [ ] Enviar mensagem de onboarding ao salvar configuração
- [ ] Implementar parser/handler de comandos (#qrcode/#help/#status/#disconnect/#attid/#updateavatar)
- [ ] Enviar resposta no Chatwoot para cada comando
- [ ] Implementar "Enable Typing Indicator" via eventos `conversation_typing_on/off` (Account Webhooks)

Critério de aceite M6:
- Comandos funcionam e não são enviados ao WhatsApp
- #qrcode entrega QR code da instancia no mesmo formato do WuzAPI (`data:image/png;base64,...`)
- Typing indicator respeita o toggle

---

### Milestone M7: Build, imagem e deploy na VPS
- [ ] Garantir que `docker build` funciona local
- [ ] Versionar/taggear imagem do fork (ex: `flownix/wuzapi-chatwoot:<tag>`)
- [ ] Documentar variáveis necessárias no stack da VPS
- [ ] Smoke test em staging (se existir) ou checklist de validação no deploy final

Critério de aceite M7:
- Imagem sobe sem erro
- Fluxos inbound/outbound funcionam em produção

---

## 8) Pendências e decisões (não bloquear desenvolvimento)
1) Qual será a normalização do “contact key” no Chatwoot?
   - opção A: `identifier = phone puro (E.164 sem +)`
   - opção B: `identifier = wa_jid completo`
2) Como lidar com conversa resolvida no Chatwoot:
   - criar nova conversation sempre que a atual estiver resolved
3) Comportamento quando webhook outbound chega sem mapeamento:
   - retornar 404 e logar (recomendado) ou tentar reconstruir pelo phone (se vier no payload)
4) Qual será o texto da assinatura quando `Sign Messages` estiver habilitado?
5) Formato de `ignored_numbers` (E.164, sem +, CSV ou JSON?)
6) Semantica exata do comando `#attid` (quais IDs limpar/atualizar?)
7) `Ignore Groups` desabilitado deve permitir grupos no MVP ou manter fora de escopo?
8) Como registrar e validar account webhooks para typing indicator?

(Manter esta seção sempre atualizada quando surgir uma nova pendência.)
