package main

import (
	"go.mau.fi/whatsmeow/types"
	"testing"
)

func TestImageStatusAudienceContract(t *testing.T) {
	for _, tc := range []struct {
		name           string
		to             types.JID
		list, contacts []string
		fail           bool
		want           string
	}{
		{name: "automatic", to: types.StatusBroadcastJID},
		{name: "PN", to: types.StatusBroadcastJID, list: []string{" 123 "}, want: types.NewJID("123", types.DefaultUserServer).String()},
		{name: "alias", to: types.StatusBroadcastJID, contacts: []string{"123@c.us"}, want: types.NewJID("123", types.DefaultUserServer).String()},
		{name: "LID", to: types.StatusBroadcastJID, list: []string{types.NewJID("456", types.HiddenUserServer).String()}, want: types.NewJID("456", types.HiddenUserServer).String()},
		{name: "empty", to: types.StatusBroadcastJID, list: []string{}, fail: true},
		{name: "both", to: types.StatusBroadcastJID, list: []string{"123"}, contacts: []string{"123"}, fail: true},
		{name: "DM", to: types.NewJID("123", types.DefaultUserServer), list: []string{"123"}, fail: true},
		{name: "group", to: types.StatusBroadcastJID, list: []string{"123@g.us"}, fail: true},
		{name: "bad user", to: types.StatusBroadcastJID, list: []string{"abc"}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extra, err := imageStatusSendExtra(tc.to, tc.list, tc.contacts)
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v", err)
			}
			if tc.want != "" && (len(extra.StatusJidList) != 1 || extra.StatusJidList[0].String() != tc.want) {
				t.Fatalf("audience=%v", extra.StatusJidList)
			}
			if tc.name == "automatic" && extra.StatusJidList != nil {
				t.Fatal("automatic changed")
			}
		})
	}
}
