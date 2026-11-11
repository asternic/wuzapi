package services

import (
	"fmt"
	"strings"
	"wuzapi-chatwoot-integration/internal/adapters/chatwoot"
	"wuzapi-chatwoot-integration/internal/adapters/wuzapi" // For wuzapi.WuzapiMessageData
	"wuzapi-chatwoot-integration/internal/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MessageSyncService handles sending messages to Chatwoot.
type MessageSyncService struct {
	wuzapiClient   *wuzapi.Client // Added Wuzapi client for downloading media
	chatwootClient *chatwoot.Client
	db             *gorm.DB // For potential future use (e.g., queuing, message status updates)
}

// NewMessageSyncService creates a new MessageSyncService.
func NewMessageSyncService(wzClient *wuzapi.Client, cwClient *chatwoot.Client, db *gorm.DB) (*MessageSyncService, error) {
	if wzClient == nil {
		return nil, fmt.Errorf("Wuzapi client cannot be nil for MessageSyncService")
	}
	if cwClient == nil {
		return nil, fmt.Errorf("Chatwoot client cannot be nil for MessageSyncService")
	}
	if db == nil {
		return nil, fmt.Errorf("database instance (gorm.DB) cannot be nil for MessageSyncService")
	}
	return &MessageSyncService{
		wuzapiClient:   wzClient,
		chatwootClient: cwClient,
		db:             db,
	}, nil
}

// SyncWuzapiTextMessageToChatwoot prepares and sends a text message from Wuzapi to Chatwoot.
func (s *MessageSyncService) SyncWuzapiTextMessageToChatwoot(
	conversationMap *models.ConversationMap,
	wuzapiMsgData *wuzapi.WuzapiMessageData,
) error {
	if conversationMap == nil {
		return fmt.Errorf("conversationMap cannot be nil")
	}
	if wuzapiMsgData == nil {
		return fmt.Errorf("wuzapiMsgData cannot be nil")
	}

	textContent := wuzapiMsgData.Text
	if textContent == "" { // Fallback to Content field if Text (body) is empty
		textContent = wuzapiMsgData.Content
	}
	if textContent == "" {
		log.Warn().Str("wuzapiMessageID", wuzapiMsgData.ID).Msg("Wuzapi message has no text content to sync.")
		return nil // Or an error if empty messages should not be synced or handled differently
	}

	log.Info().
		Str("wuzapiMessageID", wuzapiMsgData.ID).
		Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
		Msg("Attempting to sync Wuzapi text message to Chatwoot")

	payload := chatwoot.ChatwootMessagePayload{
		Content:     textContent,
		MessageType: "incoming", // Message from external source (Wuzapi) is "incoming" to Chatwoot
		ContentType: "text",
		Private:     false,
		SourceID:    wuzapiMsgData.ID, // Store Wuzapi message ID for traceability
	}

	createdMessage, err := s.chatwootClient.CreateMessage(int(conversationMap.ChatwootConversationID), payload)
	if err != nil {
		log.Error().Err(err).
			Str("wuzapiMessageID", wuzapiMsgData.ID).
			Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
			Msg("Failed to create message in Chatwoot")
		return fmt.Errorf("failed to create message in Chatwoot for Wuzapi msg ID %s: %w", wuzapiMsgData.ID, err)
	}

	log.Info().
		Str("wuzapiMessageID", wuzapiMsgData.ID).
		Int("chatwootMessageID", createdMessage.ID).
		Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
		Msg("Successfully synced Wuzapi text message to Chatwoot")

	// Future: Update QueuedMessage status if this was from a queue.
	// Or, directly log message_id mapping if needed for other processes.

	return nil
}

// SyncWuzapiMediaMessageToChatwoot handles downloading media from Wuzapi and uploading it to Chatwoot,
// then sends a message to Chatwoot with the attachment.
func (s *MessageSyncService) SyncWuzapiMediaMessageToChatwoot(
	conversationMap *models.ConversationMap,
	wuzapiMsgData *wuzapi.WuzapiMessageData,
) error {
	if conversationMap == nil {
		return fmt.Errorf("conversationMap cannot be nil for media message sync")
	}
	if wuzapiMsgData == nil {
		return fmt.Errorf("wuzapiMsgData cannot be nil for media message sync")
	}
	if wuzapiMsgData.MediaURL == "" {
		return fmt.Errorf("MediaURL is empty in wuzapiMsgData for Wuzapi msg ID %s", wuzapiMsgData.ID)
	}

	log.Info().
		Str("wuzapiMessageID", wuzapiMsgData.ID).
		Str("mediaURL", wuzapiMsgData.MediaURL).
		Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
		Msg("Attempting to sync Wuzapi media message to Chatwoot")

	// 1. Download Media from Wuzapi
	mediaData, contentType, err := s.wuzapiClient.DownloadMedia(wuzapiMsgData.MediaURL)
	if err != nil {
		log.Error().Err(err).Str("mediaURL", wuzapiMsgData.MediaURL).Msg("Failed to download media from Wuzapi")
		return fmt.Errorf("failed to download Wuzapi media %s: %w", wuzapiMsgData.MediaURL, err)
	}
	log.Info().Str("mediaURL", wuzapiMsgData.MediaURL).Str("contentType", contentType).Int("size", len(mediaData)).Msg("Media downloaded from Wuzapi")

	// Determine filename
	fileName := wuzapiMsgData.FileName
	if fileName == "" {
		// Try to derive from URL, or generate a generic one
		urlParts := strings.Split(wuzapiMsgData.MediaURL, "/")
		if len(urlParts) > 0 {
			fileName = urlParts[len(urlParts)-1]
			fileName = strings.Split(fileName, "?")[0] // Remove query params if any
		}
		if fileName == "" {
			fileName = fmt.Sprintf("%s_attachment", wuzapiMsgData.ID)
			if wuzapiMsgData.Mimetype != "" { // Try to add extension from mimetype
				parts := strings.Split(wuzapiMsgData.Mimetype, "/")
				if len(parts) == 2 {
					fileName += "." + parts[1]
				}
			}
		}
		log.Info().Str("originalFileName", wuzapiMsgData.FileName).Str("derivedFileName", fileName).Msg("Filename was empty or cleaned, derived from URL/ID and mimetype")
	}


	// 2. Upload Media to Chatwoot
	chatwootAttachment, err := s.chatwootClient.UploadFile(mediaData, fileName, contentType)
	if err != nil {
		log.Error().Err(err).Str("fileName", fileName).Msg("Failed to upload media to Chatwoot")
		return fmt.Errorf("failed to upload media to Chatwoot (file: %s): %w", fileName, err)
	}
	log.Info().Int("chatwootAttachmentID", chatwootAttachment.ID).Str("fileName", fileName).Msg("Media uploaded to Chatwoot")

	// 3. Create Message in Chatwoot with the attachment
	caption := wuzapiMsgData.Caption
	if caption == "" {
		caption = wuzapiMsgData.Text
		if caption == "" {
			caption = wuzapiMsgData.Content
		}
	}
	if caption == "" && (wuzapiMsgData.Type == "voice" || wuzapiMsgData.Type == "audio") {
		caption = "Audio message" // Default caption for voice/audio if none provided
	}


	messagePayload := chatwoot.ChatwootMessagePayload{
		Content:     caption,
		MessageType: "incoming",
		ContentType: "input_file",
		Private:     false,
		SourceID:    wuzapiMsgData.ID,
		Attachments: []chatwoot.ChatwootAttachmentToken{{ID: chatwootAttachment.ID}},
	}

	createdMessage, err := s.chatwootClient.CreateMessage(int(conversationMap.ChatwootConversationID), messagePayload)
	if err != nil {
		log.Error().Err(err).
			Str("wuzapiMessageID", wuzapiMsgData.ID).
			Int("chatwootAttachmentID", chatwootAttachment.ID).
			Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
			Msg("Failed to create message with attachment in Chatwoot")
		return fmt.Errorf("failed to create message with attachment in Chatwoot for Wuzapi msg ID %s: %w", wuzapiMsgData.ID, err)
	}

	log.Info().
		Str("wuzapiMessageID", wuzapiMsgData.ID).
		Int("chatwootMessageID", createdMessage.ID).
		Int("chatwootAttachmentID", chatwootAttachment.ID).
		Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
		Msg("Successfully synced Wuzapi media message to Chatwoot")

	return nil
}
