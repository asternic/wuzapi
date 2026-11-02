package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestHistoryTrimInterval(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if db.DriverName() == "sqlite" {
			db.SetMaxOpenConns(1)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		historyTrimInterval = 3
		t.Cleanup(func() { historyTrimInterval = 1 })
		s := &server{db: db, exPath: t.TempDir()}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('interval','Test','interval-token',1)"); err != nil {
			t.Fatal(err)
		}
		// Only the third save trims, back down to the limit of 1.
		for i, want := range []int{1, 2, 1} {
			if err := s.saveMessageToHistory("interval", "chat", "sender", fmt.Sprint(i), "text", "hello", "", "", "{}", time.Time{}); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := db.Get(&n, "SELECT COUNT(*) FROM message_history"); err != nil {
				t.Fatal(err)
			}
			if n != want {
				t.Fatalf("after save %d: %d rows, want %d", i+1, n, want)
			}
		}
	})
}

func TestHistoryTrimIntervalIsPerUserAndChat(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if db.DriverName() == "sqlite" {
			db.SetMaxOpenConns(1)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		historyTrimInterval = 3
		t.Cleanup(func() { historyTrimInterval = 1 })
		s := &server{db: db}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('first','Test','first-token',1), ('second','Test','second-token',1)"); err != nil {
			t.Fatal(err)
		}
		chats := []struct{ user, chat string }{
			{"first", "chat-a"}, {"first", "chat-b"}, {"second", "chat-a"},
		}
		// Interleave users and chats so a global counter would always trim only
		// the third entry. Every chat must independently trim on its third save.
		for i, want := range []int{1, 2, 1, 2, 3, 1} {
			for _, c := range chats {
				if err := s.saveMessageToHistory(c.user, c.chat, "sender", fmt.Sprintf("%s-%d", c.chat, i), "text", "hello", "", "", "{}", time.Time{}); err != nil {
					t.Fatal(err)
				}
				var n int
				if err := db.Get(&n, db.Rebind("SELECT COUNT(*) FROM message_history WHERE user_id=? AND chat_jid=?"), c.user, c.chat); err != nil {
					t.Fatal(err)
				}
				if n != want {
					t.Fatalf("user %s chat %s after save %d: %d rows, want %d", c.user, c.chat, i+1, n, want)
				}
			}
		}
	})
}

func TestHistorySaveDoesNotQueueBehindOtherSaves(t *testing.T) {
	db := historyTestDB(t, "postgres")
	if err := initializeSchema(db); err != nil {
		t.Fatal(err)
	}
	s := &server{db: db, exPath: t.TempDir()}
	if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('busy','Test','busy-token',10)"); err != nil {
		t.Fatal(err)
	}
	// An open transaction holding the lock a save takes.
	other, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback()
	if _, err := other.Exec("SELECT history FROM users WHERE id='busy' FOR KEY SHARE"); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- s.saveMessageToHistory("busy", "chat", "sender", "1", "text", "hello", "", "", "{}", time.Time{})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("save blocked behind another save of the same user")
	}

	// Deleting the user still has to wait for saves in progress.
	del, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer del.Rollback()
	if _, err := del.Exec("SET LOCAL lock_timeout = '200ms'"); err != nil {
		t.Fatal(err)
	}
	if _, err := del.Exec("DELETE FROM users WHERE id='busy'"); err == nil {
		t.Fatal("user deleted while a save held its row")
	}
}

func TestParallelHistorySavesTrimWithoutDeadlock(t *testing.T) {
	for _, interval := range []uint64{1, 4} {
		t.Run(fmt.Sprintf("interval_%d", interval), func(t *testing.T) {
			historyTrimInterval = interval
			t.Cleanup(func() { historyTrimInterval = 1 })
			db := historyTestDB(t, "postgres")
			if err := initializeSchema(db); err != nil {
				t.Fatal(err)
			}
			s := &server{db: db, exPath: t.TempDir()}
			if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('parallel','Test','parallel-token',5)"); err != nil {
				t.Fatal(err)
			}
			const saves = 200
			errs := make(chan error, saves)
			var wg sync.WaitGroup
			gate := make(chan struct{}, 20)
			for i := 0; i < saves; i++ {
				wg.Add(1)
				gate <- struct{}{}
				go func(i int) {
					defer wg.Done()
					defer func() { <-gate }()
					errs <- s.saveMessageToHistory("parallel", "chat", "sender", fmt.Sprint(i), "text", "hello", "", "", "{}", time.Time{})
				}(i)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			var n int
			if err := db.Get(&n, "SELECT COUNT(*) FROM message_history"); err != nil {
				t.Fatal(err)
			}
			if n != 5 {
				t.Fatalf("%d rows left, want the limit of 5", n)
			}
		})
	}
}

// A duplicate must wait for the chat lock without locking its existing row.
// Otherwise a trim holding the chat lock can wait for that row while the
// duplicate waits for the chat, forming a deadlock.
func TestHistoryDuplicateWaitsForChatBeforeLockingMessage(t *testing.T) {
	for _, interval := range []uint64{1, 3} {
		t.Run(fmt.Sprintf("interval_%d", interval), func(t *testing.T) {
			db := historyTestDB(t, "postgres")
			if err := initializeSchema(db); err != nil {
				t.Fatal(err)
			}
			historyTrimInterval = interval
			t.Cleanup(func() { historyTrimInterval = 1 })
			s := &server{db: db}
			if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('duplicate','Test','duplicate-token',1)"); err != nil {
				t.Fatal(err)
			}
			save := func() error {
				return s.saveMessageToHistory("duplicate", "chat", "sender", "old", "text", "hello", "", "", "{}", time.Time{})
			}
			if err := save(); err != nil {
				t.Fatal(err)
			}

			trimmer, err := db.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			defer trimmer.Rollback()
			if _, err := trimmer.Exec("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "duplicate chat"); err != nil {
				t.Fatal(err)
			}
			var pid int
			if err := trimmer.Get(&pid, "SELECT pg_backend_pid()"); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- save() }()
			defer func() {
				trimmer.Rollback()
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("duplicate save: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Error("duplicate save did not finish after releasing the chat lock")
				}
			}()

			// Observe the actual lock wait rather than guessing with a sleep.
			deadline := time.Now().Add(5 * time.Second)
			for {
				var waiting bool
				if err := db.Get(&waiting, `SELECT EXISTS (
					SELECT 1 FROM pg_locks waiting JOIN pg_locks held
					USING (locktype, database, classid, objid, objsubid)
					WHERE held.pid = $1 AND held.granted AND NOT waiting.granted
					AND waiting.locktype = 'advisory'
				)`, pid); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case err := <-done:
					done <- err // Preserve the result for deferred cleanup.
					t.Fatalf("duplicate save completed before obtaining the chat lock: %v", err)
				default:
				}
				if time.Now().After(deadline) {
					t.Fatal("duplicate save did not wait for the chat lock")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if _, err := trimmer.Exec("SELECT id FROM message_history WHERE user_id='duplicate' AND message_id='old' FOR UPDATE NOWAIT"); err != nil {
				t.Fatalf("duplicate locked a message before obtaining the chat lock: %v", err)
			}
			// Exercise deletion of the row the duplicate is about to upsert.
			if _, err := trimmer.Exec("DELETE FROM message_history WHERE user_id='duplicate' AND message_id='old'"); err != nil {
				t.Fatal(err)
			}
			if err := trimmer.Commit(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
