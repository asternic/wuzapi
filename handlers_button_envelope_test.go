package main

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestButtonEnvelopeModes(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		invalid     bool
	}{{"", "original", false}, {"fork", "fork", false}, {"viewonce", "viewonce", false}, {"auto", "", true}} {
		got, err := normalizeButtonEnvelope(tc.input)
		if (err != nil) != tc.invalid || got != tc.want {
			t.Fatalf("input %q: %q %v", tc.input, got, err)
		}
	}
	im := &waE2E.InteractiveMessage{InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{Name: proto.String("quick_reply")}}}}}
	for _, mode := range []string{"fork", "viewonce"} {
		msg, nodes, err := buildButtonEnvelopeAt(im, mode, "", make([]byte, 32), make([]byte, 20), 1)
		if err != nil || len(nodes) != 1 {
			t.Fatalf("mode %s: %v", mode, err)
		}
		if (msg.GetViewOnceMessage() != nil) != (mode == "viewonce") {
			t.Fatalf("wrong wrapping for %s", mode)
		}
	}
}
