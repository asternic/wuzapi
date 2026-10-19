package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMarkPlayedEndpoint(t *testing.T) {
	s := makeTestServer(t)
	const token = "tok-played"
	if _, err := s.db.Exec(
		`INSERT INTO users (id, name, token, connected) VALUES ($1,$2,$3,$4)`,
		"u-played", "tester", token, 0); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	bodies := map[string]string{"played": `{"ChatPhone":"5491112345678@s.whatsapp.net","Id":["ABC"]}`}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/chat/markplayed", strings.NewReader(body))
			req.Header.Set("token", token)
			rr := httptest.NewRecorder()
			s.router.ServeHTTP(rr, req)

			if rr.Code == http.StatusNotFound {
				t.Fatalf("POST /chat/markplayed not registered (404)")
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
