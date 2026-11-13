package main

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// Parse only: audience normalization and privacy enforcement stay in whatsmeow.
func imageStatusSendExtra(to types.JID, list, contacts []string) (whatsmeow.SendRequestExtra, error) {
	var extra whatsmeow.SendRequestExtra
	if list != nil && contacts != nil {
		return extra, fmt.Errorf("use StatusJidList OR Contacts")
	}
	if list == nil {
		list = contacts
	}
	if list == nil {
		return extra, nil
	}
	if to != types.StatusBroadcastJID {
		return extra, fmt.Errorf("status audience requires status@broadcast")
	}
	if len(list) == 0 {
		return extra, fmt.Errorf("status audience must not be empty")
	}
	extra.StatusJidList = make([]types.JID, 0, len(list))
	for _, raw := range list {
		raw = strings.TrimSpace(raw)
		if !strings.Contains(raw, "@") {
			raw += "@s.whatsapp.net"
		}
		if strings.HasSuffix(raw, "@c.us") {
			raw = strings.TrimSuffix(raw, "@c.us") + "@s.whatsapp.net"
		}
		jid, err := types.ParseJID(raw)
		if err != nil || jid.User == "" || (jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer) {
			return whatsmeow.SendRequestExtra{}, fmt.Errorf("invalid status contact")
		}
		for _, ch := range jid.User {
			if ch < '0' || ch > '9' {
				return whatsmeow.SendRequestExtra{}, fmt.Errorf("invalid status contact user")
			}
		}
		extra.StatusJidList = append(extra.StatusJidList, jid.ToNonAD())
	}
	return extra, nil
}
