package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidatePrivacySetting covers the input validation for POST /user/privacy:
// a setting name must be one of the supported types and the value must be allowed
// for that specific setting (per the whatsmeow-documented matrix).
func TestValidatePrivacySetting(t *testing.T) {
	cases := []struct {
		name, value string
		wantErr     bool
	}{
		{"last", "all", false},
		{"last", "contacts", false},
		{"last", "contact_blacklist", false},
		{"last", "none", false},
		{"groupadd", "contacts", false},
		{"status", "none", false},
		{"profile", "all", false},
		{"readreceipts", "all", false},
		{"readreceipts", "none", false},
		{"online", "match_last_seen", false},
		{"calladd", "known", false},
		// invalid value for the given setting
		{"readreceipts", "contacts", true},
		{"online", "contacts", true},
		{"calladd", "contacts", true},
		{"last", "sometimes", true},
		// Protocol-known but deliberately not exposed: whatsmeow's SetPrivacySetting
		// has no switch case for these, so a change would not round-trip into the
		// returned/cached settings. The values below ARE valid per the documented
		// matrix, proving they're rejected for the NAME, not the value.
		{"messages", "all", true},
		{"defense", "off", true},
		{"stickers", "contacts", true},
		// unknown / empty name
		{"bogus", "all", true},
		{"", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			err := validatePrivacySetting(tc.name, tc.value)
			if (err != nil) != tc.wantErr {
				t.Errorf("validatePrivacySetting(%q, %q) err=%v; wantErr=%v", tc.name, tc.value, err, tc.wantErr)
			}
		})
	}
}

// TestPrivacyEndpoints drives GET and POST /user/privacy through the real router
// and auth middleware. With no client connected the handler returns "no session";
// the point is to prove both routes are wired (not 404) and auth passes (not 401).
func TestPrivacyEndpoints(t *testing.T) {
	s := makeTestServer(t)
	const token = "tok-privacy"
	if _, err := s.db.Exec(
		`INSERT INTO users (id, name, token, connected) VALUES ($1,$2,$3,$4)`,
		"u-priv", "tester", token, 0); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	check := func(t *testing.T, method, body string) {
		t.Helper()
		req := httptest.NewRequest(method, "/user/privacy", strings.NewReader(body))
		req.Header.Set("token", token)
		rr := httptest.NewRecorder()
		s.router.ServeHTTP(rr, req)

		if rr.Code == http.StatusNotFound {
			t.Fatalf("%s /user/privacy not registered (404)", method)
		}
		if rr.Code == http.StatusUnauthorized {
			t.Fatalf("%s auth failed for a valid token (401): %s", method, rr.Body.String())
		}
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "no session") {
			t.Errorf("%s expected 500 \"no session\" (route hit, no client); got %d: %s", method, rr.Code, rr.Body.String())
		}
	}

	t.Run("GET", func(t *testing.T) { check(t, http.MethodGet, "") })
	t.Run("POST", func(t *testing.T) { check(t, http.MethodPost, `{"Name":"last","Value":"contacts"}`) })
}
