package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func specialHistoryCases() []struct {
	name, kind, text string
	msg              *waE2E.Message
} {
	return []struct {
		name, kind, text string
		msg              *waE2E.Message
	}{
		{"event", "event", "Meeting", &waE2E.Message{EventMessage: &waE2E.EventMessage{Name: proto.String("Meeting")}}},
		{"empty-event", "event", ":event:", &waE2E.Message{EventMessage: &waE2E.EventMessage{}}},
		{"invite", "event_invite", "Invite", &waE2E.Message{EventInviteMessage: &waE2E.EventInviteMessage{EventTitle: proto.String("Invite")}}},
		{"empty-invite", "event_invite", "", &waE2E.Message{EventInviteMessage: &waE2E.EventInviteMessage{}}},
		{"product", "product", "Product", &waE2E.Message{ProductMessage: &waE2E.ProductMessage{Product: &waE2E.ProductMessage_ProductSnapshot{Title: proto.String("Product")}}}},
		{"empty-product", "product", "", &waE2E.Message{ProductMessage: &waE2E.ProductMessage{}}},
		{"interactive", "interactive", "Body", &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{Body: &waE2E.InteractiveMessage_Body{Text: proto.String("Body")}}}},
		{"empty-interactive", "interactive", ":interactive:", &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{}}},
		{"response", "interactive_response", "Reply", &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{Body: &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Reply")}}}},
		{"empty-response", "interactive_response", "", &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{}}},
		{"order", "order", "Order", &waE2E.Message{OrderMessage: &waE2E.OrderMessage{OrderTitle: proto.String("Order")}}},
		{"empty-order", "order", "", &waE2E.Message{OrderMessage: &waE2E.OrderMessage{}}},
		{"template", "template", "Your bill is ready", &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{HydratedContentText: proto.String("Your bill is ready")}}}},
		{"empty-template", "template", ":template:", &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{}}},
		{"list", "list", "Pick one", &waE2E.Message{ListMessage: &waE2E.ListMessage{Title: proto.String("Menu"), Description: proto.String("Pick one")}}},
		{"list-title", "list", "Menu", &waE2E.Message{ListMessage: &waE2E.ListMessage{Title: proto.String("Menu")}}},
		{"empty-list", "list", ":list:", &waE2E.Message{ListMessage: &waE2E.ListMessage{}}},
		{"buttons", "buttons", "Choose", &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{ContentText: proto.String("Choose")}}},
		{"empty-buttons", "buttons", ":buttons:", &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{}}},
		{"template-reply", "template_button_reply", "Yes", &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{SelectedID: proto.String("yes-id"), SelectedDisplayText: proto.String("Yes")}}},
		{"template-reply-id", "template_button_reply", "yes-id", &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{SelectedID: proto.String("yes-id")}}},
		{"buttons-response", "buttons_response", "btn-1", &waE2E.Message{ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{SelectedButtonID: proto.String("btn-1")}}},
		{"list-response", "list_response", "row-1", &waE2E.Message{ListResponseMessage: &waE2E.ListResponseMessage{SingleSelectReply: &waE2E.ListResponseMessage_SingleSelectReply{SelectedRowID: proto.String("row-1")}}}},
	}
}

func TestHistorySpecialMessage(t *testing.T) {
	for _, tc := range specialHistoryCases() {
		t.Run(tc.name, func(t *testing.T) {
			kind, text := historySpecialMessage(tc.msg)
			if kind != tc.kind || text != tc.text {
				t.Fatalf("got %q, %q; want %q, %q", kind, text, tc.kind, tc.text)
			}
		})
	}
	for _, msg := range []*waE2E.Message{nil, {}, {Conversation: proto.String("text")}} {
		if kind, text := historySpecialMessage(msg); kind != "" || text != "" {
			t.Fatalf("non-special message classified as %q, %q", kind, text)
		}
	}
}

// A template carries its content in one of several formats; all of them must yield the body text.
func TestHistorySpecialTemplateFormats(t *testing.T) {
	cases := map[string]*waE2E.TemplateMessage{
		"hydrated-four-row": {Format: &waE2E.TemplateMessage_HydratedFourRowTemplate_{HydratedFourRowTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{HydratedContentText: proto.String("Body")}}},
		"interactive":       {Format: &waE2E.TemplateMessage_InteractiveMessageTemplate{InteractiveMessageTemplate: &waE2E.InteractiveMessage{Body: &waE2E.InteractiveMessage_Body{Text: proto.String("Body")}}}},
	}
	for name, tpl := range cases {
		t.Run(name, func(t *testing.T) {
			if kind, text := historySpecialMessage(&waE2E.Message{TemplateMessage: tpl}); kind != "template" || text != "Body" {
				t.Fatalf("got %q, %q; want template, Body", kind, text)
			}
		})
	}
}

// Exercise the actual event handlers and database, including messages whose
// extracted content is empty and would otherwise be dropped by the live path.
func TestHistorySpecialPersistence(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		db.SetMaxOpenConns(1)
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		const user, token = "special-history-user", "special-history-token"
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('special-history-user','Test','special-history-token',100)"); err != nil {
			t.Fatal(err)
		}
		container, err := sqlstore.New(context.Background(), "sqlite", filepath.Join(t.TempDir(), "whatsmeow.db")+"?_pragma=foreign_keys(1)", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { container.Close() })
		device := container.NewDevice()
		owner := types.NewJID("100", types.DefaultUserServer)
		device.ID = &owner
		device.Contacts = sqlstore.NewSQLStore(container, owner)
		client := &MyClient{WAClient: whatsmeow.NewClient(device, nil), userID: user, token: token, db: db, s: &server{db: db}}
		userinfocache.Set(token, Values{m: map[string]string{"Id": user, "Token": token, "History": "100", "S3Enabled": "false"}}, cache.DefaultExpiration)
		t.Cleanup(func() { userinfocache.Delete(token); lastMessageCache.Delete(user) })
		chat := types.NewJID("200", types.DefaultUserServer)
		sent := time.Unix(1700000000, 0)
		data := &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{{ID: proto.String(chat.String())}}}
		cases := specialHistoryCases()
		for _, tc := range cases {
			client.myEventHandler(&events.Message{Info: types.MessageInfo{
				MessageSource: types.MessageSource{Chat: chat, Sender: chat}, ID: "live-" + tc.name, Timestamp: sent,
			}, Message: tc.msg})
			data.Conversations[0].Messages = append(data.Conversations[0].Messages, &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
				Key:     &waCommon.MessageKey{ID: proto.String("sync-" + tc.name), RemoteJID: proto.String(chat.String())},
				Message: tc.msg, MessageTimestamp: proto.Uint64(uint64(sent.Unix())),
			}})
		}
		// A final ordinary message marks completion of the asynchronous sync batch.
		data.Conversations[0].Messages = append(data.Conversations[0].Messages, &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
			Key: &waCommon.MessageKey{ID: proto.String("sync-done")}, Message: &waE2E.Message{Conversation: proto.String("done")},
		}})
		client.myEventHandler(&events.HistorySync{Data: data})
		deadline := time.Now().Add(5 * time.Second)
		for {
			var done int
			if err := db.Get(&done, "SELECT COUNT(*) FROM message_history WHERE message_id='sync-done'"); err != nil {
				t.Fatal(err)
			}
			if done == 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("HistorySync batch did not finish")
			}
			time.Sleep(10 * time.Millisecond)
		}
		for _, mode := range []string{"live", "sync"} {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("%s/%s", mode, tc.name), func(t *testing.T) {
					var row HistoryMessage
					if err := db.Get(&row, db.Rebind("SELECT * FROM message_history WHERE user_id=? AND message_id=?"), user, mode+"-"+tc.name); err != nil {
						t.Fatal(err)
					}
					if row.MessageType != tc.kind || row.TextContent != tc.text || row.ChatJID != chat.String() || row.SenderJID != chat.String() {
						t.Fatalf("unexpected stored row: %+v", row)
					}
					if row.SentAt == nil || row.SentAt.Unix() != sent.Unix() {
						t.Fatalf("unexpected send time: %v", row.SentAt)
					}
					var stored struct{ Message *waE2E.Message }
					if err := json.Unmarshal([]byte(row.DataJson), &stored); err != nil {
						t.Fatal(err)
					}
					if !proto.Equal(stored.Message, tc.msg) {
						t.Fatal("full message content was not preserved")
					}
				})
			}
		}
	})
}
