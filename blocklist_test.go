package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// TestFormatBlocklist covers the response shaping for GET /user/blocklist:
// a nil blocklist yields an empty list (never null) and an empty dhash, and a
// populated blocklist stringifies every JID and passes the dhash through.
func TestFormatBlocklist(t *testing.T) {
	t.Run("nil yields empty list and dhash", func(t *testing.T) {
		got := formatBlocklist(nil)
		jids, ok := got["Blocklist"].([]string)
		if !ok {
			t.Fatalf("Blocklist is not []string: %T", got["Blocklist"])
		}
		if len(jids) != 0 {
			t.Errorf("len(Blocklist) = %d; want 0", len(jids))
		}
		if got["DHash"] != "" {
			t.Errorf("DHash = %v; want empty", got["DHash"])
		}
	})

	t.Run("prefers PN and falls back to LID while preserving dhash", func(t *testing.T) {
		j1, ok1 := parseJID("5491155554445")
		j2, ok2 := parseJID("456@lid")
		if !ok1 || !ok2 {
			t.Fatalf("parseJID failed: ok1=%v ok2=%v", ok1, ok2)
		}
		bl := &types.Blocklist{
			DHash: "1234567890",
			Items: []types.BlocklistItem{
				{PN: j1, LID: types.NewJID("123", types.HiddenUserServer)},
				{LID: j2},
			},
		}

		got := formatBlocklist(bl)
		jids := got["Blocklist"].([]string)
		if len(jids) != len(bl.Items) {
			t.Fatalf("len(Blocklist) = %d; want %d", len(jids), len(bl.Items))
		}
		for i, expected := range []types.JID{j1, j2} {
			if jids[i] != expected.String() {
				t.Errorf("Blocklist[%d] = %q; want %q", i, jids[i], expected.String())
			}
		}
		if got["DHash"] != "1234567890" {
			t.Errorf("DHash = %v; want %q", got["DHash"], "1234567890")
		}
	})
}

// TestGetBlocklistEndpoint exercises GET /user/blocklist through the real router
// and auth middleware. With no WhatsApp client connected in tests, the handler
// returns the "no session" error — the point is to prove the route is wired
// (not a 404) and authentication passes for a valid token (not a 401).
func TestGetBlocklistEndpoint(t *testing.T) {
	s := makeTestServer(t)
	const token = "tok-blocklist"
	if _, err := s.db.Exec(
		`INSERT INTO users (id, name, token, connected) VALUES ($1,$2,$3,$4)`,
		"u-bl", "tester", token, 0); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/blocklist", nil)
	req.Header.Set("token", token)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Fatalf("route /user/blocklist not registered (404)")
	}
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("auth failed for a valid token (401): %s", rr.Body.String())
	}
	// No client is connected in tests, so the handler reports "no session".
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "no session") {
		t.Errorf("expected 500 \"no session\" (route hit, no client); got %d: %s", rr.Code, rr.Body.String())
	}
}
