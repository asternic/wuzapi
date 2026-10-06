package main

import (
	"context"
	"errors"
)

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
	if sessionActive(userID) {
		return nil, "", errSessionActive
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
