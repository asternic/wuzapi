package services

import (
	"fmt"
	"strconv"
	"wuzapi-chatwoot-integration/internal/adapters/chatwoot"

	"github.com/rs/zerolog/log"
)

// ContactSyncService handles the logic for synchronizing contacts between Wuzapi and Chatwoot.
type ContactSyncService struct {
	chatwootClient  *chatwoot.Client
	chatwootInboxID int
}

// NewContactSyncService creates a new ContactSyncService.
func NewContactSyncService(cwClient *chatwoot.Client, inboxIDStr string) (*ContactSyncService, error) {
	if cwClient == nil {
		return nil, fmt.Errorf("Chatwoot client cannot be nil")
	}
	if inboxIDStr == "" {
		return nil, fmt.Errorf("Chatwoot inbox ID string cannot be empty")
	}

	inboxID, err := strconv.Atoi(inboxIDStr)
	if err != nil {
		log.Error().Err(err).Str("inboxIDStr", inboxIDStr).Msg("Failed to convert Chatwoot Inbox ID string to int")
		return nil, fmt.Errorf("failed to convert Chatwoot Inbox ID '%s' to int: %w", inboxIDStr, err)
	}

	return &ContactSyncService{
		chatwootClient:  cwClient,
		chatwootInboxID: inboxID,
	}, nil
}

// FindOrCreateContactFromWuzapi attempts to find an existing Chatwoot contact by the Wuzapi sender's phone number.
// If not found, it creates a new contact in Chatwoot.
func (s *ContactSyncService) FindOrCreateContactFromWuzapi(wuzapiSenderPhone, wuzapiSenderName string) (*chatwoot.ChatwootContact, error) {
	log.Info().Str("phoneNumber", wuzapiSenderPhone).Str("name", wuzapiSenderName).Msg("Attempting to find or create Chatwoot contact")

	// Normalize phone number if necessary (e.g., ensure E.164 format)
	// For now, assume wuzapiSenderPhone is in a consistent format Chatwoot expects.

	contact, err := s.chatwootClient.GetContactByPhone(wuzapiSenderPhone)
	if err != nil {
		// Don't treat "not found" from GetContactByPhone as a fatal error for this service's logic,
		// as we intend to create the contact if it's not found.
		// The client's GetContactByPhone already logs detailed errors.
		log.Warn().Err(err).Str("phoneNumber", wuzapiSenderPhone).Msg("Error trying to get contact by phone, will attempt to create.")
		// Proceed to create, err from GetContactByPhone might indicate a problem beyond just "not found"
	}

	if contact != nil {
		log.Info().Int("contactID", contact.ID).Str("phoneNumber", wuzapiSenderPhone).Msg("Found existing Chatwoot contact")
		return contact, nil
	}

	// Contact not found, or an error occurred that didn't prevent creation attempt. Let's create one.
	log.Info().Str("phoneNumber", wuzapiSenderPhone).Str("name", wuzapiSenderName).Msg("Chatwoot contact not found, creating a new one.")

	payload := chatwoot.ChatwootContactPayload{
		InboxID:     s.chatwootInboxID,
		PhoneNumber: wuzapiSenderPhone,
		Name:        wuzapiSenderName, // Chatwoot will use phone number as name if name is empty
	}

	if wuzapiSenderName == "" {
		log.Info().Str("phoneNumber", wuzapiSenderPhone).Msg("Wuzapi sender name is empty, Chatwoot will likely use phone number as name.")
	}

	newContact, createErr := s.chatwootClient.CreateContact(payload)
	if createErr != nil {
		log.Error().Err(createErr).Str("phoneNumber", wuzapiSenderPhone).Msg("Failed to create Chatwoot contact")
		return nil, fmt.Errorf("failed to create Chatwoot contact for %s: %w", wuzapiSenderPhone, createErr)
	}

	log.Info().Int("contactID", newContact.ID).Str("phoneNumber", newContact.PhoneNumber).Msg("Successfully created new Chatwoot contact")
	return newContact, nil
}
