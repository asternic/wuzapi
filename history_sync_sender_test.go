package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
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

func TestHistorySyncSenderPersistence(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		db.SetMaxOpenConns(1)
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		const user = "history-sender-test"
		if _, err := db.Exec("INSERT INTO users (id,name,token) VALUES ('history-sender-test','Test','history-sender-token')"); err != nil {
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
		client := &MyClient{WAClient: whatsmeow.NewClient(device, nil), userID: user, token: "history-sender-token", db: db, s: &server{db: db}}
		cases := []struct {
			id, chat, keyParticipant, participant, want string
			fromMe                                      bool
		}{
			{"group-key", "123@g.us", "200@lid", "", "200@lid", false},
			{"group-lid", "123@g.us", "", "200@lid", "200@lid", false},
			{"group-phone", "123@g.us", "", "200@s.whatsapp.net", "200@s.whatsapp.net", false},
			{"key-precedence", "123@g.us", "200@lid", "300@lid", "200@lid", false},
			{"broadcast", "status@broadcast", "", "200@lid", "200@lid", false},
			{"missing-sender", "123@g.us", "", "", "", false},
			{"outgoing", "123@g.us", "", "200@lid", owner.String(), true},
			{"direct", "300@s.whatsapp.net", "", "200@lid", "300@s.whatsapp.net", false},
			// The final direct message marks completion of the asynchronous batch.
			{"done", "300@s.whatsapp.net", "", "", "300@s.whatsapp.net", false},
		}
		data := &waHistorySync.HistorySync{}
		for _, tc := range cases {
			data.Conversations = append(data.Conversations, &waHistorySync.Conversation{
				ID: proto.String(tc.chat),
				Messages: []*waHistorySync.HistorySyncMsg{{Message: &waWeb.WebMessageInfo{
					Key:         &waCommon.MessageKey{ID: proto.String(tc.id), RemoteJID: proto.String(tc.chat), FromMe: proto.Bool(tc.fromMe), Participant: proto.String(tc.keyParticipant)},
					Participant: proto.String(tc.participant), MessageTimestamp: proto.Uint64(1700000000),
					Message: &waE2E.Message{Conversation: proto.String("history test")},
				}}},
			})
		}
		client.myEventHandler(&events.HistorySync{Data: data})
		deadline := time.Now().Add(5 * time.Second)
		for {
			var done int
			if err := db.Get(&done, "SELECT COUNT(*) FROM message_history WHERE message_id='done'"); err != nil {
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
		for _, tc := range cases {
			t.Run(tc.id, func(t *testing.T) {
				var rows []struct {
					Sender string `db:"sender_jid"`
					Data   string `db:"datajson"`
				}
				if err := db.Select(&rows, db.Rebind("SELECT sender_jid,datajson FROM message_history WHERE user_id=? AND message_id=?"), user, tc.id); err != nil {
					t.Fatal(err)
				}
				if tc.want == "" {
					if len(rows) != 0 {
						t.Fatal("message without a sender was saved")
					}
					return
				}
				if len(rows) != 1 || rows[0].Sender != tc.want {
					t.Fatalf("rows: %+v; want sender %s", rows, tc.want)
				}
				var stored struct{ Info types.MessageInfo }
				if err := json.Unmarshal([]byte(rows[0].Data), &stored); err != nil {
					t.Fatal(err)
				}
				if stored.Info.Sender.String() != tc.want {
					t.Fatalf("Info.Sender = %s; want %s", stored.Info.Sender, tc.want)
				}
			})
		}
	})
}
