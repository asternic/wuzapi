package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/socket"
	"go.mau.fi/whatsmeow/types"
)

const socketRecoveryGrace = 2 * time.Second
const socketRecoveryCooldown = time.Minute

// Kept small so recovery can be exercised without a live WhatsApp connection.
type recoverableSocket interface {
	IsConnected() bool
	ConnectContext(context.Context) error
}

type socketRecovery struct {
	client        recoverableSocket
	ctx           context.Context
	cancelSession context.CancelFunc
	requests      chan struct{}
	stopped       chan struct{}
	stopOnce      sync.Once
	mu            sync.Mutex
	pending       bool
	lastAttempt   time.Time
	// Serializes recovery with an explicit logout, without locking other users.
	operation sync.Mutex
}

func newSocketRecovery(ctx context.Context, cancel context.CancelFunc, client recoverableSocket) *socketRecovery {
	return &socketRecovery{client: client, ctx: ctx, cancelSession: cancel, requests: make(chan struct{}, 1), stopped: make(chan struct{})}
}

func (r *socketRecovery) stop() { r.stopOnce.Do(func() { close(r.stopped) }) }

func (r *socketRecovery) active() bool {
	select {
	case <-r.ctx.Done():
		return false
	case <-r.stopped:
		return false
	default:
		return true
	}
}

func (r *socketRecovery) request(err error, now time.Time) bool {
	if !errors.Is(err, socket.ErrSocketClosed) {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active() || r.pending || (!r.lastAttempt.IsZero() && now.Sub(r.lastAttempt) < socketRecoveryCooldown) {
		return false
	}
	r.pending = true
	r.lastAttempt = now
	r.requests <- struct{}{}
	return true
}

// Called by the existing session owner: no polling or extra idle goroutine.
func (r *socketRecovery) run(userID string, grace time.Duration) {
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.stopped:
			return
		case <-r.requests:
			timer := time.NewTimer(grace)
			select {
			case <-r.ctx.Done():
				timer.Stop()
				return
			case <-r.stopped:
				timer.Stop()
				return
			case <-timer.C:
			}
			r.reconnect(userID)
			r.mu.Lock()
			r.pending = false
			r.mu.Unlock()
		}
	}
}

func (r *socketRecovery) reconnect(userID string) {
	r.operation.Lock()
	defer r.operation.Unlock()
	if !r.active() {
		return
	}
	if r.client.IsConnected() {
		log.Debug().Str("userID", userID).Msg("Socket recovery skipped: connection already restored")
		return
	}
	log.Warn().Str("userID", userID).Msg("Recovering session after closed socket send error")
	// ConnectContext clears a stale closed socket under whatsmeow's socket lock.
	// If normal auto-reconnect wins the race, ErrAlreadyConnected is harmless.
	// Use the session lifetime, never the failed HTTP request's context.
	err := r.client.ConnectContext(r.ctx)
	if err != nil && !errors.Is(err, whatsmeow.ErrAlreadyConnected) {
		if r.active() {
			log.Warn().Str("userID", userID).Err(err).Msg("Socket recovery connection attempt failed")
		}
		return
	}
	// A nil result may mean whatsmeow scheduled its own retry, not authentication.
	log.Info().Str("userID", userID).Bool("connected", r.client.IsConnected()).Msg("Socket recovery connection attempt completed")
}

func (cm *ClientManager) registerSocketRecovery(userID string, recovery *socketRecovery) bool {
	cm.Lock()
	defer cm.Unlock()
	if cm.whatsmeowClients[userID] != recovery.client || !recovery.active() {
		return false
	}
	cm.socketRecoveries[userID] = recovery
	return true
}

func (cm *ClientManager) removeSocketRecovery(userID string, recovery *socketRecovery) {
	recovery.stop()
	cm.Lock()
	defer cm.Unlock()
	if cm.socketRecoveries[userID] == recovery {
		delete(cm.socketRecoveries, userID)
	}
}

func (cm *ClientManager) requestSocketRecovery(userID string, client recoverableSocket, err error) {
	if !*socketRecoveryEnabled || !errors.Is(err, socket.ErrSocketClosed) {
		return
	}
	cm.RLock()
	recovery := cm.socketRecoveries[userID]
	current := cm.whatsmeowClients[userID]
	cm.RUnlock()
	if recovery != nil && current == client && recovery.client == client {
		recovery.request(err, time.Now())
	}
}

// Before logout, stop queued work and wait for any connection attempt to finish.
// signalKill instead cancels the session immediately, including an active dial.
func (cm *ClientManager) stopSocketRecovery(userID string, cancelSession bool) {
	cm.RLock()
	recovery := cm.socketRecoveries[userID]
	cm.RUnlock()
	if recovery == nil {
		return
	}
	recovery.stop()
	if cancelSession {
		recovery.cancelSession()
	} else {
		recovery.operation.Lock()
		recovery.operation.Unlock()
	}
}

type messageSender interface {
	recoverableSocket
	SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

type chatPresenceSender interface {
	recoverableSocket
	SendChatPresence(context.Context, types.JID, types.ChatPresence, types.ChatPresenceMedia) error
}

// Never replay a failed send: its delivery may be uncertain. Preserve both the
// original response and error while scheduling recovery for subsequent requests.
func sendMessageWithRecovery(userID string, client messageSender, ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	response, err := client.SendMessage(ctx, to, message, extra...)
	clientManager.requestSocketRecovery(userID, client, err)
	return response, err
}

func sendChatPresenceWithRecovery(userID string, client chatPresenceSender, ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
	err := client.SendChatPresence(ctx, jid, state, media)
	clientManager.requestSocketRecovery(userID, client, err)
	return err
}
