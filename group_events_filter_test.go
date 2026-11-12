package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestGroupEventsIgnoredRequiresAnExplicitTrueValue(t *testing.T) {
	for _, value := range []string{"", "false", "TRUE", "1", " true "} {
		t.Setenv("WUZAPI_IGNORE_GROUP_EVENTS", value)
		if groupEventsIgnored() {
			t.Fatalf("enabled group event filtering for %q", value)
		}
	}
	t.Setenv("WUZAPI_IGNORE_GROUP_EVENTS", "true")
	if !groupEventsIgnored() {
		t.Fatal("did not enable group event filtering for true")
	}
}

func TestShouldDropGroupEvent(t *testing.T) {
	direct := types.NewJID("5511999999999", types.DefaultUserServer)
	group := types.NewJID("120363000000000000", types.GroupServer)

	for name, rawEvt := range map[string]any{
		"group message":               &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: group, IsGroup: true}}},
		"group undecryptable message": &events.UndecryptableMessage{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: group, IsGroup: true}}},
		"group receipt":               &events.Receipt{MessageSource: types.MessageSource{Chat: group, IsGroup: true}},
		"group chat presence":         &events.ChatPresence{MessageSource: types.MessageSource{Chat: group, IsGroup: true}},
		"group media retry":           &events.MediaRetry{ChatID: group},
		"group info":                  &events.GroupInfo{JID: group},
		"joined group":                &events.JoinedGroup{},
		"group picture":               &events.Picture{JID: group},
		"group call":                  &events.CallOffer{BasicCallMeta: types.BasicCallMeta{From: direct, GroupJID: group}},
		"group call notice":           &events.CallOfferNotice{BasicCallMeta: types.BasicCallMeta{From: direct}, Type: "group"},
	} {
		t.Run(name, func(t *testing.T) {
			if !shouldDropGroupEvent(rawEvt) {
				t.Fatal("group event was not dropped")
			}
		})
	}

	for name, rawEvt := range map[string]any{
		"direct message":       &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: direct}}},
		"direct receipt":       &events.Receipt{MessageSource: types.MessageSource{Chat: direct}},
		"direct chat presence": &events.ChatPresence{MessageSource: types.MessageSource{Chat: direct}},
		"direct call":          &events.CallOffer{BasicCallMeta: types.BasicCallMeta{From: direct}},
		"newsletter update":    &events.NewsletterLiveUpdate{},
	} {
		t.Run(name, func(t *testing.T) {
			if shouldDropGroupEvent(rawEvt) {
				t.Fatal("non-group event was dropped")
			}
		})
	}
}
