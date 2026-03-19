package main

// List of supported event types
var supportedEventTypes = []string{
	// Messages and Communication
	"Message",
	"UndecryptableMessage",
	"Receipt",
	"MediaRetry",
	"ReadReceipt",

	// Groups and Contacts
	"GroupInfo",
	"JoinedGroup",
	"Picture",
	"BlocklistChange",
	"Blocklist",

	// Connection and Session
	"Connected",
	"Disconnected",
	"ConnectFailure",
	"KeepAliveRestored",
	"KeepAliveTimeout",
	"QRTimeout",
	"LoggedOut",
	"ClientOutdated",
	"TemporaryBan",
	"StreamError",
	"StreamReplaced",
	"PairSuccess",
	"PairError",
	"QR",
	"QRScannedWithoutMultidevice",

	// Privacy and Settings
	"PrivacySettings",
	"PushNameSetting",
	"UserAbout",

	// Synchronization and State
	"AppState",
	"AppStateSyncComplete",
	"HistorySync",
	"OfflineSyncCompleted",
	"OfflineSyncPreview",

	// Calls
	"CallOffer",
	"CallAccept",
	"CallTerminate",
	"CallOfferNotice",
	"CallRelayLatency",

	// Presence and Activity
	"Presence",
	"ChatPresence",

	// Identity
	"IdentityChange",

	// Erros
	"CATRefreshError",

	// Newsletter (WhatsApp Channels)
	"NewsletterJoin",
	"NewsletterLeave",
	"NewsletterMuteChange",
	"NewsletterLiveUpdate",

	// Facebook/Meta Bridge
	"FBMessage",

	// Chat lifecycle
	"ChatNew",

	// Special - receives all events
	"All",
}

// Map for quick validation
var eventTypeMap map[string]bool

func init() {
	eventTypeMap = make(map[string]bool)
	for _, eventType := range supportedEventTypes {
		eventTypeMap[eventType] = true
	}
}

// Auxiliary function to validate event type
func isValidEventType(eventType string) bool {
	return eventTypeMap[eventType]
}
