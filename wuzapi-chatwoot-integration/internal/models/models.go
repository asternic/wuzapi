package models

import (
	"time"
)

// ConversationMap maps a Wuzapi sender ID (e.g., phone number) to Chatwoot contact and conversation IDs.
// This helps in quickly finding existing Chatwoot conversations for incoming Wuzapi messages.
type ConversationMap struct {
	ID        uint      `gorm:"primaryKey"`
	WuzapiSenderID string    `gorm:"uniqueIndex;comment:Identifier for the sender from Wuzapi, e.g., phone number"`
	ChatwootContactID uint   `gorm:"comment:ID of the contact in Chatwoot"`
	ChatwootConversationID uint `gorm:"uniqueIndex;comment:ID of the conversation in Chatwoot"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// QueuedMessage represents a message that needs to be sent to either Wuzapi or Chatwoot.
// It's used for reliable message delivery, allowing for retries.
type QueuedMessage struct {
	ID        uint      `gorm:"primaryKey"`
	WuzapiMessageID string    `gorm:"index;comment:ID from Wuzapi, if message originated from/sent to Wuzapi"`
	ChatwootMessageID uint   `gorm:"index;comment:ID from Chatwoot, if message originated from/sent to Chatwoot"`
	Direction string    `gorm:"comment:Direction of sync, e.g., 'wuzapi-to-chatwoot' or 'chatwoot-to-wuzapi'"`
	Payload   string    `gorm:"type:text;comment:JSON payload of the message to be sent/retried"`
	RetryCount int      `gorm:"default:0;comment:Number of times delivery has been attempted"`
	LastError string    `gorm:"type:text;comment:Last error message encountered during delivery attempt"`
	Status    string    `gorm:"index;comment:Current status, e.g., pending, failed, success, processing"`
	Source    string    `gorm:"comment:The system that originated the event, e.g., wuzapi, chatwoot"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	NextRetryAt time.Time `gorm:"index;comment:Scheduled time for the next retry attempt"`
}
