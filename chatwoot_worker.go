package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types"
)

const (
	chatwootDirectionOutbound = "wa_to_cw"
	chatwootDirectionInbound  = "cw_to_wa"
	chatwootOutboxPending     = "pending"
	chatwootOutboxProcessing  = "processing"
	chatwootOutboxDone        = "done"
	chatwootOutboxDead        = "dead"
	chatwootMaxAttempts       = 8
	chatwootOutboxLease       = 5 * time.Minute
)

var chatwootWorkerPollInterval = time.Second

type chatwootOutboundMessage struct {
	InstanceID  string `json:"instance_id"`
	MessageID   string `json:"message_id"`
	ChatJID     string `json:"chat_jid"`
	ContactName string `json:"contact_name,omitempty"`
	Text        string `json:"text"`
	FromMe      bool   `json:"from_me"`
}

type chatwootInboundMessage struct {
	InstanceID        string `json:"instance_id"`
	ChatwootMessageID int64  `json:"chatwoot_message_id"`
	ConversationID    int64  `json:"conversation_id"`
	ChatJID           string `json:"chat_jid"`
	Text              string `json:"text"`
}

type chatwootOutboxRow struct {
	ID              int64     `db:"id"`
	InstanceID      string    `db:"instance_id"`
	SourceMessageID string    `db:"source_message_id"`
	Direction       string    `db:"direction"`
	Payload         []byte    `db:"payload"`
	Attempts        int       `db:"attempts"`
	CreatedAt       time.Time `db:"created_at"`
}

type chatwootContactInbox struct {
	InboxID int64 `json:"inbox_id"`
	Inbox   struct {
		ID int64 `json:"id"`
	} `json:"inbox"`
	SourceID string `json:"source_id"`
}

type chatwootContact struct {
	ID             int64                  `json:"id"`
	Identifier     string                 `json:"identifier"`
	PhoneNumber    string                 `json:"phone_number"`
	ContactInboxes []chatwootContactInbox `json:"contact_inboxes"`
}

func (s *server) enqueueChatwootOutbound(ctx context.Context, message chatwootOutboundMessage) error {
	jid, ok := parseJID(message.ChatJID)
	if !ok || requireDirectChat(jid) != nil || message.InstanceID == "" || message.MessageID == "" || strings.TrimSpace(message.Text) == "" {
		return nil
	}
	var sentFromChatwoot bool
	if err := s.db.GetContext(ctx, &sentFromChatwoot, s.db.Rebind(`SELECT EXISTS(SELECT 1 FROM chatwoot_message_map WHERE instance_id=? AND wa_message_id=? AND direction=?)`), message.InstanceID, message.MessageID, chatwootDirectionInbound); err != nil {
		return err
	}
	if sentFromChatwoot {
		return nil
	}
	message.ChatJID = jid.ToNonAD().String()
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	query := s.db.Rebind(`INSERT INTO chatwoot_outbox (instance_id, direction, state, lock_key, source_message_id, payload)
        SELECT ?, ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM chatwoot_config WHERE instance_id=? AND enabled=?)
        ON CONFLICT (instance_id, direction, source_message_id) DO NOTHING`)
	_, err = s.db.ExecContext(ctx, query, message.InstanceID, chatwootDirectionOutbound, chatwootOutboxPending,
		message.InstanceID+":"+message.ChatJID, message.MessageID, payload, message.InstanceID, true)
	return err
}

func (s *server) processNextChatwootOutbox(ctx context.Context) (bool, error) {
	row, err := s.claimChatwootOutbox(ctx)
	if errors.Is(err, errChatwootOutboxEmpty) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var deliveryErr error
	switch row.Direction {
	case chatwootDirectionOutbound:
		var message chatwootOutboundMessage
		if err := json.Unmarshal(row.Payload, &message); err != nil {
			deliveryErr = fmt.Errorf("invalid queued payload")
		} else {
			deliveryErr = s.deliverChatwootOutbound(ctx, message)
		}
	case chatwootDirectionInbound:
		var message chatwootInboundMessage
		if err := json.Unmarshal(row.Payload, &message); err != nil {
			deliveryErr = fmt.Errorf("invalid queued payload")
		} else {
			deliveryErr = s.deliverChatwootInbound(ctx, message)
		}
	default:
		deliveryErr = fmt.Errorf("invalid queued direction")
	}
	if deliveryErr != nil {
		return true, s.finishChatwootOutbox(row, deliveryErr)
	}
	return true, s.completeChatwootOutbox(row.ID)
}

func (s *server) startChatwootWorker(ctx context.Context) {
	workers := chatwootWorkerCount()
	for i := 0; i < workers; i++ {
		go func() {
			ticker := time.NewTicker(chatwootWorkerPollInterval)
			defer ticker.Stop()
			for {
				processed, err := s.processNextChatwootOutbox(ctx)
				if err != nil {
					log.Warn().Err(err).Msg("Chatwoot outbox processing failed")
				}
				if processed {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
}

func (s *server) ensureChatwootWorker() {
	if s.chatwootWorkerCtx == nil {
		return
	}
	var enabled bool
	if err := s.db.Get(&enabled, `SELECT EXISTS(SELECT 1 FROM chatwoot_config WHERE enabled=TRUE)`); err != nil || !enabled {
		return
	}
	s.chatwootWorkerOnce.Do(func() { s.startChatwootWorker(s.chatwootWorkerCtx) })
}

func chatwootWorkerCount() int {
	workers, err := strconv.Atoi(os.Getenv("WUZAPI_CHATWOOT_WORKERS"))
	if err != nil || workers < 1 {
		return 1
	}
	if workers > 8 {
		return 8
	}
	return workers
}

var errChatwootOutboxEmpty = errors.New("Chatwoot outbox is empty")

func (s *server) claimChatwootOutbox(ctx context.Context) (chatwootOutboxRow, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return chatwootOutboxRow{}, err
	}
	defer tx.Rollback()
	if err := s.recoverExpiredChatwootOutbox(ctx, tx); err != nil {
		return chatwootOutboxRow{}, err
	}
	lockSuffix := ""
	if s.db.DriverName() == "postgres" {
		lockSuffix = " FOR UPDATE SKIP LOCKED"
	}
	var candidates []chatwootOutboxRow
	query := s.db.Rebind(`SELECT id, instance_id, source_message_id, direction, payload, attempts, created_at
        FROM chatwoot_outbox WHERE state=? AND next_attempt_at <= CURRENT_TIMESTAMP ORDER BY id LIMIT 16` + lockSuffix)
	if err := tx.SelectContext(ctx, &candidates, query, chatwootOutboxPending); err != nil {
		return chatwootOutboxRow{}, err
	}
	if len(candidates) == 0 {
		return chatwootOutboxRow{}, errChatwootOutboxEmpty
	}
	for _, row := range candidates {
		var lockKey string
		if err := tx.GetContext(ctx, &lockKey, s.db.Rebind(`SELECT lock_key FROM chatwoot_outbox WHERE id=?`), row.ID); err != nil {
			return chatwootOutboxRow{}, err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`DELETE FROM chatwoot_outbox_locks WHERE lock_key=? AND acquired_at < ?`), lockKey, time.Now().UTC().Add(-chatwootOutboxLease)); err != nil {
			return chatwootOutboxRow{}, err
		}
		result, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO chatwoot_outbox_locks (lock_key, outbox_id) VALUES (?, ?) ON CONFLICT DO NOTHING`), lockKey, row.ID)
		if err != nil {
			return chatwootOutboxRow{}, err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return chatwootOutboxRow{}, err
		}
		if inserted == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE chatwoot_outbox SET state=?, attempts=attempts+1, locked_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?`), chatwootOutboxProcessing, row.ID); err != nil {
			return chatwootOutboxRow{}, err
		}
		row.Attempts++
		if err := tx.Commit(); err != nil {
			return chatwootOutboxRow{}, err
		}
		return row, nil
	}
	return chatwootOutboxRow{}, errChatwootOutboxEmpty
}

func (s *server) enqueueChatwootInbound(ctx context.Context, message chatwootInboundMessage) error {
	jid, ok := parseJID(message.ChatJID)
	if !ok || requireDirectChat(jid) != nil || message.InstanceID == "" || message.ChatwootMessageID <= 0 || strings.TrimSpace(message.Text) == "" {
		return nil
	}
	message.ChatJID = jid.ToNonAD().String()
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	query := s.db.Rebind(`INSERT INTO chatwoot_outbox (instance_id, direction, state, lock_key, source_message_id, payload)
        SELECT ?, ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM chatwoot_config WHERE instance_id=? AND enabled=?)
        ON CONFLICT (instance_id, direction, source_message_id) DO NOTHING`)
	_, err = s.db.ExecContext(ctx, query, message.InstanceID, chatwootDirectionInbound, chatwootOutboxPending,
		message.InstanceID+":"+message.ChatJID, strconv.FormatInt(message.ChatwootMessageID, 10), payload, message.InstanceID, true)
	return err
}

func deterministicChatwootWhatsAppID(chatwootMessageID int64) string {
	sum := sha256.Sum256([]byte("chatwoot:" + strconv.FormatInt(chatwootMessageID, 10)))
	return strings.ToUpper(fmt.Sprintf("%x", sum[:]))[:32]
}

func (s *server) deliverChatwootInbound(ctx context.Context, message chatwootInboundMessage) error {
	jid, ok := parseJID(message.ChatJID)
	if !ok {
		return errors.New("mapped Chatwoot destination is invalid")
	}
	if err := requireDirectChat(jid); err != nil {
		return err
	}
	waMessageID := deterministicChatwootWhatsAppID(message.ChatwootMessageID)
	mapQuery := s.db.Rebind(`INSERT INTO chatwoot_message_map (instance_id, wa_message_id, chatwoot_message_id, conversation_id, direction)
        VALUES (?, ?, ?, ?, ?) ON CONFLICT (instance_id, wa_message_id) DO NOTHING`)
	if _, err := s.db.Exec(mapQuery, message.InstanceID, waMessageID, message.ChatwootMessageID, message.ConversationID, chatwootDirectionInbound); err != nil {
		return err
	}
	var token string
	if err := s.db.Get(&token, s.db.Rebind(`SELECT token FROM users WHERE id=?`), message.InstanceID); err != nil {
		return errors.New("Chatwoot instance no longer exists")
	}
	body, err := json.Marshal(map[string]string{"Phone": jid.String(), "Body": message.Text, "Id": waMessageID})
	if err != nil {
		return err
	}
	request := httptest.NewRequest(http.MethodPost, "/chat/send/text", bytes.NewReader(body))
	request.Header.Set("Token", token)
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, request)
	if recorder.Code < http.StatusOK || recorder.Code >= http.StatusMultipleChoices {
		return fmt.Errorf("WhatsApp send failed with HTTP %d", recorder.Code)
	}
	return nil
}

func (s *server) recoverExpiredChatwootOutbox(ctx context.Context, tx sqlExecer) error {
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE chatwoot_outbox SET state=?, locked_at=NULL, updated_at=CURRENT_TIMESTAMP
        WHERE state=? AND locked_at < ?`), chatwootOutboxPending, chatwootOutboxProcessing, time.Now().UTC().Add(-chatwootOutboxLease)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM chatwoot_outbox_locks WHERE outbox_id NOT IN (SELECT id FROM chatwoot_outbox WHERE state = 'processing')`)
	return err
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *server) finishChatwootOutbox(row chatwootOutboxRow, deliveryErr error) error {
	state := chatwootOutboxPending
	nextAttempt := time.Now().UTC().Add(chatwootRetryDelay(row.Attempts))
	if row.Attempts >= chatwootMaxAttempts {
		state = chatwootOutboxDead
	}
	message := sanitizeChatwootError(deliveryErr)
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(s.db.Rebind(`UPDATE chatwoot_outbox SET state=?, next_attempt_at=?, locked_at=NULL, last_error=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`), state, nextAttempt, message, row.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(s.db.Rebind(`DELETE FROM chatwoot_outbox_locks WHERE outbox_id=?`), row.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *server) completeChatwootOutbox(outboxID int64) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(s.db.Rebind(`UPDATE chatwoot_outbox SET state=?, locked_at=NULL, updated_at=CURRENT_TIMESTAMP, last_error='' WHERE id=?`), chatwootOutboxDone, outboxID); err != nil {
		return err
	}
	if _, err := tx.Exec(s.db.Rebind(`DELETE FROM chatwoot_outbox_locks WHERE outbox_id=?`), outboxID); err != nil {
		return err
	}
	return tx.Commit()
}

func chatwootRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second * time.Duration(1<<min(attempt-1, 8))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	delay += time.Duration(rand.Int63n(int64(delay/2) + 1))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func (s *server) deliverChatwootOutbound(ctx context.Context, message chatwootOutboundMessage) error {
	var mapped bool
	if err := s.db.Get(&mapped, s.db.Rebind(`SELECT EXISTS(SELECT 1 FROM chatwoot_message_map WHERE instance_id=? AND wa_message_id=?)`), message.InstanceID, message.MessageID); err != nil {
		return err
	}
	if mapped {
		return nil
	}
	var config chatwootConfigRecord
	if err := s.db.Get(&config, s.db.Rebind(`SELECT enabled, url, account_id, token, inbox_id, inbox_name FROM chatwoot_config WHERE instance_id=?`), message.InstanceID); err != nil || !config.Enabled {
		return errors.New("Chatwoot is not enabled for this instance")
	}
	token, err := decryptHMACKey(config.Token)
	if err != nil {
		return errors.New("could not decrypt Chatwoot credentials")
	}
	client := newChatwootClient(config.URL, config.AccountID, token)
	contactID, sourceID, conversationID, err := s.ensureChatwootConversation(ctx, client, config.InboxID, message)
	if err != nil {
		return err
	}
	messageType := "incoming"
	if message.FromMe {
		messageType = "outgoing"
	}
	chatwootMessageID, err := client.createConversationMessage(ctx, conversationID, message.Text, messageType, "WAID:"+message.MessageID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_message_map (instance_id, wa_message_id, chatwoot_message_id, conversation_id, direction) VALUES (?, ?, ?, ?, ?)`), message.InstanceID, message.MessageID, chatwootMessageID, conversationID, chatwootDirectionOutbound)
	if err != nil {
		return err
	}
	_ = contactID
	_ = sourceID
	return nil
}

func (s *server) ensureChatwootConversation(ctx context.Context, client *chatwootClient, inboxID int64, message chatwootOutboundMessage) (int64, string, int64, error) {
	var existing struct {
		ContactID      int64  `db:"contact_id"`
		SourceID       string `db:"source_id"`
		ConversationID int64  `db:"conversation_id"`
	}
	err := s.db.Get(&existing, s.db.Rebind(`SELECT contact_id, source_id, conversation_id FROM chatwoot_contact_map WHERE instance_id=? AND chat_jid=?`), message.InstanceID, message.ChatJID)
	if err == nil && existing.ConversationID > 0 {
		return existing.ContactID, existing.SourceID, existing.ConversationID, nil
	}
	contactID, sourceID, err := client.findContact(ctx, inboxID, message)
	if err != nil {
		return 0, "", 0, err
	}
	if contactID == 0 {
		contactID, err = client.createContact(ctx, inboxID, message)
		if err != nil {
			return 0, "", 0, err
		}
	}
	if sourceID == "" {
		sourceID = "wuzapi:" + message.ChatJID
		if err := client.createContactInbox(ctx, contactID, inboxID, sourceID); err != nil {
			return 0, "", 0, err
		}
	}
	if conversationID, err := client.findReusableConversation(ctx, contactID, inboxID); err != nil {
		return 0, "", 0, err
	} else if conversationID > 0 {
		if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_contact_map (instance_id, chat_jid, contact_id, source_id, conversation_id, updated_at)
            VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
            ON CONFLICT (instance_id, chat_jid) DO UPDATE SET contact_id=EXCLUDED.contact_id, source_id=EXCLUDED.source_id, conversation_id=EXCLUDED.conversation_id, updated_at=CURRENT_TIMESTAMP`), message.InstanceID, message.ChatJID, contactID, sourceID, conversationID); err != nil {
			return 0, "", 0, err
		}
		return contactID, sourceID, conversationID, nil
	}
	conversationID, err := client.createConversation(ctx, contactID, inboxID, sourceID)
	if err != nil {
		return 0, "", 0, err
	}
	_, err = s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_contact_map (instance_id, chat_jid, contact_id, source_id, conversation_id, updated_at)
        VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT (instance_id, chat_jid) DO UPDATE SET contact_id=EXCLUDED.contact_id, source_id=EXCLUDED.source_id, conversation_id=EXCLUDED.conversation_id, updated_at=CURRENT_TIMESTAMP`), message.InstanceID, message.ChatJID, contactID, sourceID, conversationID)
	if err != nil {
		return 0, "", 0, err
	}
	return contactID, sourceID, conversationID, nil
}

func (c *chatwootClient) findContact(ctx context.Context, inboxID int64, message chatwootOutboundMessage) (int64, string, error) {
	identifier := "wuzapi:" + message.ChatJID
	contacts, err := c.searchContacts(ctx, identifier)
	if err != nil {
		return 0, "", err
	}
	jid, _ := parseJID(message.ChatJID)
	if len(contacts) == 0 && jid.Server == types.DefaultUserServer {
		contacts, err = c.searchContacts(ctx, "+"+jid.User)
		if err != nil {
			return 0, "", err
		}
	}
	for _, contact := range contacts {
		if contact.Identifier != identifier && !(jid.Server == types.DefaultUserServer && contact.PhoneNumber == "+"+jid.User) {
			continue
		}
		if contact.ID == 0 {
			continue
		}
		for _, inbox := range contact.ContactInboxes {
			if (inbox.InboxID == inboxID || inbox.Inbox.ID == inboxID) && inbox.SourceID != "" {
				return contact.ID, inbox.SourceID, nil
			}
		}
		return contact.ID, "", nil
	}
	return 0, "", nil
}

func (c *chatwootClient) searchContacts(ctx context.Context, query string) ([]chatwootContact, error) {
	var response struct {
		Payload []chatwootContact `json:"payload"`
	}
	path := "/api/v1/accounts/" + strconv.FormatInt(c.accountID, 10) + "/contacts/search?q=" + url.QueryEscape(query)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Payload, nil
}

func (c *chatwootClient) findReusableConversation(ctx context.Context, contactID, inboxID int64) (int64, error) {
	var response struct {
		Payload []struct {
			ID      int64  `json:"id"`
			InboxID int64  `json:"inbox_id"`
			Status  string `json:"status"`
		} `json:"payload"`
	}
	path := "/api/v1/accounts/" + strconv.FormatInt(c.accountID, 10) + "/contacts/" + strconv.FormatInt(contactID, 10) + "/conversations"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		var httpErr chatwootHTTPError
		if errors.As(err, &httpErr) && httpErr.Status() == http.StatusNotFound {
			return 0, nil
		}
		return 0, err
	}
	for _, conversation := range response.Payload {
		if conversation.ID > 0 && conversation.InboxID == inboxID && (conversation.Status == "open" || conversation.Status == "pending") {
			return conversation.ID, nil
		}
	}
	return 0, nil
}

func (c *chatwootClient) createContact(ctx context.Context, inboxID int64, message chatwootOutboundMessage) (int64, error) {
	var response struct {
		Payload []struct {
			ID int64 `json:"id"`
		} `json:"payload"`
	}
	jid, _ := parseJID(message.ChatJID)
	payload := map[string]any{"inbox_id": inboxID, "identifier": "wuzapi:" + message.ChatJID, "name": message.ContactName}
	if payload["name"] == "" {
		payload["name"] = jid.User
	}
	if jid.Server == types.DefaultUserServer {
		payload["phone_number"] = "+" + jid.User
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(c.accountID, 10)+"/contacts", payload, &response); err != nil {
		return 0, err
	}
	if len(response.Payload) == 0 || response.Payload[0].ID <= 0 {
		return 0, errors.New("Chatwoot did not return a contact id")
	}
	return response.Payload[0].ID, nil
}

func (c *chatwootClient) createContactInbox(ctx context.Context, contactID, inboxID int64, sourceID string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(c.accountID, 10)+"/contacts/"+strconv.FormatInt(contactID, 10)+"/contact_inboxes", map[string]any{"inbox_id": inboxID, "source_id": sourceID}, nil)
}

func (c *chatwootClient) createConversation(ctx context.Context, contactID, inboxID int64, sourceID string) (int64, error) {
	var response struct {
		ID int64 `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(c.accountID, 10)+"/conversations", map[string]any{"source_id": sourceID, "inbox_id": inboxID, "contact_id": contactID, "status": "open"}, &response); err != nil {
		return 0, err
	}
	if response.ID <= 0 {
		return 0, errors.New("Chatwoot did not return a conversation id")
	}
	return response.ID, nil
}

func (c *chatwootClient) createConversationMessage(ctx context.Context, conversationID int64, content, messageType, sourceID string) (int64, error) {
	var response struct {
		ID int64 `json:"id"`
	}
	payload := map[string]any{"content": content, "message_type": messageType, "private": false, "source_id": sourceID, "content_attributes": map[string]bool{"wuzapi_source": true}}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(c.accountID, 10)+"/conversations/"+strconv.FormatInt(conversationID, 10)+"/messages", payload, &response); err != nil {
		return 0, err
	}
	if response.ID <= 0 {
		return 0, errors.New("Chatwoot did not return a message id")
	}
	return response.ID, nil
}

func sanitizeChatwootError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 240 {
		message = message[:240]
	}
	return strings.ReplaceAll(message, "\n", " ")
}
