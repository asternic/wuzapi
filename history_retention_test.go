package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

func TestHistoryRetentionAndDeletion(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if db.DriverName() == "sqlite" {
			db.SetMaxOpenConns(1)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db, exPath: t.TempDir()}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('retention','Test','retention-token',0),('other','Other','other-token',10)"); err != nil {
			t.Fatal(err)
		}
		save := func(user, chat, id string) error {
			return s.saveMessageToHistory(user, chat, "sender", id, "text", "hello", "", "", "{}")
		}
		count := func(user string) int {
			t.Helper()
			var n int
			if err := db.Get(&n, db.Rebind("SELECT COUNT(*) FROM message_history WHERE user_id=?"), user); err != nil {
				t.Fatal(err)
			}
			return n
		}
		if err := save("retention", "chat", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := save("missing", "chat", "orphan"); err != nil {
			t.Fatal(err)
		}
		if count("retention") != 0 || count("missing") != 0 {
			t.Fatal("disabled or missing user retained history")
		}
		if _, err := db.Exec("UPDATE users SET history=2 WHERE id='retention'"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 5; i++ {
			if err := save("retention", "chat", fmt.Sprint(i)); err != nil {
				t.Fatal(err)
			}
		}
		if err := save("retention", "another-chat", "separate"); err != nil {
			t.Fatal(err)
		}
		if count("retention") != 3 {
			t.Fatal("per-chat retention limit not enforced")
		}
		if _, err := db.Exec("UPDATE users SET history=0 WHERE id='retention'"); err != nil {
			t.Fatal(err)
		}
		if err := save("retention", "chat", "disabled-mid-batch"); err != nil {
			t.Fatal(err)
		}
		if count("retention") != 3 {
			t.Fatal("current disabled setting was ignored")
		}
		if _, err := db.Exec("UPDATE users SET history=2 WHERE id='retention'"); err != nil {
			t.Fatal(err)
		}

		if err := save("other", "chat", "other-message"); err != nil {
			t.Fatal(err)
		}

		// Race a batch of late writes against the actual full-deletion endpoint.
		start := make(chan struct{})
		errs := make(chan error, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); <-start; errs <- save("retention", "chat", fmt.Sprintf("late-%d", i)) }(i)
		}
		close(start)
		req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/admin/users/retention/full", nil), map[string]string{"id": "retention"})
		response := httptest.NewRecorder()
		s.DeleteUserComplete()(response, req)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if response.Code != http.StatusOK {
			t.Fatalf("delete: %d %s", response.Code, response.Body.String())
		}
		if count("retention") != 0 {
			t.Fatal("history survived deletion or was recreated by late writes")
		}
		if count("other") != 1 {
			t.Fatal("another user's history was removed")
		}
		if err := save("retention", "chat", "after-deletion"); err != nil {
			t.Fatal(err)
		}
		if count("retention") != 0 {
			t.Fatal("deleted user accepted history")
		}
	})
}

func TestHistoryDeletionRollsBackOnCleanupFailure(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('rollback','Test','rollback-token',10)"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("DROP TABLE message_history"); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		if err := s.deleteUserAndHistory("rollback"); err == nil {
			t.Fatal("expected cleanup failure")
		}
		var n int
		if err := db.Get(&n, "SELECT COUNT(*) FROM users WHERE id='rollback'"); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatal("user deletion committed despite history cleanup failure")
		}
	})
}
