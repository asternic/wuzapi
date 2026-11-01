package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func outgoingReaction(text string) *waE2E.Message {
	return &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
		Key:  &waCommon.MessageKey{ID: proto.String("target")},
		Text: proto.String(text),
	}}
}

// TestOutgoingActionRows checks the row stored for each action, including the
// send's own ID, the send time and the unwrapped datajson that edits need.
func TestOutgoingActionRows(t *testing.T) {
	client := deleteTestClient()
	chat := types.NewJID("999", types.DefaultUserServer)
	revoke, err := buildDeleteMessage(client, chat, "", "target")
	if err != nil {
		t.Fatal(err)
	}
	edit := client.BuildEdit(chat, "target", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("changed")}})
	sent := time.Unix(1700000000, 0)
	cases := []struct {
		id, kind, text, quoted string
		message                *waE2E.Message
	}{
		{"REACTION", "reaction", "👍", "target", outgoingReaction("👍")},
		{"UNREACTION", "reaction", "", "target", outgoingReaction("")},
		{"EDIT", "edit", "changed", "target", edit},
		{"REVOKE", "delete", "target", "", revoke},
	}
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('action-user','Action','action-token',100)"); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		userinfo := Values{m: map[string]string{"Id": "action-user", "History": "100"}}
		// The original stays untouched: actions never share its ID.
		if err := s.saveMessageToHistory("action-user", chat.String(), "me", "target", "text", "original", "", "", "{}", sent); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			s.saveOutgoingAction(userinfo, chat, whatsmeow.SendResponse{ID: tc.id, Timestamp: sent}, tc.message)
			var row HistoryMessage
			if err := db.Get(&row, db.Rebind("SELECT * FROM message_history WHERE user_id=? AND message_id=?"), "action-user", tc.id); err != nil {
				t.Fatalf("%s: %v", tc.id, err)
			}
			if row.ChatJID != chat.String() || row.SenderJID != "me" || row.MessageType != tc.kind || row.TextContent != tc.text || row.QuotedMessageID != tc.quoted {
				t.Fatalf("%s: %+v", tc.id, row)
			}
			if row.SentAt == nil || row.SentAt.Unix() != sent.Unix() {
				t.Fatalf("%s: sent_at %v", tc.id, row.SentAt)
			}
			var event struct {
				Info    struct{ IsFromMe bool }
				Message map[string]json.RawMessage
			}
			if err := json.Unmarshal([]byte(row.DataJson), &event); err != nil {
				t.Fatal(err)
			}
			if !event.Info.IsFromMe {
				t.Fatalf("%s: not marked as own: %s", tc.id, row.DataJson)
			}
			if tc.kind == "edit" && (event.Message["protocolMessage"] == nil || event.Message["editedMessage"] != nil) {
				t.Fatalf("edit datajson must be unwrapped like received edits: %s", row.DataJson)
			}
		}
		var original string
		if err := db.Get(&original, "SELECT text_content FROM message_history WHERE message_id='target'"); err != nil || original != "original" {
			t.Fatalf("original changed: %q, %v", original, err)
		}
	})
}

func TestOutgoingActionRespectsDisabledHistory(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('off-user','Off','off-token',0)"); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		userinfo := Values{m: map[string]string{"Id": "off-user", "History": "0"}}
		s.saveOutgoingAction(userinfo, types.NewJID("999", types.DefaultUserServer), whatsmeow.SendResponse{ID: "OFF"}, outgoingReaction("👍"))
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM message_history"); err != nil || count != 0 {
			t.Fatalf("stored %d rows, %v", count, err)
		}
	})
}

func TestOutgoingActionIgnoresOtherMessages(t *testing.T) {
	for _, message := range []*waE2E.Message{{}, {Conversation: proto.String("text")}} {
		if action, ok := outgoingAction(message); ok {
			t.Fatalf("recognized %+v", action)
		}
	}
}

// A failed send must leave no row: this client is not logged in, so sends return an error.
func TestFailedActionsStoreNothing(t *testing.T) {
	const user = "action-failure-test"
	clientManager.SetWhatsmeowClient(user, whatsmeow.NewClient(&store.Device{}, nil))
	defer clientManager.DeleteWhatsmeowClient(user)
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		for path, handler := range map[string]http.HandlerFunc{"/chat/react": s.React(), "/chat/send/edit": s.SendEditMessage(), "/chat/delete": s.DeleteMessage()} {
			body := strings.NewReader(`{"Phone":"5491100000000@s.whatsapp.net","Body":"x","Id":"target"}`)
			req := httptest.NewRequest(http.MethodPost, path, body)
			req = req.WithContext(context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": user, "History": "100"}}))
			response := httptest.NewRecorder()
			handler(response, req)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("%s got %d: %s", path, response.Code, response.Body.String())
			}
		}
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM message_history"); err != nil || count != 0 {
			t.Fatalf("stored %d rows, %v", count, err)
		}
	})
}
