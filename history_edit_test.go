package main

import (
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestHistoryEditContent(t *testing.T) {
	cases := []struct {
		name    string
		message *waE2E.Message
		want    string
	}{
		{"text", &waE2E.Message{Conversation: proto.String("changed")}, "changed"},
		{"extended", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("extended")}}, "extended"},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("image")}}, "image"},
		{"video", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("video")}}, "video"},
		{"document", &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{Caption: proto.String("document")}}}}, "document"},
		{"empty caption", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("")}}, ""},
	}
	client := &whatsmeow.Client{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := client.BuildEdit(types.NewJID("123", types.DefaultUserServer), "original", tc.message)
			before := proto.Clone(raw)
			live := (&events.Message{RawMessage: raw}).UnwrapRaw().Message
			for _, message := range []*waE2E.Message{raw, live} {
				got, ok := historyEdit(message)
				if !ok || got.target != "original" || got.text != tc.want {
					t.Fatalf("got %+v, %v", got, ok)
				}
			}
			if !proto.Equal(raw, before) {
				t.Fatal("changed original protobuf")
			}
		})
	}
	for _, message := range []*waE2E.Message{nil, {}, {Conversation: proto.String("normal")}, {ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum()}}} {
		if _, ok := historyEdit(message); ok {
			t.Fatal("non-edit recognized as edit")
		}
	}
}

func TestHistoryEditPersistence(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO users (id,name,token) VALUES ('edit-user','Edit','edit-token')"); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		save := func(id, kind, text, target, chat string) {
			t.Helper()
			if err := s.saveMessageToHistory("edit-user", chat, "sender", id, kind, text, "", target, `{"edit":true}`); err != nil {
				t.Fatal(err)
			}
		}
		save("original", "text", "original text", "", "chat")
		save("edit-1", "unknown", "", "", "chat")
		var oldTimestamp string
		if err := db.Get(&oldTimestamp, "SELECT CAST(timestamp AS TEXT) FROM message_history WHERE message_id='edit-1'"); err != nil {
			t.Fatal(err)
		}
		save("edit-1", "edit", "replacement", "original", "chat")
		save("edit-1", "edit", "replacement", "original", "chat")
		save("edit-1", "unknown", "", "", "chat")
		save("edit-2", "edit", "", "original", "chat")
		save("original", "edit", "must not replace", "original", "chat")
		save("other-chat", "unknown", "", "", "other")
		save("other-chat", "edit", "must not replace", "original", "chat")
		var rows []struct {
			ID        string `db:"message_id"`
			Kind      string `db:"message_type"`
			Text      string `db:"text_content"`
			Target    string `db:"quoted_message_id"`
			Timestamp string `db:"saved_time"`
		}
		if err := db.Select(&rows, "SELECT message_id,message_type,text_content,quoted_message_id,CAST(timestamp AS TEXT) AS saved_time FROM message_history ORDER BY message_id"); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 4 {
			t.Fatalf("rows: %+v", rows)
		}
		if rows[0].Kind != "edit" || rows[0].Text != "replacement" || rows[0].Target != "original" || rows[0].Timestamp != oldTimestamp {
			t.Fatalf("upgrade: %+v", rows[0])
		}
		if rows[1].Kind != "edit" || rows[1].Text != "" || rows[1].Target != "original" {
			t.Fatalf("empty second edit: %+v", rows[1])
		}
		if rows[2].Kind != "text" || rows[2].Text != "original text" {
			t.Fatalf("original changed: %+v", rows[2])
		}
		if rows[3].Kind != "unknown" {
			t.Fatalf("other chat changed: %+v", rows[3])
		}
	})
}
