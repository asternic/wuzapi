package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestUpdateUserInfoDoesNotLogCredentials(t *testing.T) {
	preserveLogLevel(t)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	previous := log.Logger
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })

	for _, field := range []string{"Token", "S3AccessKey", "S3SecretKey", "HmacKeyEncrypted", "Proxy", "Webhook"} {
		t.Run(field, func(t *testing.T) {
			output.Reset()
			secret := "credential-sentinel-" + field
			original := Values{map[string]string{"Id": "test-user"}}
			updated := updateUserInfo(original, field, secret).(Values)
			if updated.Get(field) != secret {
				t.Fatal("credential update was not preserved")
			}
			if strings.Contains(output.String(), secret) {
				t.Fatal("credential leaked into log output")
			}
			if !strings.Contains(output.String(), "User info updated") {
				t.Fatal("expected diagnostic log was not emitted")
			}
		})
	}
}

func TestPairingWebhookDoesNotLogPayload(t *testing.T) {
	preserveLogLevel(t)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	previous := log.Logger
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })

	const secret = "pairing-credential-sentinel"
	sendToUserWebHookWithHmac("", "", []byte(`{"type":"PasskeyConfirmation","code":"`+secret+`"}`), "test-user", "unused-token", nil)
	if strings.Contains(output.String(), secret) {
		t.Fatal("pairing credential leaked through webhook debug logging")
	}
	if !strings.Contains(output.String(), "jsonDataBytes") {
		t.Fatal("expected webhook metadata log was not emitted")
	}
}

func TestMissingWebhookUserDoesNotLogToken(t *testing.T) {
	previous := log.Logger
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })
	preserveLogLevel(t)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	const token = "missing-webhook-user-credential-sentinel"
	if got := getUserWebhookUrl(token); got != "" {
		t.Fatal("unexpected webhook for missing user")
	}
	if strings.Contains(output.String(), token) {
		t.Fatal("missing user warning leaked the token")
	}
	if !strings.Contains(output.String(), "Could not call webhook") {
		t.Fatal("expected warning was not emitted")
	}
}
