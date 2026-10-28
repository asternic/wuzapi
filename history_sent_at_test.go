package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// TestHistorySentAt checks that message_history keeps the send time WhatsApp
// reported (sent_at) apart from the time the row was stored (timestamp), stores
// NULL when the send time is unknown, keeps the first value when a message is
// redelivered, and returns it from /chat/history.
func TestHistorySentAt(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		// Running the migrations again must be a no-op.
		if err := initializeSchema(db); err != nil {
			t.Fatalf("second initializeSchema: %v", err)
		}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('sent-user','Sent','sent-token',100)"); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		const chat = "123456@s.whatsapp.net"
		// A non-UTC offset catches PostgreSQL silently dropping the zone when
		// this column is timestamp without time zone.
		sent := time.Unix(1700000000, 0).In(time.FixedZone("UTC-3", -3*60*60))

		if err := s.saveMessageToHistory("sent-user", chat, chat, "WITH-TIME", "text", "old", "", "", "{}", sent); err != nil {
			t.Fatal(err)
		}
		if err := s.saveMessageToHistory("sent-user", chat, chat, "NO-TIME", "text", "unknown", "", "", "{}", time.Time{}); err != nil {
			t.Fatal(err)
		}
		// A HistorySync redelivery of the same message must not replace sent_at.
		if err := s.saveMessageToHistory("sent-user", chat, chat, "WITH-TIME", "text", "old", "", "", "{}", sent.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}

		var rows []HistoryMessage
		if err := db.Select(&rows, db.Rebind("SELECT message_id, timestamp, sent_at FROM message_history WHERE user_id = ? ORDER BY message_id"), "sent-user"); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		if rows[0].MessageID != "NO-TIME" || rows[0].SentAt != nil {
			t.Fatalf("unknown send time should be NULL: %+v", rows[0])
		}
		if rows[1].MessageID != "WITH-TIME" || rows[1].SentAt == nil || rows[1].SentAt.Unix() != sent.Unix() {
			t.Fatalf("sent_at not stored or overwritten: %+v", rows[1])
		}
		// timestamp is still the storage time, not the send time.
		if rows[1].Timestamp.Unix() == sent.Unix() {
			t.Fatalf("timestamp should be the insert time, got the send time")
		}

		req := httptest.NewRequest(http.MethodGet, "/chat/history?chat_jid="+chat+"&limit=10", nil)
		req = req.WithContext(context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": "sent-user", "History": "100"}}))
		rec := httptest.NewRecorder()
		s.GetHistory()(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /chat/history: %d %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		found := map[string]interface{}{}
		for _, m := range body.Data {
			found[m["message_id"].(string)] = m["sent_at"]
		}
		if got, ok := found["WITH-TIME"].(string); !ok {
			t.Fatalf("sent_at missing from /chat/history: %s", rec.Body.String())
		} else if parsed, err := time.Parse(time.RFC3339, got); err != nil || parsed.Unix() != sent.Unix() {
			t.Fatalf("sent_at in /chat/history = %q, want %s", got, sent)
		}
		if v, ok := found["NO-TIME"]; !ok || v != nil {
			t.Fatalf("unknown sent_at should be omitted, got %v (present=%v)", v, ok)
		}
	})
}

func TestHistorySentAtTimezoneMigration(t *testing.T) {
	db := historyTestDB(t, "postgres")
	// Keep SET TIME ZONE and all queries on the same database session.
	db.SetMaxOpenConns(1)
	if err := createMigrationsTable(db); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if migration.ID < 17 {
			if err := applyMigration(db, migration); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`
		INSERT INTO users (id,name,token,history) VALUES ('tz-user','TZ','tz-token',100);
		INSERT INTO message_history (user_id,chat_jid,sender_jid,message_id,timestamp,sent_at,message_type,text_content,media_link)
		VALUES ('tz-user','chat','sender','legacy',CURRENT_TIMESTAMP,'2023-11-14 22:13:20','text','',''),
		       ('tz-user','chat','sender','unknown',CURRENT_TIMESTAMP,NULL,'text','','');
		SET TIME ZONE 'America/Argentina/Buenos_Aires';
	`); err != nil {
		t.Fatal(err)
	}
	if err := initializeSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := initializeSchema(db); err != nil {
		t.Fatalf("repeated migration: %v", err)
	}
	var columnType string
	if err := db.Get(&columnType, `SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'message_history' AND column_name = 'sent_at'`); err != nil {
		t.Fatal(err)
	}
	if columnType != "timestamp with time zone" {
		t.Fatalf("unexpected sent_at type: %s", columnType)
	}
	var legacy HistoryMessage
	if err := db.Get(&legacy, `SELECT sent_at FROM message_history WHERE message_id = 'legacy'`); err != nil {
		t.Fatal(err)
	}
	if legacy.SentAt == nil || legacy.SentAt.Unix() != 1700000000 {
		t.Fatalf("migration changed the legacy UTC interpretation: %v", legacy.SentAt)
	}
	var unknown HistoryMessage
	if err := db.Get(&unknown, `SELECT sent_at FROM message_history WHERE message_id = 'unknown'`); err != nil {
		t.Fatal(err)
	}
	if unknown.SentAt != nil {
		t.Fatalf("migration replaced NULL: %v", unknown.SentAt)
	}
	s := &server{db: db}
	for _, offset := range []int{-3 * 3600, 5*3600 + 30*60} {
		sent := time.Unix(1700000000, 0).In(time.FixedZone("test", offset))
		id := sent.Format("15:04:05Z07:00")
		if err := s.saveMessageToHistory("tz-user", "chat", "sender", id, "text", "", "", "", "{}", sent); err != nil {
			t.Fatal(err)
		}
		var row HistoryMessage
		if err := db.Get(&row, `SELECT sent_at FROM message_history WHERE message_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if row.SentAt == nil || !row.SentAt.Equal(sent) {
			t.Fatalf("offset %d changed the instant: got %v, want %v", offset, row.SentAt, sent)
		}
	}
}
