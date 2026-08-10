package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestJidUserKey(t *testing.T) {
	tests := []struct {
		name string
		jid  string
		want string
	}{
		{"empty", "", ""},
		{"plain user", "380996987502@s.whatsapp.net", "380996987502"},
		{"with device suffix", "380996987502:8@s.whatsapp.net", "380996987502"},
		{"lid style", "1234567890:1@lid", "1234567890"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jidUserKey(tt.jid); got != tt.want {
				t.Fatalf("jidUserKey(%q) = %q, want %q", tt.jid, got, tt.want)
			}
		})
	}
}

func TestJidLookupCandidates(t *testing.T) {
	candidates := jidLookupCandidates("380996987502:8@s.whatsapp.net")
	if len(candidates) < 2 {
		t.Fatalf("expected multiple candidates, got %d", len(candidates))
	}

	seen := make(map[string]struct{})
	for _, c := range candidates {
		seen[c.String()] = struct{}{}
	}

	want := []string{
		"380996987502:8@s.whatsapp.net",
		"380996987502@s.whatsapp.net",
	}
	for _, w := range want {
		if _, ok := seen[w]; !ok {
			t.Fatalf("missing candidate %q in %v", w, candidates)
		}
	}

	if got := jidUserKey(candidates[0].String()); got != "380996987502" {
		t.Fatalf("first candidate key = %q, want 380996987502", got)
	}
}

func TestJidLookupCandidatesInvalid(t *testing.T) {
	if got := jidLookupCandidates(""); got != nil {
		t.Fatalf("empty jid should return nil, got %v", got)
	}
	if got := jidLookupCandidates("@s.whatsapp.net"); got != nil {
		t.Fatalf("jid without user should return nil, got %v", got)
	}
}

func TestJidLookupCandidatesPhoneOnly(t *testing.T) {
	candidates := jidLookupCandidates("380996987502")
	if len(candidates) != 1 {
		t.Fatalf("phone-only input should yield one candidate, got %d", len(candidates))
	}
	if candidates[0].Server != types.DefaultUserServer {
		t.Fatalf("expected default user server, got %q", candidates[0].Server)
	}
}
