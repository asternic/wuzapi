package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	// "wuzapi-chatwoot-integration/config" // No longer needed for direct access
	"wuzapi-chatwoot-integration/internal/adapters/wuzapi" // Import for WuzapiEventPayload
	"wuzapi-chatwoot-integration/internal/services"       // Import for ContactSyncService

	"github.com/rs/zerolog/log"
)

// WuzapiHandler is a struct that holds dependencies for Wuzapi webhook processing.
type WuzapiHandler struct {
	contactService      *services.ContactSyncService
	conversationService *services.ConversationSyncService
	messageService      *services.MessageSyncService
	webhookSecret       string
}

// NewWuzapiHandler creates a new WuzapiHandler with necessary dependencies.
func NewWuzapiHandler(
	contactService *services.ContactSyncService,
	conversationService *services.ConversationSyncService,
	messageService *services.MessageSyncService,
	secret string,
) *WuzapiHandler {
	if contactService == nil {
		log.Fatal().Msg("ContactSyncService cannot be nil for WuzapiHandler")
	}
	if conversationService == nil {
		log.Fatal().Msg("ConversationSyncService cannot be nil for WuzapiHandler")
	}
	if messageService == nil {
		log.Fatal().Msg("MessageSyncService cannot be nil for WuzapiHandler")
	}
	return &WuzapiHandler{
		contactService:      contactService,
		conversationService: conversationService,
		messageService:      messageService,
		webhookSecret:       secret,
	}
}

// isValidSignature (Placeholder - can be a method of WuzapiHandler or a standalone utility)
// For now, keeping it similar to before but using h.webhookSecret.
func (h *WuzapiHandler) isValidSignature(body []byte, signature string) bool {
	if h.webhookSecret == "" {
		log.Warn().Msg("Webhook secret is not configured in WuzapiHandler. Skipping signature validation.")
		return true // Or false, depending on desired behavior
	}
	if signature == "" {
		log.Warn().Msg("No signature provided in X-Wuzapi-Signature header.")
		return false
	}
	// TODO: Implement actual HMAC SHA256 validation.
	if h.webhookSecret == "dev-secret" || signature == "dev-signature" {
		log.Warn().Msg("Using DEV signature validation. NOT FOR PRODUCTION.")
		return true
	}
	log.Warn().Str("signature", signature).Msg("Signature validation failed (placeholder logic).")
	return false
}

// Handle processes incoming webhooks from Wuzapi.
func (h *WuzapiHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// 1. Signature Validation
	signature := r.Header.Get("X-Wuzapi-Signature")

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body

	if !h.isValidSignature(bodyBytes, signature) {
		log.Warn().Msg("Invalid webhook signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// 2. Request Body Parsing
	var eventPayload wuzapi.WuzapiEventPayload
	if err := json.NewDecoder(r.Body).Decode(&eventPayload); err != nil {
		log.Error().Err(err).Msg("Failed to decode JSON request body into WuzapiEventPayload")
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// 3. Logging
	// Determine the primary event type string
	primaryEventType := eventPayload.Event
	if primaryEventType == "" {
		primaryEventType = eventPayload.Type
	}

	log.Info().Str("eventType", primaryEventType).Str("instanceID", eventPayload.InstanceID).Msg("Received Wuzapi event")
	log.Debug().Interface("payload", eventPayload).Msg("Wuzapi event payload")

	// 4. Basic Event Processing (Dispatch Placeholder)
	if strings.TrimSpace(primaryEventType) == "" {
		log.Warn().Interface("payload", eventPayload).Msg("Event type is empty or not found in Wuzapi payload.")
		w.WriteHeader(http.StatusOK)
		return
	}

	switch primaryEventType {
	case "message.received", "message:received", "message_received":
		log.Info().Str("eventType", primaryEventType).Msg("Processing Wuzapi incoming message event")

		if eventPayload.Message == nil {
			log.Error().Interface("payload", eventPayload).Msg("Wuzapi message.received event has no 'message' data")
			// Acknowledge to prevent retries, but this is an unexpected payload structure
			w.WriteHeader(http.StatusOK)
			return
		}

		senderPhone := eventPayload.Message.From
		senderName := eventPayload.Message.SenderName
		if senderName == "" { // Fallback if SenderName is not provided, try PushName
			senderName = eventPayload.Message.PushName
		}

		if senderPhone == "" {
			log.Error().Interface("messagePayload", eventPayload.Message).Msg("Failed to extract sender phone number from Wuzapi message.received event")
			// Depending on requirements, might send http.StatusBadRequest or just acknowledge
		} else {
			contact, err := h.contactService.FindOrCreateContactFromWuzapi(senderPhone, senderName)
			if err != nil {
				log.Error().Err(err).Str("senderPhone", senderPhone).Msg("Error finding or creating contact from Wuzapi event")
				// Acknowledge to prevent Wuzapi retries for this specific error path
				w.WriteHeader(http.StatusOK)
				return
			}

			log.Info().Int("chatwootContactID", contact.ID).Str("senderPhone", senderPhone).Msg("Successfully found/created Chatwoot contact for Wuzapi message")

			// Now, find or create the conversation
			conversationMap, err := h.conversationService.FindOrCreateConversation(senderPhone, contact)
			if err != nil {
				log.Error().Err(err).Str("senderPhone", senderPhone).Int("chatwootContactID", contact.ID).Msg("Error finding or creating conversation")
				// Acknowledge to prevent Wuzapi retries
				w.WriteHeader(http.StatusOK)
				return
			}
			log.Info().
				Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
				Str("senderPhone", senderPhone).
				Msg("Successfully ensured conversation exists and is mapped")

			// Sync the message content
			wuzapiMsgData := eventPayload.Message
			isText := wuzapiMsgData.Type == "text" || wuzapiMsgData.Type == "chat" || (wuzapiMsgData.Text != "" && wuzapiMsgData.MediaURL == "")
			isMedia := wuzapiMsgData.MediaURL != "" && (wuzapiMsgData.Type == "image" || wuzapiMsgData.Type == "video" || wuzapiMsgData.Type == "audio" || wuzapiMsgData.Type == "document" || wuzapiMsgData.Type == "sticker")

			if isText {
				err = h.messageService.SyncWuzapiTextMessageToChatwoot(conversationMap, wuzapiMsgData)
				if err != nil {
					log.Error().Err(err).
						Str("wuzapiMessageID", wuzapiMsgData.ID).
						Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
						Msg("Error syncing Wuzapi text message to Chatwoot")
					// Error already logged by service, acknowledge to Wuzapi
				} else {
					log.Info().
						Str("wuzapiMessageID", wuzapiMsgData.ID).
						Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
						Msg("Successfully initiated sync of Wuzapi text message to Chatwoot")
				}
			} else if isMedia {
				err = h.messageService.SyncWuzapiMediaMessageToChatwoot(conversationMap, wuzapiMsgData)
				if err != nil {
					log.Error().Err(err).
						Str("wuzapiMessageID", wuzapiMsgData.ID).
						Str("mediaURL", wuzapiMsgData.MediaURL).
						Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
						Msg("Error syncing Wuzapi media message to Chatwoot")
					// Error already logged by service, acknowledge to Wuzapi
				} else {
					log.Info().
						Str("wuzapiMessageID", wuzapiMsgData.ID).
						Uint("chatwootConversationID", conversationMap.ChatwootConversationID).
						Msg("Successfully initiated sync of Wuzapi media message to Chatwoot")
				}
			} else {
				log.Info().
					Str("wuzapiMessageID", wuzapiMsgData.ID).
					Str("wuzapiMessageType", wuzapiMsgData.Type).
					Msg("Wuzapi message is not a simple text or known media type, skipping sync.")
			}
		}

	case "message.sent", "message:sent", "message_sent":
		log.Info().Str("eventType", primaryEventType).Msg("Processing Wuzapi message sent event")
		// Placeholder: Call statusUpdateService.HandleWuzapiMessageSent(eventPayload)
	case "message.delivered", "message:delivered", "message_delivered":
		log.Info().Str("eventType", primaryEventType).Msg("Processing Wuzapi message delivered event")
		// Placeholder: Call statusUpdateService.HandleWuzapiMessageDelivered(eventPayload)
	case "message.read", "message:read", "message_read":
		log.Info().Str("eventType", primaryEventType).Msg("Processing Wuzapi message read event")
	case "instance.status", "instance:status", "instance_status":
		log.Info().Str("eventType", primaryEventType).Msg("Processing Wuzapi instance status event")
		// Placeholder: Call instanceStateService.HandleWuzapiStatusUpdate(eventPayload)
	default:
		log.Warn().Str("eventType", primaryEventType).Msg("Received unknown Wuzapi event type")
	}

	// 5. Respond to Wuzapi
	w.WriteHeader(http.StatusOK) // Acknowledge receipt
	// _, _ = w.Write([]byte("Acknowledged")) // Optional response body
}
