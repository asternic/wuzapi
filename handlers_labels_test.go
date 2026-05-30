package main

import "testing"

func TestSaveLabelChatAssociationDeletesUnlabeledRows(t *testing.T) {
	s := makeTestServer(t)

	userID := "user-1"
	labelID := "label-1"
	chatJID := "5511999999999@s.whatsapp.net"

	if err := s.saveLabelChatAssociation(userID, labelID, chatJID, true); err != nil {
		t.Fatalf("saveLabelChatAssociation(true) failed: %v", err)
	}
	requireAssociationCount(t, s, `
		SELECT COUNT(*)
		FROM label_chat_associations
		WHERE user_id = ? AND label_id = ? AND chat_jid = ?`, 1, userID, labelID, chatJID)

	if err := s.saveLabelChatAssociation(userID, labelID, chatJID, false); err != nil {
		t.Fatalf("saveLabelChatAssociation(false) failed: %v", err)
	}
	requireAssociationCount(t, s, `
		SELECT COUNT(*)
		FROM label_chat_associations
		WHERE user_id = ? AND label_id = ? AND chat_jid = ?`, 0, userID, labelID, chatJID)
}

func TestSaveLabelMessageAssociationDeletesUnlabeledRows(t *testing.T) {
	s := makeTestServer(t)

	userID := "user-1"
	labelID := "label-1"
	chatJID := "5511999999999@s.whatsapp.net"
	messageID := "message-1"

	if err := s.saveLabelMessageAssociation(userID, labelID, chatJID, messageID, true); err != nil {
		t.Fatalf("saveLabelMessageAssociation(true) failed: %v", err)
	}
	requireAssociationCount(t, s, `
		SELECT COUNT(*)
		FROM label_message_associations
		WHERE user_id = ? AND label_id = ? AND chat_jid = ? AND message_id = ?`, 1, userID, labelID, chatJID, messageID)

	if err := s.saveLabelMessageAssociation(userID, labelID, chatJID, messageID, false); err != nil {
		t.Fatalf("saveLabelMessageAssociation(false) failed: %v", err)
	}
	requireAssociationCount(t, s, `
		SELECT COUNT(*)
		FROM label_message_associations
		WHERE user_id = ? AND label_id = ? AND chat_jid = ? AND message_id = ?`, 0, userID, labelID, chatJID, messageID)
}

func requireAssociationCount(t *testing.T, s *server, query string, expected int, args ...interface{}) {
	t.Helper()

	var associationCount int
	if err := s.db.Get(&associationCount, query, args...); err != nil {
		t.Fatalf("failed to count label associations: %v", err)
	}
	if associationCount != expected {
		t.Fatalf("expected %d label association rows, got %d", expected, associationCount)
	}
}
