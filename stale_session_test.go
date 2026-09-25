package main

import (
	"sync"
	"testing"

	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow"
)

// The scenario behind every test in this file: two /session/connect calls race
// for the same user. Both pass the "already connected" guard (nothing is
// connected yet), so two startClient goroutines run, each with its own device
// and its own QR channel. The user scans one of them. Minutes later the other
// QR expires unscanned and its goroutine cleans up — and that cleanup used to
// address the shared state by bare userID, so it evicted the session that had
// just paired.

// TestDeleteSessionIfCurrentStaleSession is the clientManager counterpart of
// TestDeleteKillChannelStaleSession: a cleanup from an abandoned session must
// not evict the live session's entries.
func TestDeleteSessionIfCurrentStaleSession(t *testing.T) {
	const u = "stale-session-user"
	cm := NewClientManager()

	// Two whatsmeow clients, distinct pointers — the two racing sessions.
	stale := &whatsmeow.Client{}
	live := &whatsmeow.Client{}

	// The stale session registers first; the winner then replaces the entry.
	cm.SetWhatsmeowClient(u, stale)
	cm.SetWhatsmeowClient(u, live)
	cm.SetMyClient(u, &MyClient{userID: u})
	cm.SetPollOptions(u, "msg-1", []string{"yes", "no"})

	// The abandoned session's QR expires and it cleans up with ITS client.
	if cm.DeleteSessionIfCurrent(u, stale) {
		t.Error("DeleteSessionIfCurrent reported a delete for a session that is no longer registered")
	}

	if got := cm.GetWhatsmeowClient(u); got != live {
		t.Fatalf("stale cleanup evicted the live session: GetWhatsmeowClient = %v, want the live client", got)
	}
	if cm.GetMyClient(u) == nil {
		t.Error("stale cleanup removed the live session's MyClient")
	}
	if opts := cm.GetPollOptions(u, "msg-1"); len(opts) != 2 {
		t.Errorf("stale cleanup dropped the live session's poll options: %v", opts)
	}
}

// TestDeleteSessionIfCurrentOwnSession proves the guard does not block the
// legitimate case: the session that IS registered cleans itself up fully.
func TestDeleteSessionIfCurrentOwnSession(t *testing.T) {
	const u = "own-session-user"
	cm := NewClientManager()
	client := &whatsmeow.Client{}

	cm.SetWhatsmeowClient(u, client)
	cm.SetMyClient(u, &MyClient{userID: u})
	cm.SetPollOptions(u, "msg-1", []string{"yes"})

	if !cm.DeleteSessionIfCurrent(u, client) {
		t.Fatal("DeleteSessionIfCurrent refused to clean up the session that owns the entry")
	}
	if cm.GetWhatsmeowClient(u) != nil {
		t.Error("whatsmeow client still registered after its own cleanup")
	}
	if cm.GetMyClient(u) != nil {
		t.Error("MyClient still registered after its own cleanup")
	}
	if opts := cm.GetPollOptions(u, "msg-1"); opts != nil {
		t.Errorf("poll options survived the session cleanup: %v", opts)
	}
}

// TestDeleteSessionIfCurrentUnknownUser guards the no-entry path: cleanup after
// the maps were already cleared must be a silent no-op, not a panic.
func TestDeleteSessionIfCurrentUnknownUser(t *testing.T) {
	cm := NewClientManager()
	if cm.DeleteSessionIfCurrent("never-registered", &whatsmeow.Client{}) {
		t.Error("DeleteSessionIfCurrent reported a delete for a user with no entry")
	}
}

// TestDeleteSessionIfCurrentConcurrent hammers the guard from many goroutines.
// The point is the -race build: the three maps are dropped under one lock, so
// no reader can observe a half-torn-down session.
func TestDeleteSessionIfCurrentConcurrent(t *testing.T) {
	const u = "concurrent-session-user"
	cm := NewClientManager()
	live := &whatsmeow.Client{}
	cm.SetWhatsmeowClient(u, live)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cm.DeleteSessionIfCurrent(u, &whatsmeow.Client{}) // always stale
		}()
		go func() {
			defer wg.Done()
			_ = cm.GetWhatsmeowClient(u)
		}()
	}
	wg.Wait()

	if got := cm.GetWhatsmeowClient(u); got != live {
		t.Errorf("live client lost under concurrent stale cleanups: got %v", got)
	}
}

// TestSignalKillChannelTargetsOwnChannel proves a goroutine ending itself hits
// its own channel and not whatever session is registered now. signalKill(userID)
// would resolve to the newer session and shut down the wrong one.
func TestSignalKillChannelTargetsOwnChannel(t *testing.T) {
	const u = "kill-target-user"

	stale := make(chan bool, 1)
	live := make(chan bool, 1)
	setKillChannel(u, stale)
	setKillChannel(u, live) // a reconnect replaced the entry
	defer deleteKillChannel(u, live)

	// The abandoned goroutine ends ITSELF with the channel it captured.
	signalKillChannel(stale)

	select {
	case <-stale:
	default:
		t.Error("signalKillChannel did not deliver to the caller's own channel")
	}
	select {
	case <-live:
		t.Error("signalKillChannel killed the live session registered for this user")
	default:
	}
}

// TestSetUserInfoFieldKeepsConcurrentUpdates is the cache half of the bug.
// The QR loop held a Values captured before pairing (Jid empty) and rewrote the
// cache from it on every new QR code, so a code emitted after PairSuccess
// restored the empty Jid. The next /session/connect then found no JID, built a
// fresh device and asked for a new QR, while the paired device sat intact in
// the store.
func TestSetUserInfoFieldKeepsConcurrentUpdates(t *testing.T) {
	const token = "cache-token"
	userinfocache.Set(token, Values{m: map[string]string{"Jid": "", "Id": "u1"}}, cache.NoExpiration)
	defer userinfocache.Delete(token)

	// PairSuccess writes the JID.
	if !setUserInfoField(token, "Jid", "5511999999999:1@s.whatsapp.net") {
		t.Fatal("setUserInfoField reported a missing entry for a token that is cached")
	}

	// A QR code emitted after pairing updates an unrelated field.
	setUserInfoField(token, "Qrcode", "data:image/png;base64,AAAA")

	v, found := userinfocache.Get(token)
	if !found {
		t.Fatal("user info vanished from the cache")
	}
	got := v.(Values)
	if jid := got.Get("Jid"); jid != "5511999999999:1@s.whatsapp.net" {
		t.Errorf("Jid was reverted by a later field update: got %q", jid)
	}
	if qr := got.Get("Qrcode"); qr != "data:image/png;base64,AAAA" {
		t.Errorf("Qrcode not written: got %q", qr)
	}
	if id := got.Get("Id"); id != "u1" {
		t.Errorf("unrelated field lost: Id=%q", id)
	}
}

// TestSetUserInfoFieldMissingEntry documents the return value: no cached entry
// means nothing was written, and callers use that to skip their log line.
func TestSetUserInfoFieldMissingEntry(t *testing.T) {
	if setUserInfoField("token-never-cached", "Qrcode", "x") {
		t.Error("setUserInfoField reported a write for a token that is not cached")
	}
}

// TestStaleSnapshotRevertsFields pins down WHY setUserInfoField exists, by
// contrasting it with the pattern it replaced. Writing through a Values that
// was captured earlier reverts every field changed since — reading immediately
// before writing does not. Without this contrast the helper looks like a
// pointless wrapper around updateUserInfo.
func TestStaleSnapshotRevertsFields(t *testing.T) {
	const token = "snapshot-token"
	const jid = "5511999999999:1@s.whatsapp.net"
	fresh := func() { userinfocache.Set(token, Values{m: map[string]string{"Jid": ""}}, cache.NoExpiration) }
	defer userinfocache.Delete(token)

	// The pattern that caused the bug: capture once, write from it later.
	fresh()
	snapshot, _ := userinfocache.Get(token) // Jid is still empty here
	setUserInfoField(token, "Jid", jid)     // pairing writes the JID
	userinfocache.Set(token, updateUserInfo(snapshot, "Qrcode", "x"), cache.NoExpiration)

	v, _ := userinfocache.Get(token)
	if got := v.(Values).Get("Jid"); got != "" {
		t.Fatalf("test is not reproducing the stale-snapshot write: Jid=%q, want it reverted to empty", got)
	}

	// Same sequence through the helper: the JID survives.
	fresh()
	setUserInfoField(token, "Jid", jid)
	setUserInfoField(token, "Qrcode", "x")

	v, _ = userinfocache.Get(token)
	if got := v.(Values).Get("Jid"); got != jid {
		t.Errorf("setUserInfoField reverted the JID: got %q, want %q", got, jid)
	}
}
