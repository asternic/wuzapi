package main

// Flat participant-change summary on GroupInfo webhooks, shaped like WAHA's
// group.v2.participants, so integrations can tell join/leave/promote/demote
// apart without diffing the raw event. The raw event stays in "event".

import (
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type groupParticipantChange struct {
	Group        string   `json:"group"`
	Action       string   `json:"action"` // join | leave | promote | demote
	Participants []string `json:"participants"`
	Author       string   `json:"author,omitempty"`
	AuthorPN     string   `json:"authorPN,omitempty"`
	JoinReason   string   `json:"joinReason,omitempty"`
}

func groupParticipantChanges(evt *events.GroupInfo) []groupParticipantChange {
	if evt == nil {
		return nil
	}
	author, authorPN := "", ""
	if evt.Sender != nil && !evt.Sender.IsEmpty() {
		author = evt.Sender.String()
	}
	if evt.SenderPN != nil && !evt.SenderPN.IsEmpty() {
		authorPN = evt.SenderPN.String()
	}
	var out []groupParticipantChange
	add := func(action string, jids []types.JID) {
		if len(jids) == 0 {
			return
		}
		list := make([]string, len(jids))
		for i, j := range jids {
			list[i] = j.String()
		}
		c := groupParticipantChange{Group: evt.JID.String(), Action: action, Participants: list, Author: author, AuthorPN: authorPN}
		if action == "join" {
			c.JoinReason = evt.JoinReason
		}
		out = append(out, c)
	}
	add("join", evt.Join)
	add("leave", evt.Leave)
	add("promote", evt.Promote)
	add("demote", evt.Demote)
	return out
}
