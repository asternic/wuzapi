package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestChatwootWebhookVerifiesSignatureAndQueuesOutgoingMessage(t *testing.T) {
	s := makeTestServer(t)
	key := configureChatwootWebhook(t, s, "webhook-instance", "webhook-instance-token", "webhook-secret", 44)
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_contact_map (instance_id, chat_jid, contact_id, source_id, conversation_id) VALUES (?, ?, ?, ?, ?)`), "webhook-instance", "5511999999999@s.whatsapp.net", 101, "wuzapi:5511999999999@s.whatsapp.net", 201); err != nil {
		t.Fatal(err)
	}
	body := `{"event":"message_created","id":301,"message_type":"outgoing","private":false,"inbox":{"id":44},"conversation":{"id":201},"content":"reply from agent","content_attributes":{}}`
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, signedChatwootRequest(t, key, body, "webhook-secret", time.Now().UTC()))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("webhook status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var count int
	if err := s.db.Get(&count, "SELECT COUNT(*) FROM chatwoot_outbox WHERE direction=? AND source_message_id=?", chatwootDirectionInbound, "301"); err != nil || count != 1 {
		t.Fatalf("queued inbound count = %d, %v", count, err)
	}
}

func TestChatwootWebhookRejectsBadSignatureAndOldTimestamp(t *testing.T) {
	s := makeTestServer(t)
	key := configureChatwootWebhook(t, s, "webhook-reject", "webhook-reject-token", "webhook-secret", 44)
	body := `{"event":"message_created","id":301,"message_type":"outgoing","private":false,"inbox":{"id":44},"conversation":{"id":201},"content":"fixture"}`
	for _, request := range []*http.Request{
		signedChatwootRequest(t, key, body, "wrong-secret", time.Now().UTC()),
		signedChatwootRequest(t, key, body, "webhook-secret", time.Now().UTC().Add(-6*time.Minute)),
	} {
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("webhook status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestChatwootWebhookSkipsLoopedAndUnmappedMessages(t *testing.T) {
	s := makeTestServer(t)
	key := configureChatwootWebhook(t, s, "webhook-loop", "webhook-loop-token", "webhook-secret", 44)
	for _, attributes := range []string{`{"wuzapi_source":true}`, `{}`} {
		body := `{"event":"message_created","id":302,"message_type":"outgoing","private":false,"inbox":{"id":44},"conversation":{"id":999},"content":"fixture","content_attributes":` + attributes + `}`
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, signedChatwootRequest(t, key, body, "webhook-secret", time.Now().UTC()))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("webhook status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	var count int
	if err := s.db.Get(&count, "SELECT COUNT(*) FROM chatwoot_outbox"); err != nil || count != 0 {
		t.Fatalf("queued rows = %d, %v", count, err)
	}
}

func TestChatwootWebhookDeduplicatesDeliveries(t *testing.T) {
	s := makeTestServer(t)
	key := configureChatwootWebhook(t, s, "webhook-dedup", "webhook-dedup-token", "webhook-secret", 44)
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_contact_map (instance_id, chat_jid, contact_id, source_id, conversation_id) VALUES (?, ?, ?, ?, ?)`), "webhook-dedup", "123@lid", 101, "wuzapi:123@lid", 201); err != nil {
		t.Fatal(err)
	}
	body := `{"event":"message_created","id":401,"message_type":"outgoing","private":false,"inbox":{"id":44},"conversation":{"id":201},"content":"fixture","content_attributes":{}}`
	for range 2 {
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, signedChatwootRequest(t, key, body, "webhook-secret", time.Now().UTC()))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("webhook status = %d", recorder.Code)
		}
	}
	var count int
	if err := s.db.Get(&count, "SELECT COUNT(*) FROM chatwoot_outbox WHERE direction=? AND source_message_id=?", chatwootDirectionInbound, "401"); err != nil || count != 1 {
		t.Fatalf("deduplicated rows = %d, %v", count, err)
	}
}

func configureChatwootWebhook(t *testing.T, s *server, instanceID, instanceToken, secret string, inboxID int64) string {
	t.Helper()
	previousKey, previousPersistent := *globalEncryptionKey, globalEncryptionKeyPersistent
	*globalEncryptionKey = "0123456789abcdef0123456789abcdef"
	globalEncryptionKeyPersistent = true
	t.Cleanup(func() {
		*globalEncryptionKey = previousKey
		globalEncryptionKeyPersistent = previousPersistent
	})
	if _, err := s.db.Exec("INSERT INTO users (id,name,token) VALUES (?,?,?)", instanceID, "Webhook", instanceToken); err != nil {
		t.Fatal(err)
	}
	secretCiphertext, err := encryptHMACKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	key := "fixture-" + instanceID
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_config (instance_id, enabled, url, account_id, token, inbox_id, inbox_name, webhook_key, webhook_secret) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), instanceID, true, "https://chatwoot.example.test", 1, secretCiphertext, inboxID, "Fixture", key, secretCiphertext); err != nil {
		t.Fatal(err)
	}
	return key
}

func signedChatwootRequest(t *testing.T, key, body, secret string, timestamp time.Time) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(timestamp.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(ts + "." + body))
	request := httptest.NewRequest(http.MethodPost, "/chatwoot/webhook/"+key, strings.NewReader(body))
	request.Header.Set("X-Chatwoot-Timestamp", ts)
	request.Header.Set("X-Chatwoot-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set("Content-Type", "application/json")
	return request
}
