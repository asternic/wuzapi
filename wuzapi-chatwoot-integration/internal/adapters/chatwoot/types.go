package chatwoot

// ChatwootContactPayload is used to create a contact in Chatwoot.
type ChatwootContactPayload struct {
	InboxID     int    `json:"inbox_id"` // Changed to int as per requirement
	Name        string `json:"name,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
	Email       string `json:"email,omitempty"`
	// CustomAttributes map[string]string `json:"custom_attributes,omitempty"` // Example
}

// ChatwootContact represents a contact in Chatwoot. Renamed from ChatwootContactResponse for clarity.
type ChatwootContact struct {
	ID               int                    `json:"id"`
	Name             string                 `json:"name"`
	Email            string                 `json:"email"`
	PhoneNumber      string                 `json:"phone_number"`
	AvatarURL        string                 `json:"avatar_url"`
	Type             string                 `json:"type"` // "contact"
	PubsubToken      string                 `json:"pubsub_token"`
	CustomAttributes map[string]interface{} `json:"custom_attributes"`
	// Add other fields as necessary, e.g., from Chatwoot's API documentation
	// "additional_attributes", "source_id", "created_at", "updated_at"
}

// ChatwootContactSearchPayload is used when searching for contacts.
// Chatwoot API typically returns a list under a "payload" key.
type ChatwootContactSearchPayload struct {
	Payload []ChatwootContact `json:"payload"`
}

// ChatwootCreateContactResponse is the direct response when creating a contact.
// It often includes a "payload" which contains the contact itself, or just the contact fields directly.
// Assuming it returns the contact directly for simplicity, matching ChatwootContact.
// If it's nested under "payload", then this would be:
// type ChatwootCreateContactResponse struct {
//    Payload ChatwootContact `json:"payload"`
// }
// For now, let's assume the CreateContact method in the client will parse into ChatwootContact directly.


// ChatwootConversationPayload is used to create a conversation.
type ChatwootConversationPayload struct {
	SourceID    string `json:"source_id,omitempty"` // Wuzapi Sender ID (phone number) or other external ID
	InboxID     int    `json:"inbox_id"`           // Required: ID of the inbox (must be int)
	ContactID   int    `json:"contact_id"`         // Required: ID of the existing contact
	Status      string `json:"status,omitempty"`   // e.g., "open", "pending"; defaults to "open" if not provided
	AssigneeID  int    `json:"assignee_id,omitempty"`
	// AdditionalAttributes map[string]interface{} `json:"additional_attributes,omitempty"` // For custom attributes on conversation
}

// ChatwootConversation represents a conversation in Chatwoot.
// Renamed from ChatwootConversationResponse for consistency.
type ChatwootConversation struct {
	ID          int    `json:"id"`
	ContactID   int    `json:"contact_id"` // This is usually part of the contact object within the conversation payload from API
	InboxID     int    `json:"inbox_id"`
	Status      string `json:"status"`
	AccountID   int    `json:"account_id"`
	AgentLastSeenAt int64 `json:"agent_last_seen_at"` // Unix timestamp
	ContactLastSeenAt int64 `json:"contact_last_seen_at"` // Unix timestamp
	Timestamp         int64 `json:"timestamp"` // Unix timestamp of the last activity
	// Meta        ChatwootConversationMeta `json:"meta"` // Contains sender, assignee etc.
	// Add other relevant fields like messages array, labels, etc.
}

// ChatwootContactConversationsResponse is used when listing conversations for a contact.
// Chatwoot API returns a list under a "payload" key.
type ChatwootContactConversationsResponse struct {
	Payload []ChatwootConversation `json:"payload"`
}


// ChatwootMessagePayload is used to create a message in a Chatwoot conversation.
type ChatwootMessagePayload struct {
	Content     string                    `json:"content,omitempty"` // Caption for media, or text message content
	MessageType string                    `json:"message_type"`
	ContentType string                    `json:"content_type"` // "text", or "input_file" when sending attachments
	Private     bool                      `json:"private"`
	SourceID    string                    `json:"source_id,omitempty"`
	Attachments []ChatwootAttachmentToken `json:"attachment_ids,omitempty"` // Use this to send IDs of pre-uploaded attachments
}

// ChatwootAttachmentToken is a helper type for passing attachment IDs when creating a message.
type ChatwootAttachmentToken struct {
	ID int `json:"id"`
}

// ChatwootMessage represents a message object in Chatwoot, often part of a response.
// Renamed from ChatwootCreateMessageResponse for clarity and consistency.
type ChatwootMessage struct {
	ID               int                    `json:"id"`
	Content          string                 `json:"content"`
	AccountID        int                    `json:"account_id"`
	InboxID          int                    `json:"inbox_id"`
	ConversationID   int                    `json:"conversation_id"`
	MessageType      int                    `json:"message_type"` // Note: Chatwoot API uses integer for message_type (0 for incoming, 1 for outgoing, 2 for template)
	ContentType      string                 `json:"content_type"` // e.g., "text", "incoming_email"
	Private          bool                   `json:"private"`
	CreatedAt        int64                  `json:"created_at"` // Unix timestamp
	SourceID         *string                `json:"source_id"`  // Pointer to allow null
	Sender           *ChatwootMessageSender `json:"sender,omitempty"` // Details about the sender (contact or agent)
	Attachments      []ChatwootAttachment   `json:"attachments,omitempty"` // Details of attachments on a received message
}

// ChatwootMessageSender represents the sender of a message in Chatwoot.
type ChatwootMessageSender struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	Type        string `json:"type"` // "contact", "agent_bot", "user"
}


// ChatwootAttachment represents an attachment object in Chatwoot, often part of a message response or upload response.
// Renamed from ChatwootAttachmentResponse for clarity.
type ChatwootAttachment struct {
	ID        int    `json:"id"`
	FileType  string `json:"file_type"` // e.g., "image", "audio", "video", "file", "location" (for location type messages)
	DataURL   string `json:"data_url"`  // Public URL of the attachment, if available
	FileURL   string `json:"file_url"`  // Internal URL of the attachment
	ThumbURL  string `json:"thumb_url,omitempty"` // Thumbnail URL for images/videos
	FileSize  int    `json:"file_size,omitempty"`
	FileName  string `json:"file_name,omitempty"` // If provided during upload or derived
}


// ChatwootWebhookPayload represents the data received from a Chatwoot webhook.
// This will vary greatly depending on the event type. This is a generic structure.
type ChatwootWebhookPayload struct {
	Event           string      `json:"event"` // e.g., "message_created", "conversation_status_changed"
	Conversation    *ChatwootConversation `json:"conversation,omitempty"`
	Message         *ChatwootMessage `json:"message,omitempty"` // Changed to ChatwootMessage
	Contact         *ChatwootContact    `json:"contact,omitempty"`
	AccountID       int         `json:"account_id"`
	// Add other fields specific to different events
}
