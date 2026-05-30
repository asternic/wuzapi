package support

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

type WebhookPayload map[string]interface{}

type WebhookRecorder struct {
	server *httptest.Server

	mu       sync.Mutex
	payloads []WebhookPayload
	notify   chan struct{}
	next     int
}

func NewWebhookRecorder() *WebhookRecorder {
	recorder := &WebhookRecorder{
		notify: make(chan struct{}),
	}

	recorder.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		payload := WebhookPayload{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid webhook payload", http.StatusBadRequest)
			return
		}

		recorder.mu.Lock()
		recorder.payloads = append(recorder.payloads, payload)
		close(recorder.notify)
		recorder.notify = make(chan struct{})
		recorder.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	}))

	return recorder
}

func (recorder *WebhookRecorder) URL() string {
	if recorder == nil || recorder.server == nil {
		return ""
	}

	return recorder.server.URL
}

func (recorder *WebhookRecorder) Close() {
	if recorder == nil || recorder.server == nil {
		return
	}

	recorder.server.Close()
}

func (recorder *WebhookRecorder) WaitFor(ctx context.Context, matcher func(WebhookPayload) bool) (WebhookPayload, error) {
	if recorder == nil {
		return nil, fmt.Errorf("webhook recorder is not configured")
	}

	for {
		recorder.mu.Lock()
		for recorder.next < len(recorder.payloads) {
			payload := recorder.payloads[recorder.next]
			recorder.next++
			if matcher(payload) {
				recorder.mu.Unlock()
				return payload, nil
			}
		}
		notify := recorder.notify
		recorder.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-notify:
		}
	}
}

func (recorder *WebhookRecorder) WaitForMatch(ctx context.Context, timeout time.Duration, matcher func(WebhookPayload) bool) (WebhookPayload, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := recorder.WaitFor(waitCtx, matcher)
	if err != nil {
		return nil, fmt.Errorf("expected webhook event was not received: %w", err)
	}

	return payload, nil
}
