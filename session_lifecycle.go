package main

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
)

// Guarded by proxyPoolMu, like session start reservations.
var deletingSessions = make(map[string]bool)

func reserveSessionDeletion(userID string) error {
	proxyPoolMu.Lock()
	defer proxyPoolMu.Unlock()
	if deletingSessions[userID] {
		return errors.New("user deletion is already in progress")
	}
	deletingSessions[userID] = true
	return nil
}

func releaseSessionDeletion(userID string) {
	proxyPoolMu.Lock()
	delete(deletingSessions, userID)
	proxyPoolMu.Unlock()
}

var errSessionActive = errors.New("session is active; disconnect and wait for shutdown first")

// Cancel both initial connection work and retry/QR waits on disconnect. The
// caller also cancels on early return so the watcher never outlives the session.
func sessionContext(kill <-chan bool) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-kill:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// A registered goroutine owns its proxy until it has finished shutting down,
// including QR pairing, connection retries and WhatsApp automatic reconnects.
func sessionActive(userID string) bool {
	if _, registered := getKillChannel(userID); registered {
		return true
	}
	client := clientManager.GetWhatsmeowClient(userID)
	return client != nil && client.IsConnected()
}

// prepareClientStart reserves the session before the caller starts a goroutine.
// Pool assignment and reservation share the lock used by release and explicit
// proxy updates, so release cannot slip between assignment and startup.
// Startup reconnects preserve the existing proxy without assigning a new one.
func (s *server) prepareClientStart(userID string, usePool bool) (chan bool, string, error) {
	proxyPoolMu.Lock()
	defer proxyPoolMu.Unlock()
	if deletingSessions[userID] {
		return nil, "", errors.New("user deletion is in progress")
	}
	if sessionActive(userID) {
		return nil, "", errSessionActive
	}
	var exists bool
	if err := s.db.QueryRow(s.db.Rebind(`SELECT EXISTS(SELECT 1 FROM users WHERE id=?)`), userID).Scan(&exists); err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", errors.New("user no longer exists")
	}

	var proxyURL string
	if usePool {
		var err error
		proxyURL, err = s.proxyForConnectLocked(userID)
		if err != nil {
			return nil, "", err
		}
	}
	kill := make(chan bool, 1)
	setKillChannel(userID, kill)
	return kill, proxyURL, nil
}

// clearSessionQRCode cleans up transient state without changing whether the
// user should reconnect on the next server startup.
func (s *server) clearSessionQRCode(userID string) error {
	_, err := s.db.Exec(`UPDATE users SET qrcode='' WHERE id=$1`, userID)
	return err
}

// connectSessionWithRetry returns success as soon as any attempt succeeds.
func connectSessionWithRetry(ctx context.Context, connect func(context.Context) error, attempts int, baseWait time.Duration) error {
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			waitTime := time.Duration(attempt) * baseWait
			log.Warn().
				Int("attempt", attempt+1).
				Int("max_retries", attempts).
				Dur("wait_time", waitTime).
				Msg("Retrying connection after delay")
			timer := time.NewTimer(waitTime)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		err := connect(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			log.Info().
				Int("attempt", attempt+1).
				Msg("Successfully connected to WhatsApp")
			return nil
		}

		lastErr = err
		log.Warn().
			Err(err).
			Int("attempt", attempt+1).
			Int("max_retries", attempts).
			Msg("Failed to connect to WhatsApp")
	}

	return lastErr
}

// Wait for the owner to finish its deferred cleanup before removing its store.
func waitForSessionStop(ctx context.Context, userID string) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, running := getKillChannel(userID); !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
