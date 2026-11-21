package main

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestGroupParticipantChanges(t *testing.T) {
	group := types.NewJID("120363000000000000", types.GroupServer)
	admin := types.NewJID("111", types.HiddenUserServer)
	adminPN := types.NewJID("5511900000000", types.DefaultUserServer)
	evt := &events.GroupInfo{JID: group, Sender: &admin, SenderPN: &adminPN, JoinReason: "invite",
		Join:  []types.JID{types.NewJID("222", types.HiddenUserServer)},
		Leave: []types.JID{types.NewJID("333", types.HiddenUserServer), types.NewJID("444", types.HiddenUserServer)},
	}
	got := groupParticipantChanges(evt)
	if len(got) != 2 || got[0].Action != "join" || got[1].Action != "leave" || len(got[1].Participants) != 2 {
		t.Fatalf("unexpected changes: %+v", got)
	}
	if got[0].Group != group.String() || got[0].Author != admin.String() || got[0].AuthorPN != adminPN.String() || got[0].JoinReason != "invite" || got[1].JoinReason != "" {
		t.Fatalf("metadata: %+v", got)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"action":"leave"`) {
		t.Fatalf("json: %s", b)
	}
	if groupParticipantChanges(&events.GroupInfo{JID: group}) != nil {
		t.Fatal("name/topic-only change must not produce participant changes")
	}
}
