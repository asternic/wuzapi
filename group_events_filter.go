package main

import (
	"os"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// groupEventsIgnored is opt-in so existing installations retain their current
// event handling unless they explicitly request the filter.
func groupEventsIgnored() bool {
	return os.Getenv("WUZAPI_IGNORE_GROUP_EVENTS") == "true"
}

func isGroupJID(jid types.JID) bool {
	return jid.Server == types.GroupServer
}

// shouldDropGroupEvent runs before event-specific handling. In particular,
// messages must be filtered before media download, local history persistence
// and webhook delivery.
func shouldDropGroupEvent(rawEvt any) bool {
	switch evt := rawEvt.(type) {
	case *events.Message:
		return evt.Info.IsGroup || isGroupJID(evt.Info.Chat)
	case *events.UndecryptableMessage:
		return evt.Info.IsGroup || isGroupJID(evt.Info.Chat)
	case *events.Receipt:
		return evt.IsGroup || isGroupJID(evt.Chat)
	case *events.ChatPresence:
		return evt.IsGroup || isGroupJID(evt.Chat)
	case *events.MediaRetry:
		return isGroupJID(evt.ChatID)
	case *events.GroupInfo, *events.JoinedGroup:
		return true
	case *events.Picture:
		return isGroupJID(evt.JID)
	case *events.CallOffer:
		return !evt.GroupJID.IsEmpty()
	case *events.CallAccept:
		return !evt.GroupJID.IsEmpty()
	case *events.CallPreAccept:
		return !evt.GroupJID.IsEmpty()
	case *events.CallTransport:
		return !evt.GroupJID.IsEmpty()
	case *events.CallOfferNotice:
		return evt.Type == "group" || !evt.GroupJID.IsEmpty()
	case *events.CallRelayLatency:
		return !evt.GroupJID.IsEmpty()
	case *events.CallTerminate:
		return !evt.GroupJID.IsEmpty()
	case *events.CallReject:
		return !evt.GroupJID.IsEmpty()
	default:
		return false
	}
}
