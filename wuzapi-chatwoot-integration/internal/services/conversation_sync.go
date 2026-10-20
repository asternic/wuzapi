package services

import (
	"errors"
	"fmt"
	"strconv"
	"wuzapi-chatwoot-integration/internal/adapters/chatwoot"
	"wuzapi-chatwoot-integration/internal/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ConversationSyncService handles finding or creating Chatwoot conversations and mapping them.
type ConversationSyncService struct {
	chatwootClient  *chatwoot.Client
	db              *gorm.DB
	chatwootInboxID int
}

// NewConversationSyncService creates a new ConversationSyncService.
func NewConversationSyncService(cwClient *chatwoot.Client, db *gorm.DB, inboxIDStr string) (*ConversationSyncService, error) {
	if cwClient == nil {
		return nil, fmt.Errorf("Chatwoot client cannot be nil")
	}
	if db == nil {
		return nil, fmt.Errorf("database instance (gorm.DB) cannot be nil")
	}
	if inboxIDStr == "" {
		return nil, fmt.Errorf("Chatwoot inbox ID string cannot be empty")
	}

	inboxID, err := strconv.Atoi(inboxIDStr)
	if err != nil {
		log.Error().Err(err).Str("inboxIDStr", inboxIDStr).Msg("Failed to convert Chatwoot Inbox ID string to int for ConversationSyncService")
		return nil, fmt.Errorf("failed to convert Chatwoot Inbox ID '%s' to int: %w", inboxIDStr, err)
	}

	return &ConversationSyncService{
		chatwootClient:  cwClient,
		db:              db,
		chatwootInboxID: inboxID,
	}, nil
}

// FindOrCreateConversation finds an existing Chatwoot conversation for a Wuzapi sender
// or creates a new one if none suitable is found. It also maintains a local mapping in the DB.
func (s *ConversationSyncService) FindOrCreateConversation(wuzapiSenderID string, chatwootContact *chatwoot.ChatwootContact) (*models.ConversationMap, error) {
	log.Info().Str("wuzapiSenderID", wuzapiSenderID).Int("chatwootContactID", chatwootContact.ID).Msg("Finding or creating Chatwoot conversation")

	// 1. Check DB Cache First
	var conversationMap models.ConversationMap
	err := s.db.Where("wuzapi_sender_id = ?", wuzapiSenderID).First(&conversationMap).Error
	if err == nil {
		// Found in DB
		log.Info().
			Str("wuzapiSenderID", wuzapiSenderID).
			Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
			Msg("Conversation map found in DB cache")
		return &conversationMap, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error().Err(err).Str("wuzapiSenderID", wuzapiSenderID).Msg("Error querying ConversationMap from DB")
		return nil, fmt.Errorf("error querying ConversationMap: %w", err)
	}
	// Record not found, proceed to check Chatwoot

	// 2. Check Chatwoot for Existing Conversations for this contact
	log.Info().Int("chatwootContactID", chatwootContact.ID).Msg("Checking Chatwoot for existing conversations for contact")
	conversations, err := s.chatwootClient.GetConversationsForContact(chatwootContact.ID)
	if err != nil {
		log.Error().Err(err).Int("chatwootContactID", chatwootContact.ID).Msg("Failed to get conversations for contact from Chatwoot")
		// Depending on the error, we might still try to create a new one or return.
		// If it's a transient error, retrying might be an option. For now, proceed to create.
	}

	if err == nil { // Only proceed if GetConversationsForContact didn't error out
		for _, conv := range conversations {
			// We need a way to identify if this conversation is the "right" one.
			// For Wuzapi, a contact usually has one main conversation per inbox.
			// If SourceID was set on conversation creation, we could check that.
			// For now, let's assume any existing open conversation in the target inbox is usable.
			// A more robust check might involve looking for conversations with a specific source_id
			// or specific custom attributes if Wuzapi integration sets them.
			if conv.InboxID == s.chatwootInboxID && (conv.Status == "open" || conv.Status == "pending") {
				log.Info().
					Int("chatwootConversationID", conv.ID).
					Int("chatwootContactID", chatwootContact.ID).
					Str("wuzapiSenderID", wuzapiSenderID).
					Msg("Found suitable existing Chatwoot conversation for contact in the correct inbox")

				return s.storeConversationMap(wuzapiSenderID, chatwootContact.ID, uint(conv.ID))
			}
		}
	}


	// 3. Create New Conversation in Chatwoot
	log.Info().Str("wuzapiSenderID", wuzapiSenderID).Int("chatwootContactID", chatwootContact.ID).Msg("No suitable existing conversation found, creating new one in Chatwoot")
	payload := chatwoot.ChatwootConversationPayload{
		SourceID:  wuzapiSenderID, // Use Wuzapi sender ID as source_id for traceability
		InboxID:   s.chatwootInboxID,
		ContactID: chatwootContact.ID,
		Status:    "open", // Default to open status
	}

	newConv, err := s.chatwootClient.CreateConversation(payload)
	if err != nil {
		log.Error().Err(err).Str("wuzapiSenderID", wuzapiSenderID).Msg("Failed to create new conversation in Chatwoot")
		return nil, fmt.Errorf("failed to create Chatwoot conversation: %w", err)
	}

	log.Info().
		Int("newChatwootConversationID", newConv.ID).
		Str("wuzapiSenderID", wuzapiSenderID).
		Msg("Successfully created new Chatwoot conversation")

	return s.storeConversationMap(wuzapiSenderID, chatwootContact.ID, uint(newConv.ID))
}

// storeConversationMap saves the mapping to the database.
func (s *ConversationSyncService) storeConversationMap(wuzapiSenderID string, chatwootContactID int, chatwootConversationID uint) (*models.ConversationMap, error) {
	cm := models.ConversationMap{
		WuzapiSenderID:         wuzapiSenderID,
		ChatwootContactID:      uint(chatwootContactID),
		ChatwootConversationID: chatwootConversationID,
	}
	if err := s.db.Create(&cm).Error; err != nil {
		log.Error().Err(err).
			Str("wuzapiSenderID", wuzapiSenderID).
			Int("chatwootContactID", chatwootContactID).
			Uint("chatwootConversationID", chatwootConversationID).
			Msg("Failed to save ConversationMap to DB")
		return nil, fmt.Errorf("failed to save ConversationMap: %w", err)
	}
	log.Info().Str("wuzapiSenderID", cm.WuzapiSenderID).Uint("chatwootConversationID", cm.ChatwootConversationID).Msg("Conversation map stored in DB")
	return &cm, nil
}
