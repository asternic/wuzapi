package main

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

// A failed incremental sync gets one full resync per hour. When a full sync
// fails too, the primary phone is asked for a fresh copy of the collection at
// most every six hours: it only answers while WhatsApp is open, and whatsmeow
// retries the full sync (and fails again) on every app state update until then.
const (
	appStateFullSyncCooldown = time.Hour
	appStateRecoveryCooldown = 6 * time.Hour
	appStateRecoveryTimeout  = 2 * time.Minute
)

// Kept small so recovery can be exercised without a live WhatsApp connection.
type appStateRecoveryClient interface {
	FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error
	SendPeerMessage(ctx context.Context, message *waE2E.Message) (whatsmeow.SendResponse, error)
}

type appStateRecoveryAction int

const (
	appStateRecoveryNone appStateRecoveryAction = iota
	appStateRecoveryFullSync
	appStateRecoveryPeerRequest
)

// appStateRecovery repairs app state collections that fail verification
// (for example "mismatching LTHash"). Without it the collection stays broken
// and every write to it (unread, mute, archive, pin, star, labels) fails with
// a 409 conflict.
type appStateRecovery struct {
	userID       string
	client       appStateRecoveryClient
	mu           sync.Mutex
	lastFullSync map[appstate.WAPatchName]time.Time
	lastRequest  map[appstate.WAPatchName]time.Time
}

func newAppStateRecovery(userID string, client appStateRecoveryClient) *appStateRecovery {
	return &appStateRecovery{
		userID:       userID,
		client:       client,
		lastFullSync: map[appstate.WAPatchName]time.Time{},
		lastRequest:  map[appstate.WAPatchName]time.Time{},
	}
}

// plan picks the next step for a sync error and records the attempt, so
// concurrent errors for the same collection trigger it only once.
func (r *appStateRecovery) plan(evt *events.AppStateSyncError, now time.Time) appStateRecoveryAction {
	if !*appStateRecoveryEnabled {
		return appStateRecoveryNone
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !evt.FullSync {
		if last, ok := r.lastFullSync[evt.Name]; ok && now.Sub(last) < appStateFullSyncCooldown {
			return appStateRecoveryNone
		}
		r.lastFullSync[evt.Name] = now
		return appStateRecoveryFullSync
	}
	if last, ok := r.lastRequest[evt.Name]; ok && now.Sub(last) < appStateRecoveryCooldown {
		return appStateRecoveryNone
	}
	r.lastRequest[evt.Name] = now
	return appStateRecoveryPeerRequest
}

// handleError runs outside the event handler: whatsmeow dispatches sync errors
// while holding its app state lock, so a synchronous FetchAppState would deadlock.
func (r *appStateRecovery) handleError(ctx context.Context, evt *events.AppStateSyncError, now time.Time) {
	action := r.plan(evt, now)
	logEvent := log.Warn().Str("userID", r.userID).Str("collection", string(evt.Name)).Bool("fullSync", evt.FullSync).Err(evt.Error)
	switch action {
	case appStateRecoveryNone:
		logEvent.Bool("recoveryEnabled", *appStateRecoveryEnabled).Msg("App state sync failed; recovery skipped (disabled or cooling down)")
		return
	case appStateRecoveryFullSync:
		logEvent.Msg("App state sync failed; starting a full resync")
	case appStateRecoveryPeerRequest:
		logEvent.Msg("App state full sync failed; asking the primary device for a fresh copy")
	}

	ctx, cancel := context.WithTimeout(ctx, appStateRecoveryTimeout)
	defer cancel()
	switch action {
	case appStateRecoveryFullSync:
		// A failure here dispatches a FullSync error, which leads to the peer request.
		if err := r.client.FetchAppState(ctx, evt.Name, true, false); err != nil {
			log.Warn().Str("userID", r.userID).Str("collection", string(evt.Name)).Err(err).Msg("App state full resync failed")
			return
		}
		log.Info().Str("userID", r.userID).Str("collection", string(evt.Name)).Msg("App state full resync completed")
	case appStateRecoveryPeerRequest:
		resp, err := r.client.SendPeerMessage(ctx, whatsmeow.BuildAppStateRecoveryRequest(evt.Name))
		if err != nil {
			log.Warn().Str("userID", r.userID).Str("collection", string(evt.Name)).Err(err).Msg("Failed to send app state recovery request")
			// Allow another attempt on the next error instead of waiting out the cooldown.
			r.mu.Lock()
			delete(r.lastRequest, evt.Name)
			r.mu.Unlock()
			return
		}
		// whatsmeow applies the phone's answer itself and emits AppStateSyncComplete with Recovery set.
		log.Info().Str("userID", r.userID).Str("collection", string(evt.Name)).Str("requestID", resp.ID).Msg("App state recovery requested; waiting for the primary device")
	}
}

// completed resets the cooldowns once the phone's copy has been applied, so a
// later failure of the same collection is handled right away.
func (r *appStateRecovery) completed(evt *events.AppStateSyncComplete) {
	if !evt.Recovery {
		return
	}
	r.mu.Lock()
	delete(r.lastFullSync, evt.Name)
	delete(r.lastRequest, evt.Name)
	r.mu.Unlock()
	log.Info().Str("userID", r.userID).Str("collection", string(evt.Name)).Uint64("version", evt.Version).Msg("App state recovered from the primary device")
}
