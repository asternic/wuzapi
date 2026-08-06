package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
)

type exportedAccount struct {
	Details             string `json:"details"`
	AccountSignatureKey string `json:"accountSignatureKey"`
	AccountSignature    string `json:"accountSignature"`
	DeviceSignature     string `json:"deviceSignature"`
}

type exportSessionPayload struct {
	NoisePrivate          string          `json:"noisePrivate"`
	NoisePublic           string          `json:"noisePublic"`
	IdentityPrivate       string          `json:"identityPrivate"`
	IdentityPublic        string          `json:"identityPublic"`
	RegistrationID        uint32          `json:"registrationId"`
	AdvSecretKey          string          `json:"advSecretKey"`
	Account               exportedAccount `json:"account"`
	ID                    string          `json:"jid"`
	LID                   string          `json:"lid"`
	Platform              string          `json:"platform"`
	SignedPreKeyID        uint32          `json:"signedPreKeyId"`
	SignedPreKeyPublic    string          `json:"signedPreKeyPublic"`
	SignedPreKeyPrivate   string          `json:"signedPreKeyPrivate"`
	SignedPreKeySignature string          `json:"signedPreKeySignature"`
}

// ExportSession devolve as credenciais criptográficas da sessão conectada, no mesmo formato
// aceito por ImportSession (espelho exato, campo a campo) — usado pra migrar o Server 2 (Wuzapi)
// pra outro provider sem reconectar/reescanear QR.
func (s *server) ExportSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil || client.Store == nil {
			s.Respond(w, r, http.StatusServiceUnavailable, errors.New("session not initialized"))
			return
		}
		device := client.Store
		if device.ID == nil || device.Account == nil || device.NoiseKey == nil || device.IdentityKey == nil || device.SignedPreKey == nil {
			s.Respond(w, r, http.StatusServiceUnavailable, errors.New("session not fully paired yet"))
			return
		}

		b64 := base64.StdEncoding.EncodeToString

		payload := exportSessionPayload{
			NoisePrivate:    b64(device.NoiseKey.Priv[:]),
			NoisePublic:     b64(device.NoiseKey.Pub[:]),
			IdentityPrivate: b64(device.IdentityKey.Priv[:]),
			IdentityPublic:  b64(device.IdentityKey.Pub[:]),
			RegistrationID:  device.RegistrationID,
			AdvSecretKey:    b64(device.AdvSecretKey),
			Account: exportedAccount{
				Details:             b64(device.Account.GetDetails()),
				AccountSignatureKey: b64(device.Account.GetAccountSignatureKey()),
				AccountSignature:    b64(device.Account.GetAccountSignature()),
				DeviceSignature:     b64(device.Account.GetDeviceSignature()),
			},
			ID:                    device.ID.String(),
			Platform:              device.Platform,
			SignedPreKeyID:        device.SignedPreKey.KeyID,
			SignedPreKeyPublic:    b64(device.SignedPreKey.Pub[:]),
			SignedPreKeyPrivate:   b64(device.SignedPreKey.Priv[:]),
			SignedPreKeySignature: b64(device.SignedPreKey.Signature[:]),
		}
		if !device.LID.IsEmpty() {
			payload.LID = device.LID.String()
		}

		respJSON, _ := json.Marshal(payload)
		s.Respond(w, r, http.StatusOK, string(respJSON))
	}
}
