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
}
