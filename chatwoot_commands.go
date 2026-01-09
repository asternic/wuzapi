package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

const chatwootHelpMessage = "Comandos disponiveis:\n#qrcode\n#help\n#status\n#disconnect\n#attid (pendente)\n#updateavatar (pendente)"

var (
	errChatwootCommandPending = errors.New("chatwoot command pending")
	errChatwootNoSession      = errors.New("no session")
	errChatwootNotConnected   = errors.New("not connected")
	errChatwootAlreadyLinked  = errors.New("already logged in")
	errChatwootQRCodeMissing  = errors.New("qrcode missing")
)

func (s *server) handleChatwootCommand(ctx context.Context, cfg *ChatwootConfig, payload *chatwootCallbackPayload, cmd string) error {
	if cfg == nil {
		return errors.New("chatwoot config is nil")
	}
	if payload == nil {
		return errors.New("chatwoot payload is nil")
	}

	contactIdentifier := payload.contactIdentifier()
	if contactIdentifier == "" {
		contactIdentifier = cfg.SystemContactIdentifier
	}

	conversationID := payload.Conversation.ID
	if conversationID == 0 && cfg.SystemConversationID.Valid {
		conversationID = int(cfg.SystemConversationID.Int64)
	}
	if contactIdentifier == "" || conversationID == 0 {
		return errors.New("missing system contact context")
	}

	response, cmdErr := s.buildChatwootCommandResponse(ctx, cfg, cmd)
	if response == "" {
		response = "Comando nao suportado."
	}

	if err := s.sendChatwootCommandResponse(ctx, cfg, contactIdentifier, conversationID, response); err != nil {
		return err
	}

	if cmdErr != nil {
		log.Warn().Err(cmdErr).Str("command", cmd).Msg("Chatwoot command returned warning")
	}

	if cfg.SystemContactIdentifier == "" || !cfg.SystemConversationID.Valid ||
		cfg.SystemContactIdentifier != contactIdentifier || cfg.SystemConversationID.Int64 != int64(conversationID) {
		cfg.SystemContactIdentifier = contactIdentifier
		cfg.SystemConversationID = sql.NullInt64{Int64: int64(conversationID), Valid: true}
		if err := s.UpsertChatwootConfig(cfg); err != nil {
			log.Warn().Err(err).Msg("Chatwoot command: failed to update system context")
		}
	}

	return nil
}

func (s *server) sendChatwootCommandResponse(ctx context.Context, cfg *ChatwootConfig, contactIdentifier string, conversationID int, content string) error {
	if cfg == nil {
		return errors.New("chatwoot config is nil")
	}
	if strings.TrimSpace(cfg.InboxIdentifier) == "" {
		return errors.New("chatwoot inbox identifier is required")
	}
	client, err := newChatwootClient(cfg.ChatwootBaseURL, cfg.AccountID, cfg.APIToken)
	if err != nil {
		return fmt.Errorf("create chatwoot client: %w", err)
	}
	if _, err := client.CreateMessage(ctx, cfg.InboxIdentifier, contactIdentifier, conversationID, content); err != nil {
		return fmt.Errorf("send chatwoot command response: %w", err)
	}
	return nil
}

func (s *server) buildChatwootCommandResponse(ctx context.Context, cfg *ChatwootConfig, cmd string) (string, error) {
	switch cmd {
	case "#help":
		return chatwootHelpMessage, nil
	case "#status":
		return s.chatwootStatusMessage(cfg.WuzapiUserID), nil
	case "#qrcode":
		return s.chatwootQRCodeMessage(cfg.WuzapiUserID)
	case "#disconnect":
		return s.chatwootDisconnectMessage(cfg.WuzapiUserID)
	case "#attid", "#updateavatar":
		return fmt.Sprintf("Comando %s ainda esta pendente de definicao.", cmd), errChatwootCommandPending
	default:
		return "", nil
	}
}

func (s *server) chatwootStatusMessage(userID string) string {
	client := clientManager.GetWhatsmeowClient(userID)
	if client == nil {
		return "Sessao nao iniciada."
	}
	return fmt.Sprintf(
		"Status da sessao:\nConectado: %s\nLogado: %s",
		boolToYesNo(client.IsConnected()),
		boolToYesNo(client.IsLoggedIn()),
	)
}

func (s *server) chatwootQRCodeMessage(userID string) (string, error) {
	code, err := s.loadChatwootQRCode(userID)
	if err != nil {
		return chatwootQRCodeErrorMessage(err), err
	}
	return fmt.Sprintf("Aqui esta seu QR code:\n%s", code), nil
}

func (s *server) loadChatwootQRCode(userID string) (string, error) {
	client := clientManager.GetWhatsmeowClient(userID)
	if client == nil {
		return "", errChatwootNoSession
	}
	if !client.IsConnected() {
		return "", errChatwootNotConnected
	}
	if client.IsLoggedIn() {
		return "", errChatwootAlreadyLinked
	}

	var code string
	query := `SELECT qrcode FROM users WHERE id = ? LIMIT 1`
	if err := s.db.Get(&code, s.db.Rebind(query), userID); err != nil {
		return "", fmt.Errorf("load qrcode: %w", err)
	}
	if strings.TrimSpace(code) == "" {
		return "", errChatwootQRCodeMissing
	}
	return code, nil
}

func chatwootQRCodeErrorMessage(err error) string {
	switch {
	case errors.Is(err, errChatwootNoSession):
		return "Nenhuma sessao ativa. Conecte primeiro."
	case errors.Is(err, errChatwootNotConnected):
		return "Sessao nao conectada. Conecte primeiro."
	case errors.Is(err, errChatwootAlreadyLinked):
		return "Sessao ja esta conectada."
	case errors.Is(err, errChatwootQRCodeMissing):
		return "QR code ainda nao esta disponivel. Tente novamente em alguns segundos."
	default:
		return "Nao foi possivel gerar o QR code agora."
	}
}

func (s *server) chatwootDisconnectMessage(userID string) (string, error) {
	if err := s.disconnectChatwootSession(userID); err != nil {
		switch {
		case errors.Is(err, errChatwootNoSession):
			return "Nenhuma sessao ativa.", err
		case errors.Is(err, errChatwootNotConnected):
			return "Sessao ja esta desconectada.", err
		default:
			return "Falha ao desconectar a sessao.", err
		}
	}
	return "Sessao desconectada.", nil
}

func (s *server) disconnectChatwootSession(userID string) error {
	client := clientManager.GetWhatsmeowClient(userID)
	if client == nil {
		return errChatwootNoSession
	}
	if !client.IsConnected() {
		return errChatwootNotConnected
	}

	_, err := s.db.Exec(s.db.Rebind("UPDATE users SET connected=0, events=? WHERE id=?"), "", userID)
	if err != nil {
		return fmt.Errorf("update user connection: %w", err)
	}

	if token, err := s.lookupUserToken(userID); err == nil {
		if cachedUserInfo, found := userinfocache.Get(token); found {
			updatedUserInfo := updateUserInfo(cachedUserInfo, "Events", "")
			userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
		}
	}

	clientManager.DeleteWhatsmeowClient(userID)
	if ch, ok := killchannel[userID]; ok {
		select {
		case ch <- true:
		default:
		}
	}
	return nil
}

func (s *server) lookupUserToken(userID string) (string, error) {
	var token string
	query := `SELECT token FROM users WHERE id = ? LIMIT 1`
	if err := s.db.Get(&token, s.db.Rebind(query), userID); err != nil {
		return "", fmt.Errorf("load user token: %w", err)
	}
	return token, nil
}

func boolToYesNo(value bool) string {
	if value {
		return "sim"
	}
	return "nao"
}
