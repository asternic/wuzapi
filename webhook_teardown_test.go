package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// A LoggedOut handler signals the session to stop as soon as it has queued its
// webhook, and the session's teardown deletes the user's HTTP client. The
// delivery must still go out with the client that existed when it was queued.
func TestUserWebhookSurvivesSessionTeardown(t *testing.T) {
	// One P: the delivery goroutine cannot run before this one blocks, so the
	// teardown below always lands first, as it did in production.
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	t.Setenv("WEBHOOK_FORMAT", "json")
	oldEnabled := *webhookRetryEnabled
	*webhookRetryEnabled = false
	defer func() { *webhookRetryEnabled = oldEnabled }()

	received := make(chan string, 1)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
	}))
	defer endpoint.Close()

	const userID = "logged-out-user"
	clientManager.SetHTTPClient(userID, resty.New())
	sendToUserWebHookWithHmac(endpoint.URL, "", []byte(`{"type":"LoggedOut","event":{"OnConnect":false,"Reason":401}}`), userID, "unused-token", nil)
	clientManager.DeleteHTTPClient(userID) // what signalKill's teardown does next

	select {
	case body := <-received:
		if !strings.Contains(body, `"Reason":401`) {
			t.Fatalf("webhook lost the logout code: %s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoggedOut webhook was dropped by the session teardown")
	}
}
