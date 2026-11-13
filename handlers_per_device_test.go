package main

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestAutoEnvelopeSelectsPrimaryDevice(t *testing.T) {
	original := &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{Name: proto.String("quick_reply"), ButtonParamsJSON: proto.String(`{}`)}}}}}}
	base, nodes, hook, err := buildAutoButtonEnvelope(original, "", nil, types.EmptyJID)
	if err != nil || base == nil || len(nodes) != 1 {
		t.Fatalf("build: %v", err)
	}
	if hook(types.NewJID("123", types.DefaultUserServer)).GetViewOnceMessage() == nil {
		t.Fatal("primary envelope missing")
	}
	if hook(types.NewADJID("123", 0, 1)) != nil {
		t.Fatal("linked device must use base")
	}
}
