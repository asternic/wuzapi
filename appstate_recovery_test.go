package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

type fakeAppStateClient struct {
	mu        sync.Mutex
	fullSyncs []appstate.WAPatchName
	requests  []string
	fetchErr  error
	sendErr   error
}

func (f *fakeAppStateClient) FetchAppState(_ context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !fullSync || onlyIfNotSynced {
		return errors.New("recovery must force a full sync")
	}
	f.fullSyncs = append(f.fullSyncs, name)
	return f.fetchErr
}

func (f *fakeAppStateClient) SendPeerMessage(_ context.Context, message *waE2E.Message) (whatsmeow.SendResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, message.GetProtocolMessage().GetPeerDataOperationRequestMessage().GetSyncdCollectionFatalRecoveryRequest().GetCollectionName())
	return whatsmeow.SendResponse{ID: "REQ"}, f.sendErr
}

func (f *fakeAppStateClient) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.fullSyncs), len(f.requests)
}

func withAppStateRecovery(t *testing.T, enabled bool) {
	t.Helper()
	previous := *appStateRecoveryEnabled
	*appStateRecoveryEnabled = enabled
	t.Cleanup(func() { *appStateRecoveryEnabled = previous })
}

func syncError(name appstate.WAPatchName, fullSync bool) *events.AppStateSyncError {
	return &events.AppStateSyncError{Name: name, FullSync: fullSync, Error: errors.New("failed to verify patch v39: mismatching LTHash")}
}

func TestAppStateRecoveryIncrementalErrorRunsFullSync(t *testing.T) {
	withAppStateRecovery(t, true)
	client := &fakeAppStateClient{}
	r := newAppStateRecovery("user", client)
	now := time.Unix(1700000000, 0)

	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, false), now)
	if full, req := client.counts(); full != 1 || req != 0 || client.fullSyncs[0] != appstate.WAPatchRegularLow {
		t.Fatalf("expected one full sync of regular_low, got full=%v requests=%v", client.fullSyncs, client.requests)
	}
	// Repeats inside the cooldown do nothing; after it, the full sync runs again.
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, false), now.Add(appStateFullSyncCooldown-time.Second))
	if full, _ := client.counts(); full != 1 {
		t.Fatalf("full sync repeated inside the cooldown: %d", full)
	}
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, false), now.Add(appStateFullSyncCooldown))
	if full, _ := client.counts(); full != 2 {
		t.Fatalf("full sync not retried after the cooldown: %d", full)
	}
}

func TestAppStateRecoveryFullSyncErrorAsksPrimaryDevice(t *testing.T) {
	withAppStateRecovery(t, true)
	client := &fakeAppStateClient{}
	r := newAppStateRecovery("user", client)
	now := time.Unix(1700000000, 0)

	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, true), now)
	if full, req := client.counts(); full != 0 || req != 1 || client.requests[0] != string(appstate.WAPatchRegularLow) {
		t.Fatalf("expected one recovery request for regular_low, got full=%v requests=%v", client.fullSyncs, client.requests)
	}
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, true), now.Add(appStateRecoveryCooldown-time.Second))
	if _, req := client.counts(); req != 1 {
		t.Fatalf("recovery request repeated inside the cooldown: %d", req)
	}
	// Other collections have their own cooldown.
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularHigh, true), now)
	if _, req := client.counts(); req != 2 {
		t.Fatalf("a different collection should not share the cooldown: %d", req)
	}
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, true), now.Add(appStateRecoveryCooldown))
	if _, req := client.counts(); req != 3 {
		t.Fatalf("recovery request not retried after the cooldown: %d", req)
	}
}

func TestAppStateRecoveryCompletionResetsCooldowns(t *testing.T) {
	withAppStateRecovery(t, true)
	client := &fakeAppStateClient{}
	r := newAppStateRecovery("user", client)
	now := time.Unix(1700000000, 0)

	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, false), now)
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, true), now)
	// An ordinary full sync completion does not reset anything.
	r.completed(&events.AppStateSyncComplete{Name: appstate.WAPatchRegularLow, Version: 200})
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, true), now.Add(time.Minute))
	if full, req := client.counts(); full != 1 || req != 1 {
		t.Fatalf("non-recovery completion reset the cooldown: full=%d requests=%d", full, req)
	}
	r.completed(&events.AppStateSyncComplete{Name: appstate.WAPatchRegularLow, Version: 216, Recovery: true})
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, false), now.Add(time.Minute))
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, true), now.Add(time.Minute))
	if full, req := client.counts(); full != 2 || req != 2 {
		t.Fatalf("recovery completion should reset both cooldowns: full=%d requests=%d", full, req)
	}
}

func TestAppStateRecoveryFailedRequestCanRetry(t *testing.T) {
	withAppStateRecovery(t, true)
	client := &fakeAppStateClient{sendErr: errors.New("not connected")}
	r := newAppStateRecovery("user", client)
	now := time.Unix(1700000000, 0)

	r.handleError(context.Background(), syncError(appstate.WAPatchRegular, true), now)
	r.handleError(context.Background(), syncError(appstate.WAPatchRegular, true), now.Add(time.Second))
	if _, req := client.counts(); req != 2 {
		t.Fatalf("a request that failed to send should not start the cooldown: %d", req)
	}
}

func TestAppStateRecoveryDisabled(t *testing.T) {
	withAppStateRecovery(t, false)
	client := &fakeAppStateClient{}
	r := newAppStateRecovery("user", client)
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, false), time.Now())
	r.handleError(context.Background(), syncError(appstate.WAPatchRegularLow, true), time.Now())
	if full, req := client.counts(); full != 0 || req != 0 {
		t.Fatalf("disabled recovery acted: full=%d requests=%d", full, req)
	}
}
