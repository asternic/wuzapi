package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
)

func TestSessionQRNormalClosure(t *testing.T) {
	qr := make(chan whatsmeow.QRChannelItem, 2)
	qr <- whatsmeow.QRChannelItem{Event: "code", Code: "test-code"}
	qr <- whatsmeow.QRChannelSuccess
	close(qr)
	for _, want := range []string{"code", "success"} {
		evt, ok := receiveSessionQR(context.Background(), qr)
		if !ok || evt.Event != want {
			t.Fatalf("event=%q ok=%v, want %q", evt.Event, ok, want)
		}
	}
	if _, ok := receiveSessionQR(context.Background(), qr); ok {
		t.Fatal("closed QR channel reported an event")
	}
}

func TestSessionQRCanceledBeforeReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	qr := make(chan whatsmeow.QRChannelItem, 1)
	qr <- whatsmeow.QRChannelSuccess
	if _, ok := receiveSessionQR(ctx, qr); ok {
		t.Fatal("processed a buffered QR event after cancellation")
	}
}

func TestFullDeletionCancelsOpenQRChannel(t *testing.T) {
	s := makeTestServer(t)
	const userID = "delete-open-qr"
	insertDeviceTestUser(t, s, userID, "", "")
	kill := make(chan bool, 1)
	setKillChannel(userID, kill)
	t.Cleanup(func() { deleteKillChannel(userID, kill) })
	otherKill := make(chan bool, 1)
	setKillChannel("qr-other-session", otherKill)
	t.Cleanup(func() { deleteKillChannel("qr-other-session", otherKill) })
	ctx, cancel := sessionContext(kill)
	defer cancel()
	qr := make(chan whatsmeow.QRChannelItem)
	ownerDone := make(chan struct{})
	go func() {
		defer close(ownerDone)
		defer deleteKillChannel(userID, kill)
		for {
			if _, ok := receiveSessionQR(ctx, qr); !ok {
				return
			}
		}
	}()
	// Synchronize with the owner consuming QR events before requesting deletion.
	select {
	case qr <- whatsmeow.QRChannelItem{Event: "code", Code: "test-code"}:
	case <-time.After(2 * time.Second):
		t.Fatal("session owner did not start consuming QR events")
	}
	// Leave qr open, as Whatsmeow can do after an expected disconnect.
	response := httptest.NewRecorder()
	req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"id": userID})
	s.DeleteUserComplete()(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	select {
	case <-ownerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("QR session owner did not exit")
	}
	if _, running := getKillChannel(userID); running {
		t.Fatal("deleted session still registered")
	}
	var count int
	if err := s.db.Get(&count, `SELECT COUNT(*) FROM users WHERE id=$1`, userID); err != nil || count != 0 {
		t.Fatalf("user retained: count=%d err=%v", count, err)
	}
	if ch, running := getKillChannel("qr-other-session"); !running || ch != otherKill {
		t.Fatal("another session's registration changed")
	}
	select {
	case <-otherKill:
		t.Fatal("another session was signaled")
	default:
	}
}

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
