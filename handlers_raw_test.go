package main

import (
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"strings"
	"testing"
	"time"
)

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

func TestRawMessageValidation(t *testing.T) {
	for _, payload := range []string{
		`{}`, `null`, `{"imageMessage":{}}`, `{"viewOnceMessage":{"message":{}}}`, `{"typo":true}`, `{"conversation":"hi","typo":true}`,
		`{"protocolMessage":{"type":"REVOKE"}}`,
		`{"senderKeyDistributionMessage":{}}`,
		`{"fastRatchetKeySenderKeyDistributionMessage":{}}`,
		`{"deviceSentMessage":{"message":{"conversation":"hi"}}}`,
		`{"viewOnceMessage":{"message":{"protocolMessage":{"type":"REVOKE"}}}}`,
		`{"extendedTextMessage":{"text":"hi","contextInfo":{"quotedMessage":{"protocolMessage":{"type":"REVOKE"}}}}}`,
	} {
		if _, err := decodeRawMessage([]byte(payload)); err == nil {
			t.Errorf("accepted %s", payload)
		}
	}
	if _, err := decodeRawMessage([]byte(`{"viewOnceMessage":{"message":{"conversation":"hi"}}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRawMessageHistory(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('raw-user','Raw','raw-token',100)"); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		info := Values{m: map[string]string{"Id": "raw-user", "History": "100"}}
		sent := time.Unix(1700000000, 0)
		for i, payload := range []string{`{"conversation":"hello"}`, `{"productMessage":{"product":{"productID":"1","title":"T"}}}`} {
			msg, err := decodeRawMessage([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			id := []string{"TEXT", "PRODUCT"}[i]
			s.recordRawMessage(info, types.NewJID("999", types.DefaultUserServer), id, msg, whatsmeow.SendResponse{Timestamp: sent})
			var row HistoryMessage
			if err := db.Get(&row, db.Rebind("SELECT * FROM message_history WHERE user_id=? AND message_id=?"), "raw-user", id); err != nil {
				t.Fatal(err)
			}
			if row.SenderJID != "me" || row.SentAt.Unix() != sent.Unix() {
				t.Fatalf("invalid row: %+v", row)
			}
			if i == 0 && (row.MessageType != "text" || row.TextContent != "hello") {
				t.Fatalf("invalid text: %+v", row)
			}
			if i == 1 && (row.MessageType != "raw" || !strings.Contains(row.DataJson, "productMessage")) {
				t.Fatalf("product lost: %+v", row)
			}
			var event struct{ Info struct{ IsFromMe bool } }
			if err := json.Unmarshal([]byte(row.DataJson), &event); err != nil || !event.Info.IsFromMe {
				t.Fatalf("invalid envelope: %s", row.DataJson)
			}
		}
	})
}
