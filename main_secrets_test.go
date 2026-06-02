package main

import (
	"strings"
	"testing"
)

// TestSecureToken verifies the crypto/rand-backed generator used for the
// auto-generated admin token and global encryption/HMAC keys: correct length,
// alphanumeric-only output, randomness across calls, and a sane zero-length
// edge case.
func TestSecureToken(t *testing.T) {
	const n = 32
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	tok, err := secureToken(n)
	if err != nil {
		t.Fatalf("secureToken returned error: %v", err)
	}
	if len(tok) != n {
		t.Errorf("length = %d, want %d", len(tok), n)
	}
	for i, c := range tok {
		if !strings.ContainsRune(charset, c) {
			t.Errorf("char %q at index %d is outside the allowed charset", c, i)
		}
	}

	// Two independent calls must differ — a sanity check that the output is
	// actually random and not constant. Collision probability ~62^-32.
	other, err := secureToken(n)
	if err != nil {
		t.Fatalf("secureToken returned error: %v", err)
	}
	if tok == other {
		t.Errorf("two secureToken(%d) calls returned identical values — not random", n)
	}

	// Zero length is a valid edge case: empty string, no error.
	if empty, err := secureToken(0); err != nil || empty != "" {
		t.Errorf("secureToken(0) = %q, %v; want \"\", nil", empty, err)
	}
}
