package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// TestMyEventHandlerEmptyDeliveredReceipt covers the Receipt panic guard: a
// "Delivered" receipt with an empty MessageIDs slice indexed [0]. In production
// whatsmeow's dispatch recovers the panic (so it is not a crash), but the
// receipt is silently dropped and a stack trace is logged. Called directly here
// there is no dispatch recover, so an empty receipt must not panic.
func TestMyEventHandlerEmptyDeliveredReceipt(t *testing.T) {
	s := makeTestServer(t)
	mycli := &MyClient{userID: "evt-receipt-user", token: "evt-receipt-user", db: s.db, s: s}

	// MessageIDs intentionally left empty — this previously panicked.
	mycli.myEventHandler(&events.Receipt{
		Type: types.ReceiptTypeDelivered,
	})
}
