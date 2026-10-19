package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestSessionConnectRetry(t *testing.T) {
	failure := errors.New("temporary connection failure")
	for _, successAt := range []int{1, 2, 3, 0} {
		t.Run(map[int]string{1: "first attempt", 2: "second attempt", 3: "last attempt", 0: "exhausted"}[successAt], func(t *testing.T) {
			calls := 0
			err := connectSessionWithRetry(context.Background(), func(context.Context) error {
				calls++
				if calls == successAt {
					return nil
				}
				return failure
			}, 3, 0)
			if successAt == 0 {
				if !errors.Is(err, failure) || calls != 3 {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			} else if err != nil || calls != successAt {
				t.Fatalf("successful retry treated as failure: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestSessionConnectRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := connectSessionWithRetry(ctx, func(context.Context) error {
		calls++
		cancel()
		return errors.New("dial interrupted")
	}, 3, time.Hour)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestSessionCleanupPreservesReconnectState(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, connected INTEGER, qrcode TEXT, events TEXT);
 INSERT INTO users VALUES ('session', 1, 'old QR', 'Message')`); err != nil {
		t.Fatal(err)
	}
	s := &server{db: db}
	check := func(want int) {
		t.Helper()
		var connected int
		var qr, events string
		if err := db.QueryRow(`SELECT connected,qrcode,events FROM users WHERE id='session'`).Scan(&connected, &qr, &events); err != nil {
			t.Fatal(err)
		}
		if connected != want || qr != "" || events != "Message" {
			t.Fatalf("connected=%d qr=%q events=%q", connected, qr, events)
		}
	}
	// Failed startup connection must remain eligible for the next restart.
	if err := s.clearSessionQRCode("session"); err != nil {
		t.Fatal(err)
	}
	check(1)
	// Explicit disconnect/logout disables reconnect, and cleanup must preserve it.
	if err := s.setDisconnectedState("session", false); err != nil {
		t.Fatal(err)
	}
	if err := s.clearSessionQRCode("session"); err != nil {
		t.Fatal(err)
	}
	check(0)
}
