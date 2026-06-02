package main

import (
	"fmt"
	"sync"
	"testing"
)

// TestKillChannelHelpers covers the mutex-guarded killchannel helpers that
// replaced direct map access. The set/get/signal/delete cycle must behave
// correctly, and concurrent access must not panic. Under `go test -race` this
// also proves the previous unguarded map access ("concurrent map read and map
// write" from request + session goroutines) is gone.
func TestKillChannelHelpers(t *testing.T) {
	const u = "func-user"

	// set -> get returns the same channel.
	ch := make(chan bool, 1)
	setKillChannel(u, ch)
	got, ok := getKillChannel(u)
	if !ok || got != ch {
		t.Fatalf("getKillChannel after set: got=%v ok=%v, want the same channel", got, ok)
	}

	// signalKill delivers a non-blocking value into the buffered channel.
	signalKill(u)
	select {
	case v := <-ch:
		if !v {
			t.Errorf("kill channel delivered %v, want true", v)
		}
	default:
		t.Error("signalKill did not deliver a value")
	}

	// delete removes the entry; signalKill on a missing entry is a safe no-op.
	deleteKillChannel(u)
	if _, ok := getKillChannel(u); ok {
		t.Error("entry still present after deleteKillChannel")
	}
	signalKill(u) // must not panic on a missing entry
}

// TestKillChannelConcurrent hammers the helpers from many goroutines. The point
// is the -race build: the old bare-map access raced; the guarded helpers do not.
func TestKillChannelConcurrent(t *testing.T) {
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		uid := fmt.Sprintf("race-user-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			setKillChannel(uid, make(chan bool, 1))
			signalKill(uid)
			_, _ = getKillChannel(uid)
			deleteKillChannel(uid)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = getKillChannel(uid)
			signalKill(uid)
		}()
	}
	wg.Wait()
}
