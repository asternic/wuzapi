package main

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestHistorySpecialMessage(t *testing.T) {
	tests := []struct {
		msg           *waE2E.Message
		kind, content string
	}{
		{&waE2E.Message{EventMessage: &waE2E.EventMessage{Name: proto.String("Meeting")}}, "event", "Meeting"},
		{&waE2E.Message{EventMessage: &waE2E.EventMessage{}}, "event", ":event:"},
		{&waE2E.Message{EventInviteMessage: &waE2E.EventInviteMessage{EventTitle: proto.String("Invite")}}, "event_invite", "Invite"},
		{&waE2E.Message{ProductMessage: &waE2E.ProductMessage{Product: &waE2E.ProductMessage_ProductSnapshot{Title: proto.String("Product")}}}, "product", "Product"},
		{&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{Body: &waE2E.InteractiveMessage_Body{Text: proto.String("Body")}}}, "interactive", "Body"},
		{&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{}}, "interactive", ":interactive:"},
		{&waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{Body: &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Reply")}}}, "interactive_response", "Reply"},
		{&waE2E.Message{OrderMessage: &waE2E.OrderMessage{OrderTitle: proto.String("Order")}}, "order", "Order"},
		{&waE2E.Message{}, "", ""},
	}
	for _, tt := range tests {
		kind, content := historySpecialMessage(tt.msg)
		if kind != tt.kind || content != tt.content {
			t.Errorf("got %q, %q; want %q, %q", kind, content, tt.kind, tt.content)
		}
	}
}
