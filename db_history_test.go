package main

import (
	"testing"
)

// TestSaveMessageToHistoryIdempotent verifies the fix for #292: persisting a
// message whose (user_id, message_id) already exists must NOT return an error
// (the plain INSERT previously violated the message_history unique constraint
// and was logged at ERROR on every HistorySync), and must not create a
// duplicate row.
func TestSaveMessageToHistoryIdempotent(t *testing.T) {
	s := makeTestServer(t)

	const (
		userID = "user-1"
		chat   = "123456@s.whatsapp.net"
		sender = "123456@s.whatsapp.net"
		msgID  = "MSG-DUP-1"
	)

	// First insert: a normal live Message event.
	if err := s.saveMessageToHistory(userID, chat, sender, msgID, "text", "hello", "", "", "{}"); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	// Second insert with the same (user_id, message_id): simulates the same
	// message arriving again in a HistorySync batch or on reconnect. With the
	// fix this is a silent no-op; without it, it returns a unique-constraint
	// violation.
	if err := s.saveMessageToHistory(userID, chat, sender, msgID, "text", "hello", "", "", "{}"); err != nil {
		t.Fatalf("duplicate insert should be a silent no-op, got error: %v", err)
	}

	// Exactly one row must exist for this (user_id, message_id).
	var count int
	if err := s.db.Get(&count,
		"SELECT COUNT(*) FROM message_history WHERE user_id = ? AND message_id = ?",
		userID, msgID); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after duplicate insert, got %d", count)
	}

	// A different message_id for the same user must still insert normally
	// (the conflict clause must not swallow legitimate inserts).
	if err := s.saveMessageToHistory(userID, chat, sender, "MSG-OTHER", "text", "world", "", "", "{}"); err != nil {
		t.Fatalf("insert of a distinct message failed: %v", err)
	}
	if err := s.db.Get(&count,
		"SELECT COUNT(*) FROM message_history WHERE user_id = ?", userID); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 distinct rows, got %d", count)
	}
}
