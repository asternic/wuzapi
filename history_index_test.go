package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

// The legacy index must enforce the same user boundary as message history,
// including when the authenticated user has no history rows of their own.
func TestHistoryIndexTenantIsolation(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db, router: mux.NewRouter()}
		s.routes()
		for _, user := range []string{"index-a", "index-b", "index-empty"} {
			seedClientHistory(t, s, user, user+"-token", 100)
			userinfocache.Delete(user + "-token")
			t.Cleanup(func() { userinfocache.Delete(user + "-token") })
		}
		stamp := time.Now().UTC().Truncate(time.Second)
		insertClientMessage(t, s, "index-a", "123@lid", "a1", "first", stamp)
		insertClientMessage(t, s, "index-a", "123@lid", "a2", "latest", stamp.Add(time.Minute))
		insertClientMessage(t, s, "index-a", "456@s.whatsapp.net", "a3", "older", stamp)
		insertClientMessage(t, s, "index-b", "123@lid", "b1", "shared chat identifier", stamp.Add(time.Hour))
		insertClientMessage(t, s, "index-b", "789@g.us", "b2", "private group", stamp)

		check := func(t *testing.T, data map[string]interface{}, user string, chats []string) {
			t.Helper()
			if len(chats) == 0 {
				if len(data) != 0 {
					t.Fatalf("empty user's index leaked other users' data: %v", data)
				}
				return
			}
			rows, ok := data[user].([]interface{})
			if len(data) != 1 || !ok || len(rows) != len(chats) {
				t.Fatalf("index must contain only %s's chats: %v", user, data)
			}
			for i, chat := range chats {
				row := rows[i].(map[string]interface{})
				if row["chat_jid"] != chat || row["last_updated"] == "" {
					t.Fatalf("unexpected chat summary at %d: %v", i, row)
				}
			}
			if user == "index-a" && rows[0].(map[string]interface{})["last_updated"] != stamp.Add(time.Minute).Format(time.RFC3339Nano) {
				t.Fatalf("index must use the latest timestamp from this user only: %v", rows[0])
			}
		}
		for _, tc := range []struct {
			user  string
			chats []string
		}{
			{"index-a", []string{"123@lid", "456@s.whatsapp.net"}},
			{"index-b", []string{"123@lid", "789@g.us"}},
			{"index-empty", nil},
		} {
			t.Run(tc.user, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/chat/history?chat_jid=index", nil)
				req.Header.Set("token", tc.user+"-token")
				rec := httptest.NewRecorder()
				s.router.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("index returned %d: %s", rec.Code, rec.Body.String())
				}
				var envelope struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				check(t, envelope.Data, tc.user, tc.chats)

				rpc := executeRequest(t, s, newRequest("index", "chat.history", map[string]interface{}{
					"token": tc.user + "-token", "chat_jid": "index",
				}).toJSON(t))
				data := assertJSONRPC20Success(t, rpc, "index").(map[string]interface{})
				check(t, data, tc.user, tc.chats)
			})
		}
	})
}
