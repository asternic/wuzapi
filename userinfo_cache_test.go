package main

import (
	"fmt"
	"sync"
	"testing"
)

// TestUpdateUserInfoCopyOnWrite proves updateUserInfo does not mutate the
// shared map. The map inside Values lives in userinfocache and is handed to
// request goroutines via the request context, so an in-place write races with
// concurrent readers. This assertion is deterministic (no -race needed): the
// previous implementation mutated `base` and would fail here.
func TestUpdateUserInfoCopyOnWrite(t *testing.T) {
	base := Values{m: map[string]string{"Events": "Message", "Webhook": "https://example.test"}}

	updated := updateUserInfo(base, "Events", "Message,ReadReceipt").(Values)

	if got := base.Get("Events"); got != "Message" {
		t.Errorf("shared Values was mutated in place: base Events=%q, want %q", got, "Message")
	}
	if got := updated.Get("Events"); got != "Message,ReadReceipt" {
		t.Errorf("returned Values not updated: Events=%q, want %q", got, "Message,ReadReceipt")
	}
	if got := updated.Get("Webhook"); got != "https://example.test" {
		t.Errorf("returned Values lost an unrelated field: Webhook=%q", got)
	}
}

// TestUpdateUserInfoConcurrent exercises the data race directly. Run with
// `go test -race`: the previous in-place mutation triggered "concurrent map
// read and map write" when one goroutine updated while another read. The
// copy-on-write version never writes to the shared map, so it is clean.
func TestUpdateUserInfoConcurrent(t *testing.T) {
	base := Values{m: map[string]string{"Events": "Message"}}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_ = updateUserInfo(base, "Events", fmt.Sprintf("v%d", i))
		}(i)
		go func() {
			defer wg.Done()
			_ = base.Get("Events")
		}()
	}
	wg.Wait()
}
