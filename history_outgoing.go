package main

import (
	"strconv"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// outgoingHistoryAction is the history row for a reaction, edit or revoke sent
// through the API. Rows use the same columns as received ones: reactions and
// edits keep their target in quoted_message_id, revokes in text_content.
type outgoingHistoryAction struct {
	messageType string
	textContent string
	target      string
	message     *waE2E.Message
}

// outgoingAction reads a built message the way the live event handler reads a
// received one. The stored message is unwrapped, because received events are:
// consumers read datajson's Message.protocolMessage, which BuildEdit nests
// inside editedMessage.
func outgoingAction(message *waE2E.Message) (outgoingHistoryAction, bool) {
	unwrapped := (&events.Message{RawMessage: message}).UnwrapRaw().Message
	if edit, ok := historyEdit(unwrapped); ok {
		return outgoingHistoryAction{"edit", edit.text, edit.target, unwrapped}, true
	}
	if reaction := unwrapped.GetReactionMessage(); reaction != nil {
		return outgoingHistoryAction{"reaction", reaction.GetText(), reaction.GetKey().GetID(), unwrapped}, true
	}
	if protocol := unwrapped.GetProtocolMessage(); protocol != nil && protocol.GetType() == waE2E.ProtocolMessage_REVOKE {
		return outgoingHistoryAction{"delete", protocol.GetKey().GetID(), "", unwrapped}, true
	}
	return outgoingHistoryAction{}, false
}

// saveOutgoingAction stores a reaction, edit or revoke after a successful send.
// The row's message_id is the ID of the send itself (resp.ID), not the target's:
// saving under the target's ID would collide with the original message and be
// dropped by the unique (user_id, message_id) constraint.
func (s *server) saveOutgoingAction(userinfo Values, chat types.JID, resp whatsmeow.SendResponse, message *waE2E.Message) {
	action, ok := outgoingAction(message)
	if !ok {
		log.Warn().Str("messageID", resp.ID).Msg("Outgoing message is not a reaction, edit or revoke, skipping history")
		return
	}
	historyLimit, _ := strconv.Atoi(userinfo.Get("History"))
	s.saveOutgoingRow(userinfo.Get("Id"), chat.String(), resp.ID, action.messageType, action.textContent, "", action.target, resp.Timestamp, historyLimit, action.message)
}
