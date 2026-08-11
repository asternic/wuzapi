// ratelimit.go - Rate limit por peer (sender) para prevenir decrypt burst
//
// Contexto: quando WhatsApp faz sender key rotation em grupos ativos, msgs podem
// chegar em burst com iterations muito à frente. libsignal-go fará ratchet
// forward de até 2000 steps por msg (groups/GroupCipher.go:getSenderKey). Se
// 100+ msgs chegam próximas do mesmo peer, CPU/mem satura, decrypt trunca
// payloads longos.
//
// Fix: token bucket por peer. Se peer excede N msgs em janela T, dropar
// excedente. Msgs legítimas de conversação (< 5 msgs/segundo) passam livres.
// Bombardeio de rotação (>50 msgs/segundo) é achatado.
//
// Config via env:
//   RATE_LIMIT_PEER_MSG_PER_SEC=20   (default; ajuste conforme uso)
//   RATE_LIMIT_PEER_ENABLED=true     (default; setar false pra desligar)

package main

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type peerBucket struct {
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

type peerRateLimiter struct {
	buckets  sync.Map // key = peer JID string, value = *peerBucket
	rate     float64  // tokens per second
	capacity float64  // max tokens
	enabled  bool
}

var globalPeerLimiter *peerRateLimiter
var globalPeerLimiterOnce sync.Once

func getPeerLimiter() *peerRateLimiter {
	globalPeerLimiterOnce.Do(func() {
		rate := parseEnvFloat("RATE_LIMIT_PEER_MSG_PER_SEC", 20.0)
		enabled := parseEnvBool("RATE_LIMIT_PEER_ENABLED", true)
		globalPeerLimiter = &peerRateLimiter{
			rate:     rate,
			capacity: rate * 2, // burst tolerance = 2s
			enabled:  enabled,
		}
		log.Info().
			Float64("rate_per_sec", rate).
			Float64("capacity", rate*2).
			Bool("enabled", enabled).
			Msg("[PEER_RATE_LIMIT] initialized")
	})
	return globalPeerLimiter
}

// Allow returns true if peer msg should be processed, false if dropped.
func (p *peerRateLimiter) Allow(peerKey string) bool {
	if !p.enabled {
		return true
	}
	now := time.Now()
	bucketI, _ := p.buckets.LoadOrStore(peerKey, &peerBucket{
		tokens:     p.capacity,
		lastRefill: now,
	})
	b := bucketI.(*peerBucket)
	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * p.rate
	if b.tokens > p.capacity {
		b.tokens = p.capacity
	}
	b.lastRefill = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func parseEnvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func parseEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
