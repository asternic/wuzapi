package main

import (
	"testing"

	"github.com/patrickmn/go-cache"
)

// TestUpdateAndGetUserSubscriptionsFromCache proves that event-subscription
// resolution works entirely through the userinfocache/DB path — which is why
// the write-only MyClient.subscriptions field (removed in this change) was
// unnecessary. updateAndGetUserSubscriptions is the only consumer of a user's
// subscriptions and re-reads them per event, so dropping the dead field cannot
// affect which events are delivered.
func TestUpdateAndGetUserSubscriptionsFromCache(t *testing.T) {
	s := makeTestServer(t)
	const token = "sub-token"
	const userID = "sub-user"

	// Seed the cache exactly as the connect/webhook handlers do.
	userinfocache.Set(token, Values{m: map[string]string{"Events": "Message,ReadReceipt"}}, cache.NoExpiration)
	t.Cleanup(func() { userinfocache.Delete(token) })

	mycli := &MyClient{userID: userID, token: token, db: s.db}
	got, err := updateAndGetUserSubscriptions(mycli)
	if err != nil {
		t.Fatalf("updateAndGetUserSubscriptions returned error: %v", err)
	}

	want := []string{"Message", "ReadReceipt"}
	if len(got) != len(want) {
		t.Fatalf("subscriptions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("subscriptions[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Unsupported event names must be filtered out.
	userinfocache.Set(token, Values{m: map[string]string{"Events": "Message,NotAReal Event"}}, cache.NoExpiration)
	got, err = updateAndGetUserSubscriptions(mycli)
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "Message" {
		t.Errorf("unsupported events not filtered: got %v, want [Message]", got)
	}
}
