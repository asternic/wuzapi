package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPinMessageEndpoint drives POST /chat/pin through the real router and auth
// middleware. With no client connected the handler returns "no session"; the
// point is to prove the route is wired (not 404) and auth passes (not 401).
// The payload validation paths (missing Chat/Id/Sender, bad duration) all run
// after the session/connection checks, so they require a live whatsmeow client
// and cannot be exercised here without a connected account.
func TestPinMessageEndpoint(t *testing.T) {
	s := makeTestServer(t)
	const token = "tok-pin"
	if _, err := s.db.Exec(
		`INSERT INTO users (id, name, token, connected) VALUES ($1,$2,$3,$4)`,
		"u-pin", "tester", token, 0); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	bodies := map[string]string{
		"pin":   `{"Chat":"120363012345678901@g.us","Sender":"5491123456789@s.whatsapp.net","Id":"3EB0ABCDEF1234567890","DurationSeconds":604800,"Pin":true}`,
		"unpin": `{"Chat":"120363012345678901@g.us","Sender":"5491123456789@s.whatsapp.net","Id":"3EB0ABCDEF1234567890","Pin":false}`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/chat/pin", strings.NewReader(body))
			req.Header.Set("token", token)
			rr := httptest.NewRecorder()
			s.router.ServeHTTP(rr, req)

			if rr.Code == http.StatusNotFound {
				t.Fatalf("POST /chat/pin not registered (404)")
			}
			if rr.Code == http.StatusUnauthorized {
				t.Fatalf("auth failed for a valid token (401): %s", rr.Body.String())
			}
			if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "no session") {
				t.Errorf("expected 500 \"no session\" (route hit, no client); got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}
