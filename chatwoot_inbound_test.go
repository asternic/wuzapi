package main

import (
	"context"
	"testing"

	"wuzapi/internal/chatwoot"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type chatwootStub struct {
	calls []string
}

func (s *chatwootStub) CreateContact(ctx context.Context, inboxIdentifier string, payload chatwoot.CreateContactRequest) (string, error) {
	s.calls = append(s.calls, "CreateContact")
	return "contact-1", nil
}

func (s *chatwootStub) CreateConversation(ctx context.Context, inboxIdentifier, contactIdentifier string, payload chatwoot.CreateConversationRequest) (int, error) {
	s.calls = append(s.calls, "CreateConversation")
	return 101, nil
}

func (s *chatwootStub) CreateMessage(ctx context.Context, inboxIdentifier, contactIdentifier string, conversationID int, content string) (string, error) {
	s.calls = append(s.calls, "CreateMessage")
	return "msg-1", nil
}

func (s *chatwootStub) ToggleConversationStatus(ctx context.Context, conversationID int, status string) error {
	s.calls = append(s.calls, "ToggleStatus")
	return nil
}

func TestChatwootInboundIgnoresFromMe(t *testing.T) {
	s := makeTestServer(t)
	cfg := &ChatwootConfig{Enabled: true, InboxIdentifier: "inbox-1"}

	evt := newTestMessageEvent("5511999999999@s.whatsapp.net", "5511999999999@s.whatsapp.net", true, false, "hi")
	stub := &chatwootStub{}

	if err := s.handleChatwootInboundWithClient(context.Background(), cfg, evt, "user-1", stub); err != nil {
		t.Fatalf("handleChatwootInboundWithClient: %v", err)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no calls, got %v", stub.calls)
	}
}

func TestChatwootInboundIgnoresGroups(t *testing.T) {
	s := makeTestServer(t)
	cfg := &ChatwootConfig{Enabled: true, InboxIdentifier: "inbox-1", IgnoreGroups: true}

	evt := newTestMessageEvent("12345@g.us", "5511999999999@s.whatsapp.net", false, true, "hi")
	stub := &chatwootStub{}

	if err := s.handleChatwootInboundWithClient(context.Background(), cfg, evt, "user-1", stub); err != nil {
		t.Fatalf("handleChatwootInboundWithClient: %v", err)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no calls, got %v", stub.calls)
	}
}

func TestChatwootInboundCreatesContactConversationMessage(t *testing.T) {
	s := makeTestServer(t)
	cfg := &ChatwootConfig{Enabled: true, InboxIdentifier: "inbox-1"}

	evt := newTestMessageEvent("5511999999999@s.whatsapp.net", "5511999999999@s.whatsapp.net", false, false, "hi")
	stub := &chatwootStub{}

	if err := s.handleChatwootInboundWithClient(context.Background(), cfg, evt, "user-1", stub); err != nil {
		t.Fatalf("handleChatwootInboundWithClient: %v", err)
	}

	expected := []string{"CreateContact", "CreateConversation", "CreateMessage"}
	if len(stub.calls) != len(expected) {
		t.Fatalf("expected %v calls, got %v", expected, stub.calls)
	}
	for i, call := range expected {
		if stub.calls[i] != call {
			t.Fatalf("expected call %s at %d, got %s", call, i, stub.calls[i])
		}
	}

	mapping, err := s.GetChatwootMapByJID("user-1", "5511999999999@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetChatwootMapByJID: %v", err)
	}
	if mapping.ChatwootContactIdentifier != "contact-1" {
		t.Fatalf("expected contact identifier contact-1, got %s", mapping.ChatwootContactIdentifier)
	}
	if !mapping.ChatwootConversationID.Valid || mapping.ChatwootConversationID.Int64 != 101 {
		t.Fatalf("expected conversation id 101, got %v", mapping.ChatwootConversationID)
	}
}

func newTestMessageEvent(chatJID, senderJID string, isFromMe, isGroup bool, text string) *events.Message {
	chat, _ := types.ParseJID(chatJID)
	sender, _ := types.ParseJID(senderJID)

	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   sender,
			IsFromMe: isFromMe,
			IsGroup:  isGroup,
		},
		ID:       types.MessageID("msg-1"),
		PushName: "Alice",
	}

	msg := &waE2E.Message{Conversation: proto.String(text)}
	return &events.Message{Info: info, Message: msg}
}
