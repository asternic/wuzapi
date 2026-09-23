package main

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

type historyEditContent struct{ target, text string }

// historyEdit accepts both live, already-unwrapped messages and raw HistorySync
// messages, using Whatsmeow's wrapper handling for both paths.
func historyEdit(message *waE2E.Message) (historyEditContent, bool) {
	event := (&events.Message{RawMessage: message}).UnwrapRaw()
	protocol := event.Message.GetProtocolMessage()
	if protocol == nil || protocol.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
		return historyEditContent{}, false
	}
	content := historyEditContent{target: protocol.GetKey().GetID()}
	edited := (&events.Message{RawMessage: protocol.GetEditedMessage()}).UnwrapRaw().Message
	switch {
	case edited.GetConversation() != "":
		content.text = edited.GetConversation()
	case edited.GetExtendedTextMessage() != nil:
		content.text = edited.GetExtendedTextMessage().GetText()
	case edited.GetImageMessage() != nil:
		content.text = edited.GetImageMessage().GetCaption()
	case edited.GetVideoMessage() != nil:
		content.text = edited.GetVideoMessage().GetCaption()
	case edited.GetDocumentMessage() != nil:
		content.text = edited.GetDocumentMessage().GetCaption()
	}
	return content, true
}
