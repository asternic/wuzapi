package main

import "testing"

func TestDecodeRawMessage(t *testing.T) {
	m, err := decodeRawMessage([]byte(`{"productMessage":{"product":{"productID":"1","title":"T","productImageCount":5},"businessOwnerJID":"1@lid"}}`))
	if err != nil || m.GetProductMessage().GetProduct().GetProductID() != "1" || m.GetProductMessage().GetBusinessOwnerJID() != "1@lid" {
		t.Fatalf("product decode: %v %v", err, m)
	}
	m, err = decodeRawMessage([]byte(`{"eventInviteMessage":{"eventID":"9","eventTitle":"E","startTime":"1794857400"}}`))
	if err != nil || m.GetEventInviteMessage().GetEventID() != "9" || m.GetEventInviteMessage().GetStartTime() != 1794857400 {
		t.Fatalf("event decode: %v %v", err, m)
	}
	if _, err = decodeRawMessage([]byte(`{}`)); err == nil {
		t.Fatal("empty message accepted")
	}
}
