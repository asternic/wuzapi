package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/socket"
	"go.mau.fi/whatsmeow/types"
)

type recoveryTestClient struct {
	connected atomic.Bool
	connects  atomic.Int32
	sends     atomic.Int32
	presences atomic.Int32
	connect   func(context.Context) error
	sendErr   error
}

func (c *recoveryTestClient) IsConnected() bool { return c.connected.Load() }
func (c *recoveryTestClient) ConnectContext(ctx context.Context) error {
	c.connects.Add(1)
	if c.connect != nil {
		return c.connect(ctx)
	}
	return nil
}
func (c *recoveryTestClient) SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	c.sends.Add(1)
	return whatsmeow.SendResponse{ID: "original-id"}, c.sendErr
}
func (c *recoveryTestClient) SendChatPresence(context.Context, types.JID, types.ChatPresence, types.ChatPresenceMedia) error {
	c.presences.Add(1)
	return c.sendErr
}

func waitRecovery(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for recovery")
		}
		time.Sleep(time.Millisecond)
	}
}
func recoveryIdle(r *socketRecovery) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.pending
}
func runRecoveryTest(t *testing.T, r *socketRecovery, grace time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); r.run(t.Name(), grace) }()
	t.Cleanup(func() {
		r.stop()
		r.cancelSession()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("recovery goroutine did not exit")
		}
	})
}

func TestSocketRecoveryErrorFilter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newSocketRecovery(ctx, cancel, &recoveryTestClient{})
	for _, err := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.New("frame socket is closed"), errors.New("timeout")} {
		if r.request(err, time.Now()) {
			t.Fatalf("unexpected recovery for %v", err)
		}
	}
	if !r.request(fmt.Errorf("usync: %w", socket.ErrSocketClosed), time.Now()) {
		t.Fatal("wrapped closed socket error did not trigger recovery")
	}
}

func TestSocketRecoveryCoalescingAndCooldown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &recoveryTestClient{}
	r := newSocketRecovery(ctx, cancel, c)
	now := time.Now()
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r.request(socket.ErrSocketClosed, now) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d recoveries, want 1", accepted.Load())
	}
	runRecoveryTest(t, r, 0)
	waitRecovery(t, func() bool { return recoveryIdle(r) })
	if c.connects.Load() != 1 {
		t.Fatalf("got %d connection attempts", c.connects.Load())
	}
	if r.request(socket.ErrSocketClosed, now.Add(socketRecoveryCooldown-time.Nanosecond)) {
		t.Fatal("cooldown allowed a second attempt")
	}
	if !r.request(socket.ErrSocketClosed, now.Add(socketRecoveryCooldown)) {
		t.Fatal("cooldown never expired")
	}
	waitRecovery(t, func() bool { return recoveryIdle(r) })
	if c.connects.Load() != 2 {
		t.Fatalf("got %d connection attempts after cooldown", c.connects.Load())
	}
}

func TestSocketRecoverySkipsRestoredConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &recoveryTestClient{}
	r := newSocketRecovery(ctx, cancel, c)
	r.request(socket.ErrSocketClosed, time.Now())
	c.connected.Store(true) // Normal auto-reconnect won before recovery runs.
	runRecoveryTest(t, r, 0)
	waitRecovery(t, func() bool { return recoveryIdle(r) })
	if c.connects.Load() != 0 {
		t.Fatal("recovery touched an already restored connection")
	}
}

func TestSocketRecoveryStoppedDuringGrace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &recoveryTestClient{}
	r := newSocketRecovery(ctx, cancel, c)
	r.request(socket.ErrSocketClosed, time.Now())
	runRecoveryTest(t, r, time.Hour)
	r.stop()
	r.reconnect(t.Name())
	if c.connects.Load() != 0 {
		t.Fatal("stopped session reconnected")
	}
	if r.request(socket.ErrSocketClosed, time.Now().Add(time.Hour)) {
		t.Fatal("stopped session accepted recovery")
	}
}

func TestSocketRecoveryManualShutdownCancelsDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	exited := make(chan struct{})
	c := &recoveryTestClient{connect: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	}}
	r := newSocketRecovery(ctx, cancel, c)
	cm := NewClientManager()
	cm.socketRecoveries["user"] = r
	runRecoveryTest(t, r, 0)
	r.request(socket.ErrSocketClosed, time.Now())
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("dial not started")
	}
	cm.stopSocketRecovery("user", true)
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("manual shutdown did not cancel dial")
	}
	if r.active() {
		t.Fatal("session still active after shutdown")
	}
}

func TestSocketRecoveryLogoutWaitsForDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	c := &recoveryTestClient{connect: func(ctx context.Context) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	r := newSocketRecovery(ctx, cancel, c)
	cm := NewClientManager()
	cm.socketRecoveries["user"] = r
	runRecoveryTest(t, r, 0)
	r.request(socket.ErrSocketClosed, time.Now())
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("dial not started")
	}
	stopped := make(chan struct{})
	go func() { cm.stopSocketRecovery("user", false); close(stopped) }()
	select {
	case <-r.stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("logout did not stop recovery")
	}
	select {
	case <-stopped:
		t.Fatal("logout raced with ongoing dial")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("logout did not resume")
	}
	if ctx.Err() != nil {
		t.Fatal("logout canceled the connection before sending logout")
	}
}

func TestSocketRecoveryRegistryIdentityAndOptOut(t *testing.T) {
	enabled := *socketRecoveryEnabled
	defer func() { *socketRecoveryEnabled = enabled }()
	*socketRecoveryEnabled = true
	cm := NewClientManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldClient, newClient := &whatsmeow.Client{}, &whatsmeow.Client{}
	cm.SetWhatsmeowClient("user", newClient)
	oldRecovery := newSocketRecovery(ctx, cancel, oldClient)
	if cm.registerSocketRecovery("user", oldRecovery) {
		t.Fatal("stale client registered recovery")
	}
	r := newSocketRecovery(ctx, cancel, newClient)
	if !cm.registerSocketRecovery("user", r) {
		t.Fatal("current client could not register")
	}
	cm.requestSocketRecovery("user", oldClient, socket.ErrSocketClosed)
	if !recoveryIdle(r) {
		t.Fatal("stale send triggered recovery of replacement client")
	}
	*socketRecoveryEnabled = false
	cm.requestSocketRecovery("user", newClient, socket.ErrSocketClosed)
	if !recoveryIdle(r) {
		t.Fatal("disabled recovery queued work")
	}
	*socketRecoveryEnabled = true
	cm.requestSocketRecovery("user", newClient, socket.ErrSocketClosed)
	if recoveryIdle(r) {
		t.Fatal("current client did not queue recovery")
	}
	cm.removeSocketRecovery("user", oldRecovery)
	if cm.socketRecoveries["user"] != r {
		t.Fatal("stale cleanup removed current recovery")
	}
	cm.DeleteWhatsmeowClient("user")
	if r.active() {
		t.Fatal("deleting client did not stop recovery")
	}
	cm.removeSocketRecovery("user", r)
	if len(cm.socketRecoveries) != 0 {
		t.Fatal("recovery state leaked")
	}
}

func TestSocketRecoveryPreservesSendResultsWithoutRetry(t *testing.T) {
	original := fmt.Errorf("failed usync: %w", socket.ErrSocketClosed)
	c := &recoveryTestClient{sendErr: original}
	response, err := sendMessageWithRecovery(t.Name(), c, context.Background(), types.JID{}, &waE2E.Message{})
	if err != original || response.ID != "original-id" {
		t.Fatal("original send result changed")
	}
	if c.sends.Load() != 1 {
		t.Fatal("failed message was replayed")
	}
	if err := sendChatPresenceWithRecovery(t.Name(), c, context.Background(), types.JID{}, types.ChatPresenceComposing, types.ChatPresenceMediaText); err != original {
		t.Fatal("original presence error changed")
	}
	if c.presences.Load() != 1 {
		t.Fatal("presence was replayed")
	}
}

func TestSocketRecoveryFailedAttemptCanBeRetriedLater(t *testing.T) {
	for _, connectErr := range []error{errors.New("dial failed"), whatsmeow.ErrAlreadyConnected} {
		t.Run(connectErr.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := &recoveryTestClient{connect: func(context.Context) error { return connectErr }}
			r := newSocketRecovery(ctx, cancel, c)
			runRecoveryTest(t, r, 0)
			now := time.Now()
			r.request(socket.ErrSocketClosed, now)
			waitRecovery(t, func() bool { return recoveryIdle(r) })
			if !r.request(socket.ErrSocketClosed, now.Add(socketRecoveryCooldown)) {
				t.Fatal("failed attempt left recovery blocked")
			}
			waitRecovery(t, func() bool { return recoveryIdle(r) })
			if c.connects.Load() != 2 {
				t.Fatal("missing second connection attempt")
			}
		})
	}
}

func TestSocketRecoveryIndependentSessions(t *testing.T) {
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	started := make(chan struct{})
	a := &recoveryTestClient{connect: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	b := &recoveryTestClient{}
	ra, rb := newSocketRecovery(ctxA, cancelA, a), newSocketRecovery(ctxB, cancelB, b)
	runRecoveryTest(t, ra, 0)
	runRecoveryTest(t, rb, 0)
	ra.request(socket.ErrSocketClosed, time.Now())
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first session did not start recovery")
	}
	rb.request(socket.ErrSocketClosed, time.Now())
	waitRecovery(t, func() bool { return recoveryIdle(rb) })
	if b.connects.Load() != 1 {
		t.Fatal("slow session prevented another session's recovery")
	}
}

func TestSocketRecoveryReplacementStopsOldSession(t *testing.T) {
	cm := NewClientManager()
	oldClient, newClient := &whatsmeow.Client{}, &whatsmeow.Client{}
	oldCtx, cancelOld := context.WithCancel(context.Background())
	defer cancelOld()
	cm.SetWhatsmeowClient("user", oldClient)
	old := newSocketRecovery(oldCtx, cancelOld, oldClient)
	cm.registerSocketRecovery("user", old)
	old.request(socket.ErrSocketClosed, time.Now())
	cm.SetWhatsmeowClient("user", newClient)
	if old.active() || oldCtx.Err() == nil {
		t.Fatal("replaced session retained active recovery")
	}
	newCtx, cancelNew := context.WithCancel(context.Background())
	defer cancelNew()
	current := newSocketRecovery(newCtx, cancelNew, newClient)
	if !cm.registerSocketRecovery("user", current) {
		t.Fatal("replacement could not register recovery")
	}
	cm.removeSocketRecovery("user", old)
	if cm.socketRecoveries["user"] != current || !current.active() {
		t.Fatal("old cleanup affected replacement")
	}
}

func TestSocketRecoverySignalKillStopsQueuedRecovery(t *testing.T) {
	userID := t.Name()
	client := &whatsmeow.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kill := make(chan bool, 1)
	setKillChannel(userID, kill)
	clientManager.SetWhatsmeowClient(userID, client)
	r := newSocketRecovery(ctx, cancel, client)
	if !clientManager.registerSocketRecovery(userID, r) {
		t.Fatal("could not register session")
	}
	t.Cleanup(func() {
		clientManager.removeSocketRecovery(userID, r)
		clientManager.DeleteWhatsmeowClient(userID)
		deleteKillChannel(userID, kill)
	})
	r.request(socket.ErrSocketClosed, time.Now())
	signalKill(userID)
	if ctx.Err() == nil || r.active() {
		t.Fatal("signalKill left recovery active")
	}
	select {
	case <-kill:
	default:
		t.Fatal("session kill signal was lost")
	}
}
