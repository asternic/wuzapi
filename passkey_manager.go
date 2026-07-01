package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
)

type passkeyCredential struct {
	CredentialID []byte `json:"credentialId"`
	PrivateKey   []byte `json:"privateKey"` // PKCS8 DER
}

// loadOrCreatePasskeyCredential loads the persistent EC P-256 credential for a user,
// or generates a new one if none exists. Stored in the passkey_authenticator DB column.
func loadOrCreatePasskeyCredential(db *sqlx.DB, userID string, log zerolog.Logger) (*passkeyCredential, error) {
	var stored sql.NullString
	if err := db.QueryRow("SELECT passkey_authenticator FROM users WHERE id=$1", userID).Scan(&stored); err != nil {
		log.Warn().Err(err).Str("userID", userID).Msg("[passkey] failed to query stored credential")
	}

	if stored.Valid && stored.String != "" {
		var cred passkeyCredential
		if err := json.Unmarshal([]byte(stored.String), &cred); err == nil &&
			len(cred.CredentialID) > 0 && len(cred.PrivateKey) > 0 {
			log.Debug().Str("userID", userID).Msg("[passkey] loaded existing credential from DB")
			return &cred, nil
		}
		log.Warn().Str("userID", userID).Msg("[passkey] stored credential invalid, regenerating")
	}

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate EC P-256 key: %w", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal PKCS8 key: %w", err)
	}
	// Use the compressed public key as credential ID (33 bytes)
	credID := elliptic.MarshalCompressed(elliptic.P256(), privKey.PublicKey.X, privKey.PublicKey.Y)

	cred := &passkeyCredential{CredentialID: credID, PrivateKey: privBytes}
	data, _ := json.Marshal(cred)
	if _, dbErr := db.Exec("UPDATE users SET passkey_authenticator=$1 WHERE id=$2", string(data), userID); dbErr != nil {
		log.Warn().Err(dbErr).Str("userID", userID).Msg("[passkey] failed to persist new credential")
	} else {
		log.Info().Str("userID", userID).Msg("[passkey] generated and persisted new credential")
	}
	return cred, nil
}

// buildPasskeyResponse creates a WebAuthn assertion response for the given challenge.
// Implements the virtual authenticator: signs the authenticatorData + clientDataHash
// using the stored EC P-256 key.
func buildPasskeyResponse(cred *passkeyCredential, pubKey *types.WebAuthnPublicKey) (*types.WebAuthnResponse, error) {
	privKeyAny, err := x509.ParsePKCS8PrivateKey(cred.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	ecKey, ok := privKeyAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("expected *ecdsa.PrivateKey, got %T", privKeyAny)
	}

	// authenticatorData: rpIdHash(32) + flags(1: UP|UV=0x05) + signCount(4)
	rpIDHash := sha256.Sum256([]byte("web.whatsapp.com"))
	authData := make([]byte, 37)
	copy(authData[:32], rpIDHash[:])
	authData[32] = 0x05 // UP (user present) | UV (user verified)
	binary.BigEndian.PutUint32(authData[33:37], 0)

	// Challenge is already decoded to raw bytes by UnpaddedURLBytes unmarshaler
	challengeB64 := base64.RawURLEncoding.EncodeToString(pubKey.Challenge)
	clientDataJSON, err := json.Marshal(map[string]any{
		"type":        "webauthn.get",
		"challenge":   challengeB64,
		"origin":      "https://web.whatsapp.com",
		"crossOrigin": false,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal clientDataJSON: %w", err)
	}

	// Signature over sha256(authData || sha256(clientDataJSON))
	clientDataHash := sha256.Sum256(clientDataJSON)
	sigInput := append(authData, clientDataHash[:]...)
	sigHash := sha256.Sum256(sigInput)
	r, s, err := ecdsa.Sign(rand.Reader, ecKey, sigHash[:])
	if err != nil {
		return nil, fmt.Errorf("ecdsa sign: %w", err)
	}
	sigDER, err := asn1.Marshal(struct{ R, S *big.Int }{r, s})
	if err != nil {
		return nil, fmt.Errorf("marshal DER signature: %w", err)
	}

	credIDStr := base64.RawURLEncoding.EncodeToString(cred.CredentialID)
	return &types.WebAuthnResponse{
		ID:    credIDStr,
		RawID: cred.CredentialID,
		Type:  "public-key",
		Response: types.WebAuthnResponseData{
			ClientDataJSON:    clientDataJSON,
			AuthenticatorData: authData,
			Signature:         sigDER,
		},
	}, nil
}
