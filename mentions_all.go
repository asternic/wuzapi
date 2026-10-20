package main

import (
	"context"
	"fmt"
	"regexp"

	"go.mau.fi/whatsmeow/types"
)

var everyoneToken = regexp.MustCompile(`(^|[\s(\[{,;!?])@everyone($|[\s)\]},;!?.])`)

// MentionAll keeps the visible message unchanged and expands current group members.
func resolveAllMentions(ctx context.Context, chat types.JID, explicit []string, text string, all bool, lookup func(context.Context, types.JID) (*types.GroupInfo, error)) ([]string, error) {
	if !all && !everyoneToken.MatchString(text) {
		return explicit, nil
	}
	if chat.Server != types.GroupServer {
		return nil, fmt.Errorf("mention-all requires a group recipient")
	}
	info, err := lookup(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("fetch mention-all participants: %w", err)
	}
	if info == nil {
		return nil, fmt.Errorf("group information unavailable")
	}
	result := append([]string(nil), explicit...)
	for _, member := range info.Participants {
		if !member.JID.IsEmpty() {
			result = append(result, member.JID.String())
		}
	}
	return mergeMentionJIDs(result, ""), nil
}
