package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/keys"
)

type importSessionPayload struct {
	NoiseCandidates []struct {
		Private string `json:"private"`
		Public  string `json:"public"`
	} `json:"noiseCandidates"`
	IdentityKey struct {
		Private string `json:"private"`
		Public  string `json:"public"`
	} `json:"identityKey"`
	RegistrationID uint32 `json:"registrationId"`
	AdvSecretKey   string `json:"advSecretKey"`
	Account        struct {
		Details             string `json:"details"`
		AccountSignatureKey string `json:"accountSignatureKey"`
		AccountSignature    string `json:"accountSignature"`
		DeviceSignature     string `json:"deviceSignature"`
	} `json:"account"`
	ID       string `json:"id"`
	LID      string `json:"lid"`
	Platform string `json:"platform"`
}

func decodeB64(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func (s *server) ImportSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		var payload importSessionPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid JSON payload"))
			return
		}

		if len(payload.NoiseCandidates) == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("noiseCandidates is required"))
			return
		}
		if payload.ID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("id (JID) is required"))
			return
		}

		// Find the correct noise candidate by verifying the keypair mathematically:
		// derive the public key from the private key and compare with the provided public key.
		var noiseKeyPair *keys.KeyPair
		for i, candidate := range payload.NoiseCandidates {
			priv, err := decodeB64(candidate.Private)
			if err != nil || len(priv) != 32 {
				continue
			}
			pub, err := decodeB64(candidate.Public)
			if err != nil || len(pub) != 32 {
				continue
			}
			derived := keys.NewKeyPairFromPrivateKey(*(*[32]byte)(priv))
			if string(derived.Pub[:]) == string(pub) {
				noiseKeyPair = derived
				log.Info().Int("candidateIndex", i).Str("userid", txtid).Msg("Noise keypair candidate verified mathematically")
				break
			}
		}
		if noiseKeyPair == nil {
			// Fallback to last candidate if none matched
			lastPriv, err := decodeB64(payload.NoiseCandidates[len(payload.NoiseCandidates)-1].Private)
			if err != nil || len(lastPriv) != 32 {
				s.Respond(w, r, http.StatusBadRequest, errors.New("invalid noiseKey: no valid candidate found"))
				return
			}
			noiseKeyPair = keys.NewKeyPairFromPrivateKey(*(*[32]byte)(lastPriv))
			log.Warn().Str("userid", txtid).Msg("No noise candidate matched mathematically; using last as fallback")
		}

		identPriv, err := decodeB64(payload.IdentityKey.Private)
		if err != nil || len(identPriv) != 32 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid identityKey: must be 32 bytes"))
			return
		}

		advSecret, err := decodeB64(payload.AdvSecretKey)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid advSecretKey"))
			return
		}

		advDetails, err := decodeB64(payload.Account.Details)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid account.details"))
			return
		}
		advAccSig, _ := decodeB64(payload.Account.AccountSignature)
		advAccSigKey, _ := decodeB64(payload.Account.AccountSignatureKey)
		advDevSig, _ := decodeB64(payload.Account.DeviceSignature)

		jid, err := types.ParseJID(payload.ID)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid JID: "+err.Error()))
			return
		}

		var lid types.JID
		if payload.LID != "" {
			lid, _ = types.ParseJID(payload.LID)
		}

		identKeyPair := keys.NewKeyPairFromPrivateKey(*(*[32]byte)(identPriv))

		device := container.NewDevice()
		device.NoiseKey = noiseKeyPair
		device.IdentityKey = identKeyPair
		device.SignedPreKey = identKeyPair.CreateSignedPreKey(1)
		device.RegistrationID = payload.RegistrationID
		device.AdvSecretKey = advSecret
		device.Account = &waAdv.ADVSignedDeviceIdentity{
			Details:             advDetails,
			AccountSignatureKey: advAccSigKey,
			AccountSignature:    advAccSig,
			DeviceSignature:     advDevSig,
		}
		device.ID = &jid
		device.LID = lid
		device.Platform = payload.Platform

		ctx := context.Background()
		if err := device.Save(ctx); err != nil {
			log.Error().Err(err).Str("userid", txtid).Msg("Failed to save imported session")
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to save session: "+err.Error()))
			return
		}

		if _, err = s.db.Exec("UPDATE users SET jid=$1 WHERE id=$2", jid.String(), txtid); err != nil {
			log.Error().Err(err).Str("userid", txtid).Msg("Failed to update users.jid after session import")
		}

		userinfocache.Delete(token)

		log.Info().Str("jid", jid.String()).Str("userid", txtid).Msg("Session imported successfully")

		resp := map[string]interface{}{
			"details": "Sessão importada com sucesso. Conecte agora.",
			"jid":     jid.String(),
		}
		respJSON, _ := json.Marshal(resp)
		s.Respond(w, r, http.StatusOK, string(respJSON))
	}
}
