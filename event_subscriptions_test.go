package main

import "testing"

// TestResolveConnectEvents covers the Connect-side half of issue #305: a connect
// call with no subscribe list must PRESERVE the user's existing subscriptions
// instead of overwriting them with an empty string. A subscribe list still
// replaces them (keeping only supported, de-duplicated types).
func TestResolveConnectEvents(t *testing.T) {
	cases := []struct {
		name        string
		subscribe   []string
		existing    string
		wantEvents  string
		wantChanged bool
	}{
		{"no subscribe preserves existing", nil, "Message,ReadReceipt", "Message,ReadReceipt", false},
		{"empty subscribe preserves existing", []string{}, "Message", "Message", false},
		{"no subscribe, no existing stays empty", nil, "", "", false},
		{"valid subscribe replaces and flags change", []string{"Message", "ReadReceipt"}, "Old", "Message,ReadReceipt", true},
		{"subscribe equal to existing yields no change", []string{"Message"}, "Message", "Message", false},
		{"subscribe equal to multi-value existing yields no change", []string{"Message", "ReadReceipt"}, "Message,ReadReceipt", "Message,ReadReceipt", false},
		{"unsupported types are filtered", []string{"Message", "Bogus"}, "", "Message", true},
		{"duplicates are de-duplicated", []string{"Message", "Message"}, "", "Message", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotEvents, gotChanged := resolveConnectEvents(tc.subscribe, tc.existing)
			if gotEvents != tc.wantEvents || gotChanged != tc.wantChanged {
				t.Fatalf("resolveConnectEvents(%v, %q) = (%q, %v); want (%q, %v)",
					tc.subscribe, tc.existing, gotEvents, gotChanged, tc.wantEvents, tc.wantChanged)
			}
		})
	}
}

// TestSetDisconnectedState covers the Disconnect-side half of issue #305:
// disconnect preserves event subscriptions by default and only clears them when
// explicitly asked (clear=true). Verified against the real schema via an
// in-memory DB.
func TestSetDisconnectedState(t *testing.T) {
	const id = "user-305"

	seed := func(t *testing.T, s *server) {
		t.Helper()
		if _, err := s.db.Exec(
			`INSERT INTO users (id, name, token, events, connected) VALUES ($1,$2,$3,$4,$5)`,
			id, "tester", "tok-305", "Message,ReadReceipt", 1); err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	read := func(t *testing.T, s *server) (events string, connected int) {
		t.Helper()
		if err := s.db.QueryRow(`SELECT events, connected FROM users WHERE id=$1`, id).Scan(&events, &connected); err != nil {
			t.Fatalf("read user: %v", err)
		}
		return events, connected
	}

	t.Run("default preserves events", func(t *testing.T) {
		s := makeTestServer(t)
		seed(t, s)
		if err := s.setDisconnectedState(id, false); err != nil {
			t.Fatalf("setDisconnectedState: %v", err)
		}
		events, connected := read(t, s)
		if events != "Message,ReadReceipt" {
			t.Errorf("events = %q; want preserved %q", events, "Message,ReadReceipt")
		}
		if connected != 0 {
			t.Errorf("connected = %d; want 0", connected)
		}
	})

	t.Run("clear=true resets events", func(t *testing.T) {
		s := makeTestServer(t)
		seed(t, s)
		if err := s.setDisconnectedState(id, true); err != nil {
			t.Fatalf("setDisconnectedState: %v", err)
		}
		events, connected := read(t, s)
		if events != "" {
			t.Errorf("events = %q; want cleared %q", events, "")
		}
		if connected != 0 {
			t.Errorf("connected = %d; want 0", connected)
		}
	})
}
