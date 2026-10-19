package main

import (
	"fmt"
	"regexp"
	"strconv"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var revokeSenderPattern = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]+))?(?::([0-9]+))?@(s\.whatsapp\.net|lid|c\.us)$`)

// ParseJID alone also accepts server-only strings and silently truncates device
// numbers. Validate the complete user JID before parsing or normalizing it.
func parseRevokeSender(value string) (types.JID, error) {
	if value == "" {
		return types.EmptyJID, nil
	}
	invalid := fmt.Errorf("invalid SenderJID: expected a complete numeric user JID ending in @s.whatsapp.net or @lid")
	parts := revokeSenderPattern.FindStringSubmatch(value)
	if parts == nil {
		return types.EmptyJID, invalid
	}
	for i, bits := range []int{8, 16} {
		if parts[i+2] != "" {
			if _, err := strconv.ParseUint(parts[i+2], 10, bits); err != nil {
				return types.EmptyJID, invalid
			}
		}
	}
	sender, err := types.ParseJID(value)
	if err != nil {
		return types.EmptyJID, invalid
	}
	if sender.Server == types.LegacyUserServer {
		sender.Server = types.DefaultUserServer
	}
	return sender.ToNonAD(), nil
}

func buildDeleteMessage(client *whatsmeow.Client, chat types.JID, senderValue, id string) (*waE2E.Message, error) {
	sender, err := parseRevokeSender(senderValue)
	if err != nil {
		return nil, err
	}
	message := client.BuildRevoke(chat, sender, id)
	// Let Whatsmeow recognize either of the account's own PN/LID identities.
	// Providing one's own SenderJID must not require a group or admin privileges.
	if !message.GetProtocolMessage().GetKey().GetFromMe() && chat.Server != types.GroupServer {
		return nil, fmt.Errorf("SenderJID for another user is only supported in group chats")
	}
	return message, nil
}
