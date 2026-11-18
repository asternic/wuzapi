package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestUnhandledEventLogsOnlyItsType(t *testing.T) {
	preserveLogLevel(t)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	previous := log.Logger
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })

	const phone, name = "15550001111", "push-name-sentinel"
	(&MyClient{}).myEventHandler(&events.PushName{
		JID:         types.NewJID(phone, types.DefaultUserServer),
		OldPushName: name + "-old",
		NewPushName: name,
	})

	logged := output.String()
	if !strings.Contains(logged, "Unhandled event") || !strings.Contains(logged, "*events.PushName") {
		t.Fatalf("expected an unhandled-event line naming the event type, got %q", logged)
	}
	for _, private := range []string{phone, name} {
		if strings.Contains(logged, private) {
			t.Fatalf("event contents leaked into log output: %q", private)
		}
	}
}
