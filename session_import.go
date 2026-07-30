package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/keys"
	"google.golang.org/protobuf/proto"
)

type sessionImportPayload struct {
	Device     sessionDeviceImport      `json:"device"`
	Sessions   []sessionRecordImport    `json:"sessions"`
	Identities []sessionIdentityImport  `json:"identities"`
	PreKeys    []sessionPreKeyImport    `json:"pre_keys"`
	SenderKeys []sessionSenderKeyImport `json:"sender_keys"`
}

type sessionDeviceImport struct {
	JID               string  `json:"jid"`
	LID               string  `json:"lid"`
	RegistrationID    uint32  `json:"registration_id"`
	IdentityKey       string  `json:"identity_key"`
	NoiseKey          *string `json:"noise_key"`
	SignedPreKeyID    uint32  `json:"signed_pre_key_id"`
	SignedPreKey      string  `json:"signed_pre_key"`
	SignedPreKeySig   string  `json:"signed_pre_key_sig"`
	AdvSecretKey      string  `json:"adv_secret_key"`
	AdvSignedIdentity string  `json:"adv_signed_identity"`
	Platform          string  `json:"platform"`
	PushName          string  `json:"push_name"`
}

type sessionRecordImport struct {
	OurJID  string          `json:"our_jid"`
	TheirID string          `json:"their_id"`
	Session json.RawMessage `json:"session"`
}

type sessionIdentityImport struct {
	OurJID   string `json:"our_jid"`
	TheirID  string `json:"their_id"`
	Identity string `json:"identity"`
}

type sessionPreKeyImport struct {
	OurJID   string `json:"our_jid"`
	KeyID    uint32 `json:"key_id"`
	Key      string `json:"key"`
	Uploaded bool   `json:"uploaded"`
}

type sessionSenderKeyImport struct {
	OurJID    string `json:"our_jid"`
	ChatID    string `json:"chat_id"`
	SenderID  string `json:"sender_id"`
	SenderKey string `json:"sender_key"`
}

func (s *server) ImportSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")
		oldJID := r.Context().Value("userinfo").(Values).Get("Jid")

		var payload sessionImportPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		if payload.Device.JID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("device.jid is required"))
			return
		}

		forceDisconnectClient(txtid)

		jid, stats, err := importWhatsAppSession(r.Context(), s.exPath, payload, oldJID)
		if err != nil {
			log.Error().Err(err).Str("userId", txtid).Msg("Failed to import WhatsApp session")
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		_, err = s.db.Exec("UPDATE users SET jid=$1, connected=1, qrcode='' WHERE id=$2", jid.String(), txtid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to update user: %w", err))
			return
		}

		v := updateUserInfo(r.Context().Value("userinfo"), "Jid", jid.String())
		v = updateUserInfo(v, "Qrcode", "")
		userinfocache.Set(token, v, cache.NoExpiration)

		response := map[string]interface{}{
			"jid":     jid.String(),
			"details": "Session imported",
			"stats":   stats,
		}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(responseJSON))
	}
}

type sessionImportStats struct {
	Sessions   int `json:"sessions"`
	Identities int `json:"identities"`
	PreKeys    int `json:"pre_keys"`
	SenderKeys int `json:"sender_keys"`
}

func importWhatsAppSession(ctx context.Context, exPath string, payload sessionImportPayload, previousUserJID string) (types.JID, sessionImportStats, error) {
	var stats sessionImportStats

	device, jid, err := buildDeviceFromImport(payload.Device)
	if err != nil {
		return types.EmptyJID, stats, err
	}

	if err := deleteExistingDevice(ctx, jid, previousUserJID); err != nil {
		return types.EmptyJID, stats, err
	}

	if err := container.PutDevice(ctx, device); err != nil {
		return types.EmptyJID, stats, fmt.Errorf("failed to save device: %w", err)
	}

	deviceStore, err := container.GetDevice(ctx, jid)
	if err != nil {
		return types.EmptyJID, stats, fmt.Errorf("failed to load imported device: %w", err)
	}
	if deviceStore == nil {
		return types.EmptyJID, stats, errors.New("device was not persisted")
	}

	ourJID := jid.String()

	for _, sess := range payload.Sessions {
		if sess.TheirID == "" {
			return types.EmptyJID, stats, errors.New("session entry missing their_id")
		}
		sessionBytes, err := decodeSessionBytes(sess.Session)
		if err != nil {
			return types.EmptyJID, stats, fmt.Errorf("session %s: %w", sess.TheirID, err)
		}
		if err := deviceStore.Sessions.PutSession(ctx, sess.TheirID, sessionBytes); err != nil {
			return types.EmptyJID, stats, fmt.Errorf("failed to save session %s: %w", sess.TheirID, err)
		}
		stats.Sessions++
	}

	for _, ident := range payload.Identities {
		if ident.TheirID == "" {
			return types.EmptyJID, stats, errors.New("identity entry missing their_id")
		}
		key, err := decodeBase64Key32(ident.Identity, "identity")
		if err != nil {
			return types.EmptyJID, stats, fmt.Errorf("identity %s: %w", ident.TheirID, err)
		}
		if err := deviceStore.Identities.PutIdentity(ctx, ident.TheirID, key); err != nil {
			return types.EmptyJID, stats, fmt.Errorf("failed to save identity %s: %w", ident.TheirID, err)
		}
		stats.Identities++
	}

	for _, sk := range payload.SenderKeys {
		if sk.ChatID == "" || sk.SenderID == "" {
			return types.EmptyJID, stats, errors.New("sender_key entry missing chat_id or sender_id")
		}
		senderKey, err := decodeBase64Bytes(sk.SenderKey, "sender_key")
		if err != nil {
			return types.EmptyJID, stats, fmt.Errorf("sender_key %s/%s: %w", sk.ChatID, sk.SenderID, err)
		}
		if err := deviceStore.SenderKeys.PutSenderKey(ctx, sk.ChatID, sk.SenderID, senderKey); err != nil {
			return types.EmptyJID, stats, fmt.Errorf("failed to save sender_key %s/%s: %w", sk.ChatID, sk.SenderID, err)
		}
		stats.SenderKeys++
	}

	if len(payload.PreKeys) > 0 {
		if err := importPreKeys(ctx, exPath, ourJID, payload.PreKeys); err != nil {
			return types.EmptyJID, stats, err
		}
		stats.PreKeys = len(payload.PreKeys)
	}

	return jid, stats, nil
}

func buildDeviceFromImport(d sessionDeviceImport) (*store.Device, types.JID, error) {
	jid, ok := parseJID(d.JID)
	if !ok {
		return nil, types.EmptyJID, errors.New("invalid device.jid")
	}

	identityPriv, err := decodeBase64Key32(d.IdentityKey, "identity_key")
	if err != nil {
		return nil, types.EmptyJID, err
	}

	var noiseKey *keys.KeyPair
	if d.NoiseKey != nil && strings.TrimSpace(*d.NoiseKey) != "" {
		noisePriv, err := decodeBase64Key32(*d.NoiseKey, "noise_key")
		if err != nil {
			return nil, types.EmptyJID, err
		}
		noiseKey = keys.NewKeyPairFromPrivateKey(noisePriv)
	} else {
		noiseKey = keys.NewKeyPair()
		log.Warn().Str("jid", jid.String()).Msg("noise_key not provided; generated a new noise key pair")
	}

	signedPreKeyPriv, err := decodeBase64Key32(d.SignedPreKey, "signed_pre_key")
	if err != nil {
		return nil, types.EmptyJID, err
	}
	signedPreKeySig, err := decodeBase64Key64(d.SignedPreKeySig, "signed_pre_key_sig")
	if err != nil {
		return nil, types.EmptyJID, err
	}

	advSecret, err := decodeBase64Bytes(d.AdvSecretKey, "adv_secret_key")
	if err != nil {
		return nil, types.EmptyJID, err
	}

	advBytes, err := decodeBase64Bytes(d.AdvSignedIdentity, "adv_signed_identity")
	if err != nil {
		return nil, types.EmptyJID, err
	}
	var account waAdv.ADVSignedDeviceIdentity
	if err := proto.Unmarshal(advBytes, &account); err != nil {
		return nil, types.EmptyJID, fmt.Errorf("adv_signed_identity: invalid protobuf: %w", err)
	}

	var lid types.JID
	if strings.TrimSpace(d.LID) != "" {
		lid, ok = parseJID(d.LID)
		if !ok {
			return nil, types.EmptyJID, errors.New("invalid device.lid")
		}
	}

	device := &store.Device{
		ID:             &jid,
		LID:            lid,
		RegistrationID: d.RegistrationID,
		NoiseKey:       noiseKey,
		IdentityKey:    keys.NewKeyPairFromPrivateKey(identityPriv),
		SignedPreKey: &keys.PreKey{
			KeyPair:   *keys.NewKeyPairFromPrivateKey(signedPreKeyPriv),
			KeyID:     d.SignedPreKeyID,
			Signature: &signedPreKeySig,
		},
		AdvSecretKey: advSecret,
		Account:      &account,
		Platform:     d.Platform,
		PushName:     d.PushName,
	}

	return device, jid, nil
}

func deleteExistingDevice(ctx context.Context, newJID types.JID, previousUserJID string) error {
	jidsToDelete := map[string]struct{}{
		newJID.String(): {},
	}
	if previousUserJID != "" && previousUserJID != newJID.String() {
		jidsToDelete[previousUserJID] = struct{}{}
	}

	for jidStr := range jidsToDelete {
		jid, ok := parseJID(jidStr)
		if !ok {
			continue
		}
		existing, err := container.GetDevice(ctx, jid)
		if err != nil {
			return fmt.Errorf("failed to check existing device %s: %w", jidStr, err)
		}
		if existing != nil {
			if err := container.DeleteDevice(ctx, existing); err != nil {
				return fmt.Errorf("failed to delete existing device %s: %w", jidStr, err)
			}
		}
	}
	return nil
}

func importPreKeys(ctx context.Context, exPath, ourJID string, preKeys []sessionPreKeyImport) error {
	wdb, err := openWhatsmeowStoreDB(exPath)
	if err != nil {
		return err
	}
	defer wdb.Close()

	query := `INSERT INTO whatsmeow_pre_keys (jid, key_id, key, uploaded) VALUES ($1, $2, $3, $4)
		ON CONFLICT (jid, key_id) DO UPDATE SET key=excluded.key, uploaded=excluded.uploaded`
	if wdb.DriverName() == "sqlite" {
		query = `INSERT INTO whatsmeow_pre_keys (jid, key_id, key, uploaded) VALUES (?, ?, ?, ?)
			ON CONFLICT (jid, key_id) DO UPDATE SET key=excluded.key, uploaded=excluded.uploaded`
	}

	for _, pk := range preKeys {
		keyBytes, err := decodeBase64Key32(pk.Key, "pre_key")
		if err != nil {
			return fmt.Errorf("pre_key %d: %w", pk.KeyID, err)
		}
		if _, err := wdb.ExecContext(ctx, query, ourJID, pk.KeyID, keyBytes[:], pk.Uploaded); err != nil {
			return fmt.Errorf("failed to save pre_key %d: %w", pk.KeyID, err)
		}
	}
	return nil
}

func openWhatsmeowStoreDB(exPath string) (*sqlx.DB, error) {
	config := getDatabaseConfig(exPath, *dataDir)
	if config.Type == "postgres" {
		dsn := fmt.Sprintf(
			"user=%s password=%s dbname=%s host=%s port=%s sslmode=%s",
			config.User, config.Password, config.Name, config.Host, config.Port, config.SSLMode,
		)
		return sqlx.Open("postgres", dsn)
	}
	dbPath := filepath.Join(config.Path, "main.db")
	return sqlx.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)&_busy_timeout=3000")
}

func decodeBase64Bytes(value, field string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid base64: %w", field, err)
		}
	}
	return data, nil
}

func decodeBase64Key32(value, field string) ([32]byte, error) {
	data, err := decodeBase64Bytes(value, field)
	if err != nil {
		return [32]byte{}, err
	}
	if len(data) != 32 {
		return [32]byte{}, fmt.Errorf("%s must be 32 bytes, got %d", field, len(data))
	}
	return *(*[32]byte)(data), nil
}

func decodeBase64Key64(value, field string) ([64]byte, error) {
	data, err := decodeBase64Bytes(value, field)
	if err != nil {
		return [64]byte{}, err
	}
	if len(data) != 64 {
		return [64]byte{}, fmt.Errorf("%s must be 64 bytes, got %d", field, len(data))
	}
	return *(*[64]byte)(data), nil
}

func decodeSessionBytes(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("session data is empty")
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return decodeBase64Bytes(asString, "session")
	}

	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObject); err == nil {
		for _, key := range []string{"data", "session", "record", "Record", "bytes"} {
			if nested, ok := asObject[key]; ok {
				if bytes, err := decodeSessionBytes(nested); err == nil {
					return bytes, nil
				}
			}
		}
		return json.Marshal(asObject)
	}

	return raw, nil
}
