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

- **Status (last completed milestone):** _none_
- **Last review:** 2026-01-05 — avaliação de adequação do specs para integração Chatwoot (pendências registradas fora do tracker)
- **Last update:** 2026-01-05 — ajustes de escopo/fluxo (tenant no callback, naming e docs)
- **Next up:** **Milestone M0: Preparar fork, ambiente e harness de testes**

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

### Fora de escopo (primeira versão)
- Mídia (imagem, áudio, documento, vídeo, sticker)
- Templates WhatsApp
- Mensagens de grupo (@g.us)
- Sincronização de status de leitura, typing, presence
- Suporte a múltiplas contas Chatwoot por um mesmo usuário do WuzAPI (pode entrar depois)

---

## 2) Arquitetura e fluxo

### 2.1 Inbound (WhatsApp -> Chatwoot)
Ponto de hook: `wmiau.go`, handler de eventos, `case *events.Message`.

Regras:
- Ignorar mensagens `IsFromMe == true` (evita eco do que o próprio WuzAPI enviou).
- Ignorar eventos de grupo (JID termina com `@g.us`) no MVP.
- Para mensagem válida:
  1) Determinar “contact key” (ex: phone E.164 ou número puro; decisão abaixo).
  2) Garantir que existe um Contact no Chatwoot:
     - se não houver mapeamento local, criar Contact via Client API e armazenar `contact_identifier` (source_id).
  3) Garantir que existe uma Conversation aberta:
     - se não houver mapeamento ou se a conversa estiver resolvida, criar nova Conversation e armazenar `conversation_id`.
  4) Criar Message (incoming) na Conversation com o texto.

### 2.2 Outbound (Chatwoot -> WhatsApp) via callback do API inbox
Ao criar o API Inbox no Chatwoot, será configurada uma Callback URL. Sempre que uma nova mensagem for criada nesse API Inbox, o Chatwoot fará um POST para essa Callback URL com o evento `message_created` e o campo `message_type` (incoming ou outgoing).

Regras:
- Validar segredo (token) no request (query param ou header). Sem isso vira relay aberto.
- Resolver `wuzapi_user_id`/config pelo `callback_secret` (token) OU pelo `inbox_identifier` presente no payload.
  - `callback_secret` deve ser único por usuário para evitar colisões.
- Processar somente evento `message_created`.
- Processar somente `message_type == "outgoing"`.
- Ignorar `private == true` (notas internas), se presente.
- Resolver destinatário (WhatsApp JID) a partir do payload (identificador do contato) consultando o mapeamento salvo no banco.
- Enviar via pipeline existente do WuzAPI (o mesmo fluxo que o endpoint `/chat/send/text` usa internamente).

### 2.3 Prevenção de loop
- Lado WuzAPI -> Chatwoot: filtrar `IsFromMe == true` no hook inbound.
- Lado Chatwoot -> WuzAPI: processar apenas `message_type == outgoing`.

---

## 3) Persistência e dados

### 3.1 Tabelas novas (migração SQL)
Criar uma tabela para configuração e uma para mapeamento.

1) `chatwoot_config`
- id (PK)
- wuzapi_user_id (FK lógico para o usuário/token do WuzAPI)
- chatwoot_base_url (text)
- inbox_identifier (text)
- callback_secret (text)
- hmac_secret (text, opcional, se formos usar identifier_hash)
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
- `chatwoot_config`: UNIQUE (`wuzapi_user_id`), UNIQUE (`callback_secret`), UNIQUE (`inbox_identifier`)
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
- inbox_identifier
- callback_secret
- (opcional) hmac_secret

### 4.2 Segurança do callback do API inbox
O callback do API inbox deve ser tratado como um endpoint sensível.

Requisitos de segurança (MVP):
- Exigir um `callback_secret` (token longo) e rejeitar qualquer request sem o segredo correto.
- Rejeitar se não houver config encontrada para o segredo ou inbox.
- O segredo deve ser validado via:
  - query param (ex.: `?token=...`) OU
  - header dedicado (ex.: `X-Chatwoot-Token: ...`).
- Logar tentativas inválidas com rate limit (quando aplicável).

---

## 5) API mapping (alto nível)

### 5.1 Chatwoot Client APIs usadas (inbound) no API inbox
Autenticação das Client APIs usa `inbox_identifier` e `contact_identifier`.
- Create Contact
- Create Conversation (se aplicável)
- Create Message (pode criar Conversation automaticamente; validar resposta e capturar `conversation_id`)

### 5.2 WuzAPI endpoints usados (outbound)
- Internamente, reusar o mesmo fluxo de envio do `/chat/send/text`.
- Não chamar HTTP em loop dentro do próprio processo; extrair/reusar função interna de envio.

---

## 6) Testes

### 6.1 Testes unitários (obrigatórios)
- Chatwoot client: build URL, headers, parse de responses, retry/backoff (se existir)
- Parser de webhook do Chatwoot: aceita payload válido, rejeita sem segredo, ignora incoming/private
- Resolver tenant/config: segredo/inbox inexistente => erro controlado; sucesso => retorna config correta
- Mapper: resolve wa_jid por contact_identifier e vice-versa

### 6.2 Teste de integração local (obrigatório antes de marcar milestone MVP)
- Subir WuzAPI local (docker compose) com banco
- Subir um mock HTTP server para Chatwoot (httptest) OU usar um Chatwoot local (opcional)
- Simular:
  - evento inbound WhatsApp -> cria contact/conversation/message (mock Chatwoot recebe chamadas)
  - webhook outbound Chatwoot -> WuzAPI -> chama SendMessage (mockar whatsmeow client)

---

## 7) Milestones e tarefas

### Milestone M0: Preparar fork, ambiente e harness de testes
- [ ] Criar fork do repositório e adicionar `upstream` remoto (para comparar e puxar mudanças quando necessário)
- [ ] Documentar no README (ou em `specs.md` seção “Como rodar local”) o fluxo básico: `go test ./...`, `docker compose up`, `go run`
- [ ] Criar `docker-compose.dev.yml` (ou equivalente) para subir banco + WuzAPI local com variáveis mínimas
- [ ] Adicionar um pacote de testes base (ex: `internal/testutil`) para facilitar mocks HTTP e fixtures JSON
- [ ] Rodar `go test ./...` e corrigir qualquer falha existente (se houver) antes de iniciar features

Critério de aceite M0:
- `go test ./...` passa localmente
- existe um caminho claro e reproduzível para rodar WuzAPI local

---

### Milestone M1: Persistência (migrações) e modelo de config/mapeamento
- [ ] Adicionar migração SQL (PostgreSQL + SQLite) para criar `chatwoot_config` e `chatwoot_map`
- [ ] Adicionar índices únicos conforme seção 3.1
- [ ] Implementar funções DB (sqlx) para:
  - [ ] upsert de config por wuzapi_user_id
  - [ ] get config por wuzapi_user_id
  - [ ] upsert de map por wa_jid
  - [ ] get map por wa_jid
  - [ ] get map por chatwoot_contact_identifier
  - [ ] update conversation_id/status

Critério de aceite M1:
- Migração aplica com banco limpo
- CRUD básico funciona com testes unitários

---

### Milestone M2: Chatwoot client (Client APIs)
- [ ] Criar pacote `internal/chatwoot` (ou similar) com client HTTP
- [ ] Confirmar endpoints e payloads do API Inbox (Chatwoot) antes de fixar assinatura das funções
- [ ] Implementar:
  - [ ] `CreateContact(inbox_identifier, payload) -> contact_identifier`
  - [ ] `CreateConversation(inbox_identifier, contact_identifier) -> conversation_id` (se necessário)
  - [ ] `CreateMessage(inbox_identifier, contact_identifier, conversation_id?, content) -> message_id` (capturar `conversation_id` se vier na resposta)
- [ ] Definir política de timeout e retry (no mínimo timeout)
- [ ] Testes unitários com mock server (httptest)

Critério de aceite M2:
- Testes cobrem respostas 200 e erros comuns (401/404/500)
- Client não faz logs de segredo (inbox_identifier/callback_secret)

---

### Milestone M3: Inbound WhatsApp -> Chatwoot
- [ ] No hook `case *events.Message`:
  - [ ] Ignorar `IsFromMe == true`
  - [ ] Ignorar grupos (`@g.us`) no MVP
  - [ ] Extrair phone/JID e conteúdo de texto (primeiro suportar apenas texto simples)
- [ ] Implementar pipeline:
  - [ ] Load config do usuário (wuzapi_user_id)
  - [ ] Resolver ou criar contact no Chatwoot e salvar mapping
  - [ ] Resolver ou criar conversation e salvar mapping
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
- [ ] Resolver config/tenant via `callback_secret` OU `inbox_identifier` do payload
- [ ] Implementar parser para payload do callback do API inbox:
  - [ ] aceitar apenas `event == "message_created"`
  - [ ] aceitar apenas `message_type == "outgoing"`
  - [ ] ignorar `private == true` (se presente no payload)
- [ ] Resolver destinatário:
  - [ ] extrair identificador do contato do payload (ex.: `source_id` ou equivalente no payload do callback)
  - [ ] buscar em `chatwoot_map` o `wa_jid`
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

### Milestone M5: Endpoints de configuração e UX mínima
- [ ] Implementar GET/POST `/integrations/chatwoot/config`
- [ ] Validar inputs:
  - [ ] base_url válido (http/https)
  - [ ] inbox_identifier não vazio
  - [ ] callback_secret >= 16 chars
- [ ] Documentar no `specs.md` (ou README) o passo a passo:
  - [ ] criar API Inbox no Chatwoot
  - [ ] obter inbox_identifier
  - [ ] criar API Inbox no Chatwoot e definir a Callback URL apontando para:
        `https://SEU_WUZAPI/integrations/chatwoot/callback?token=SEU_CALLBACK_SECRET`
  - [ ] confirmar que o Chatwoot envia `event=message_created` para a Callback URL quando houver mensagens no API Inbox
  - [ ] configurar no WuzAPI via endpoint
- [ ] Atualizar `API.md` e `static/api/spec.yml` com os novos endpoints

Critério de aceite M5:
- Config persistida e recuperável
- Passo a passo reproduzível por alguém que não conhece o código

---

### Milestone M6: Build, imagem e deploy na VPS
- [ ] Garantir que `docker build` funciona local
- [ ] Versionar/taggear imagem do fork (ex: `flownix/wuzapi-chatwoot:<tag>`)
- [ ] Documentar variáveis necessárias no stack da VPS
- [ ] Smoke test em staging (se existir) ou checklist de validação no deploy final

Critério de aceite M6:
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
4) Quais campos exatos o callback do Chatwoot envia para identificar inbox e contato?
5) O API Inbox exige CreateConversation separado ou CreateMessage já cria conversation e retorna `conversation_id`?

(Manter esta seção sempre atualizada quando surgir uma nova pendência.)
