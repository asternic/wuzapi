package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestSQLiteHistoryTrimWithoutSecrets(t *testing.T) {
	for _, brokenSecrets := range []bool{false, true} {
		t.Run(fmt.Sprintf("broken_secrets_%t", brokenSecrets), func(t *testing.T) {
			db, err := sqlx.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			for _, query := range []string{
				`CREATE TABLE users (id TEXT PRIMARY KEY, history INTEGER)`,
				`CREATE TABLE message_history (id INTEGER PRIMARY KEY, user_id TEXT, chat_jid TEXT, sender_jid TEXT, message_id TEXT, timestamp DATETIME, sent_at DATETIME, message_type TEXT, text_content TEXT, media_link TEXT, quoted_message_id TEXT, datajson TEXT, UNIQUE(user_id, message_id))`,
				`INSERT INTO users (id, history) VALUES ('user', 2)`,
			} {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			if brokenSecrets {
				if _, err := db.Exec(`CREATE TABLE whatsmeow_message_secrets (other_column TEXT)`); err != nil {
					t.Fatal(err)
				}
			}
			s := &server{db: db}
			for i := 0; i < 3; i++ {
				if err := s.saveMessageToHistory("user", "chat", "sender", fmt.Sprint(i), "text", "hello", "", "", "{}", time.Now().Add(time.Duration(i)*time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			var count int
			if err := db.Get(&count, `SELECT COUNT(*) FROM message_history`); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("got %d history rows, want 2", count)
			}
		})
	}
}
