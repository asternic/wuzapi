package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
)

// parseWebAuthnChallenge extrai o campo "challenge" (base64url) do JSON WebAuthn
// PublicKeyCredentialRequestOptions que o servidor WhatsApp envia via passkey_request_options.
func parseWebAuthnChallenge(blob []byte) ([]byte, error) {
	var opts struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(blob, &opts); err != nil || opts.Challenge == "" {
		// fallback: usa o blob bruto (mantém compatibilidade se o formato mudar)
		return blob, nil
	}
	return base64.RawURLEncoding.DecodeString(opts.Challenge)
}

// loadOrCreatePasskeyAuthenticator carrega o VirtualAuthenticator persistido para o usuário,
// ou cria um novo se ainda não existir. A chave privada é armazenada em JSON na coluna
// passkey_authenticator da tabela users, garantindo persistência entre reinícios.
func loadOrCreatePasskeyAuthenticator(db *sqlx.DB, userID string, log zerolog.Logger) *whatsmeow.VirtualAuthenticator {
	var stored sql.NullString
	if err := db.QueryRow("SELECT passkey_authenticator FROM users WHERE id=$1", userID).Scan(&stored); err != nil {
		log.Warn().Err(err).Str("userID", userID).Msg("[passkey] failed to load stored authenticator, creating new one")
	}

	if stored.Valid && stored.String != "" {
		va, err := whatsmeow.ImportVirtualAuthenticator([]byte(stored.String))
		if err == nil {
			va.ParseRequestOptions = parseWebAuthnChallenge
			log.Debug().Str("userID", userID).Msg("[passkey] loaded existing VirtualAuthenticator from DB")
			return va
		}
		log.Warn().Err(err).Str("userID", userID).Msg("[passkey] failed to import stored authenticator, creating new one")
	}

	va, err := whatsmeow.NewVirtualAuthenticator()
	if err != nil {
		log.Error().Err(err).Str("userID", userID).Msg("[passkey] failed to create VirtualAuthenticator")
		return nil
	}
	// O servidor envia o blob como JSON WebAuthn (PublicKeyCredentialRequestOptions).
	// O default do VirtualAuthenticator usa o blob bruto como challenge — incorreto.
	// Aqui extraímos o campo "challenge" em base64url.
	va.ParseRequestOptions = parseWebAuthnChallenge

	exported, err := va.Export()
	if err != nil {
		log.Error().Err(err).Str("userID", userID).Msg("[passkey] failed to export VirtualAuthenticator")
		return va
	}

	if _, err = db.Exec("UPDATE users SET passkey_authenticator=$1 WHERE id=$2", string(exported), userID); err != nil {
		log.Warn().Err(err).Str("userID", userID).Msg("[passkey] failed to persist VirtualAuthenticator")
	} else {
		log.Info().Str("userID", userID).Msg("[passkey] created and persisted new VirtualAuthenticator")
	}

	return va
}
