package main

import "testing"

// TestParseJIDEmptyString guards a real panic: parseJID indexed arg[0] before
// checking length, so an empty string — reachable from handlers that don't
// pre-validate, e.g. an empty GroupJID in SetGroupName/SetGroupTopic — panicked
// with "index out of range [0] with length 0". Verified red→green.
func TestParseJIDEmptyString(t *testing.T) {
	if _, ok := parseJID(""); ok { // must not panic
		t.Errorf(`parseJID("") returned ok=true, want false`)
	}
	if _, ok := parseJID("5491155554444"); !ok {
		t.Errorf("parseJID of a bare number should still succeed")
	}
	if _, ok := parseJID("+5491155554444"); !ok {
		t.Errorf("parseJID of a +-prefixed number should still succeed")
	}
}
