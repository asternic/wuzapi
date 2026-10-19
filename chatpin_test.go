package main

import (
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestChatPinRequest(t *testing.T) {
	for _, chat := range []string{"123@g.us", "456@s.whatsapp.net", "789@lid"} {
		for _, pin := range []bool{true, false} {
			patch, err := (chatPinRequest{Chat: chat, Pin: proto.Bool(pin)}).patch()
			if err != nil {
				t.Fatal(err)
			}
			m := patch.Mutations[0]
			if m.Value.GetPinAction().GetPinned() != pin || m.Index[1] != chat {
				t.Fatalf("incorrect pin mutation: %+v", m)
			}
		}
	}
	for _, r := range []chatPinRequest{{Chat: "123@g.us"}, {Pin: proto.Bool(true)}, {Chat: "123@newsletter", Pin: proto.Bool(true)}} {
		if _, err := r.patch(); err == nil {
			t.Fatalf("accepted invalid request: %+v", r)
		}
	}
}
