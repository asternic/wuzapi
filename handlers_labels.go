package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

const labelAppStateTimeout = 60 * time.Second

type labelReadModel struct {
	LabelID string   `json:"labelId"`
	Name    string   `json:"name"`
	Color   int32    `json:"color"`
	Deleted bool     `json:"deleted"`
	Chats   []string `json:"chats"`
}

type persistedLabel struct {
	LabelID string `db:"label_id"`
	Name    string `db:"name"`
	Color   int32  `db:"color"`
	Deleted bool   `db:"deleted"`
}

type persistedLabelChat struct {
	LabelID string `db:"label_id"`
	ChatJID string `db:"chat_jid"`
}

func (s *server) EditLabel() http.HandlerFunc {
	type editLabelRequest struct {
		LabelID string  `json:"labelId"`
		Name    *string `json:"name"`
		Color   *int32  `json:"color"`
		Deleted *bool   `json:"deleted"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(userID)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		var request editLabelRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		request.LabelID = strings.TrimSpace(request.LabelID)
		if request.Name != nil {
			name := strings.TrimSpace(*request.Name)
			request.Name = &name
		}
		if request.LabelID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing labelId in Payload"))
			return
		}

		existingLabel, err := s.getLabel(userID, request.LabelID)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		name, color, deleted := resolvedLabelState(existingLabel, request.Name, request.Color, request.Deleted)
		if !deleted && name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing name in Payload"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), labelAppStateTimeout)
		defer cancel()

		if err := sendAppStatePatch(ctx, client, func() appstate.PatchInfo {
			return appstate.BuildLabelEdit(request.LabelID, name, color, deleted)
		}); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to edit label: %w", err))
			return
		}

		if err := s.saveLabel(userID, request.LabelID, name, color, deleted, marshalLabelDataJSON(request)); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		response := map[string]interface{}{
			"success": true,
			"labelId": request.LabelID,
			"name":    name,
			"color":   color,
			"deleted": deleted,
		}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(responseJSON))
	}
}

func (s *server) LabelChat() http.HandlerFunc {
	type labelChatRequest struct {
		JID     string `json:"jid"`
		LabelID string `json:"labelId"`
		Labeled bool   `json:"labeled"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(userID)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		var request labelChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		request.JID = strings.TrimSpace(request.JID)
		request.LabelID = strings.TrimSpace(request.LabelID)
		if request.JID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing jid in Payload"))
			return
		}
		if request.LabelID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing labelId in Payload"))
			return
		}

		chatJID, err := types.ParseJID(request.JID)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid jid format"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), labelAppStateTimeout)
		defer cancel()

		if err := sendAppStatePatch(ctx, client, func() appstate.PatchInfo {
			return appstate.BuildLabelChat(chatJID, request.LabelID, request.Labeled)
		}); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to update chat label: %w", err))
			return
		}

		if err := s.saveLabelChatAssociation(userID, request.LabelID, chatJID.String(), request.Labeled); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		response := map[string]interface{}{
			"success": true,
			"jid":     chatJID.String(),
			"labelId": request.LabelID,
			"labeled": request.Labeled,
		}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(responseJSON))
	}
}

func (s *server) LabelMessage() http.HandlerFunc {
	type labelMessageRequest struct {
		JID       string `json:"jid"`
		LabelID   string `json:"labelId"`
		MessageID string `json:"messageId"`
		Labeled   bool   `json:"labeled"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(userID)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		var request labelMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		request.JID = strings.TrimSpace(request.JID)
		request.LabelID = strings.TrimSpace(request.LabelID)
		request.MessageID = strings.TrimSpace(request.MessageID)
		if request.JID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing jid in Payload"))
			return
		}
		if request.LabelID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing labelId in Payload"))
			return
		}
		if request.MessageID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing messageId in Payload"))
			return
		}

		chatJID, err := types.ParseJID(request.JID)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid jid format"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), labelAppStateTimeout)
		defer cancel()

		if err := sendAppStatePatch(ctx, client, func() appstate.PatchInfo {
			return appstate.BuildLabelMessage(chatJID, request.LabelID, request.MessageID, request.Labeled)
		}); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to update message label: %w", err))
			return
		}

		if err := s.saveLabelMessageAssociation(userID, request.LabelID, chatJID.String(), request.MessageID, request.Labeled); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		response := map[string]interface{}{
			"success":   true,
			"jid":       chatJID.String(),
			"labelId":   request.LabelID,
			"messageId": request.MessageID,
			"labeled":   request.Labeled,
		}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(responseJSON))
	}
}

func (s *server) ListLabels() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		labels, err := s.listLabels(userID)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		response := map[string]interface{}{"labels": labels}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(responseJSON))
	}
}

func (s *server) saveLabel(userID, labelID, name string, color int32, deleted bool, dataJSON string) error {
	query := `
		INSERT INTO labels (user_id, label_id, name, color, deleted, updated_at, datajson)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, label_id) DO UPDATE SET
			name = EXCLUDED.name,
			color = EXCLUDED.color,
			deleted = EXCLUDED.deleted,
			updated_at = EXCLUDED.updated_at,
			datajson = EXCLUDED.datajson`
	if s.db.DriverName() == "sqlite" {
		query = `
			INSERT INTO labels (user_id, label_id, name, color, deleted, updated_at, datajson)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (user_id, label_id) DO UPDATE SET
				name = excluded.name,
				color = excluded.color,
				deleted = excluded.deleted,
				updated_at = excluded.updated_at,
				datajson = excluded.datajson`
	}

	if _, err := s.db.Exec(query, userID, labelID, name, color, deleted, time.Now().UTC(), dataJSON); err != nil {
		return fmt.Errorf("failed to save label: %w", err)
	}
	return nil
}

func (s *server) saveLabelEvent(userID, labelID, name string, color int32, colorKnown bool, deleted bool, dataJSON string) error {
	if deleted || name == "" {
		existingLabel, err := s.getLabel(userID, labelID)
		if err != nil {
			return err
		}
		if existingLabel != nil {
			if name == "" {
				name = existingLabel.Name
			}
			if !colorKnown {
				color = existingLabel.Color
			}
		}
	}

	return s.saveLabel(userID, labelID, name, color, deleted, dataJSON)
}

func (s *server) getLabel(userID, labelID string) (*persistedLabel, error) {
	query := `
		SELECT label_id, name, color, deleted
		FROM labels
		WHERE user_id = $1 AND label_id = $2`
	if s.db.DriverName() == "sqlite" {
		query = `
			SELECT label_id, name, color, deleted
			FROM labels
			WHERE user_id = ? AND label_id = ?`
	}

	var label persistedLabel
	if err := s.db.Get(&label, query, userID, labelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	return &label, nil
}

func resolvedLabelState(existingLabel *persistedLabel, name *string, color *int32, deleted *bool) (string, int32, bool) {
	resolvedName := ""
	resolvedColor := int32(0)
	resolvedDeleted := false
	if existingLabel != nil {
		resolvedName = existingLabel.Name
		resolvedColor = existingLabel.Color
		resolvedDeleted = existingLabel.Deleted
	}

	if name != nil {
		resolvedName = *name
	}
	if color != nil {
		resolvedColor = *color
	}
	if deleted != nil {
		resolvedDeleted = *deleted
	}

	return resolvedName, resolvedColor, resolvedDeleted
}

func (s *server) saveLabelChatAssociation(userID, labelID, chatJID string, labeled bool) error {
	if !labeled {
		query := `
			DELETE FROM label_chat_associations
			WHERE user_id = $1 AND label_id = $2 AND chat_jid = $3`
		if s.db.DriverName() == "sqlite" {
			query = `
				DELETE FROM label_chat_associations
				WHERE user_id = ? AND label_id = ? AND chat_jid = ?`
		}

		if _, err := s.db.Exec(query, userID, labelID, chatJID); err != nil {
			return fmt.Errorf("failed to delete chat label association: %w", err)
		}
		return nil
	}

	query := `
		INSERT INTO label_chat_associations (user_id, label_id, chat_jid, labeled, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, label_id, chat_jid) DO UPDATE SET
			labeled = EXCLUDED.labeled,
			updated_at = EXCLUDED.updated_at`
	if s.db.DriverName() == "sqlite" {
		query = `
			INSERT INTO label_chat_associations (user_id, label_id, chat_jid, labeled, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (user_id, label_id, chat_jid) DO UPDATE SET
				labeled = excluded.labeled,
				updated_at = excluded.updated_at`
	}

	if _, err := s.db.Exec(query, userID, labelID, chatJID, true, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to save chat label association: %w", err)
	}
	return nil
}

func (s *server) saveLabelMessageAssociation(userID, labelID, chatJID, messageID string, labeled bool) error {
	if !labeled {
		query := `
			DELETE FROM label_message_associations
			WHERE user_id = $1 AND label_id = $2 AND chat_jid = $3 AND message_id = $4`
		if s.db.DriverName() == "sqlite" {
			query = `
				DELETE FROM label_message_associations
				WHERE user_id = ? AND label_id = ? AND chat_jid = ? AND message_id = ?`
		}

		if _, err := s.db.Exec(query, userID, labelID, chatJID, messageID); err != nil {
			return fmt.Errorf("failed to delete message label association: %w", err)
		}
		return nil
	}

	query := `
		INSERT INTO label_message_associations (user_id, label_id, chat_jid, message_id, labeled, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, label_id, chat_jid, message_id) DO UPDATE SET
			labeled = EXCLUDED.labeled,
			updated_at = EXCLUDED.updated_at`
	if s.db.DriverName() == "sqlite" {
		query = `
			INSERT INTO label_message_associations (user_id, label_id, chat_jid, message_id, labeled, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (user_id, label_id, chat_jid, message_id) DO UPDATE SET
				labeled = excluded.labeled,
				updated_at = excluded.updated_at`
	}

	if _, err := s.db.Exec(query, userID, labelID, chatJID, messageID, true, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to save message label association: %w", err)
	}
	return nil
}

func (s *server) listLabels(userID string) ([]labelReadModel, error) {
	query := `
		SELECT label_id, name, color, deleted
		FROM labels
		WHERE user_id = $1
		ORDER BY deleted ASC, name ASC, label_id ASC`
	if s.db.DriverName() == "sqlite" {
		query = `
			SELECT label_id, name, color, deleted
			FROM labels
			WHERE user_id = ?
			ORDER BY deleted ASC, name ASC, label_id ASC`
	}

	rows := []persistedLabel{}
	if err := s.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	labels := make([]labelReadModel, 0, len(rows))
	labelIndex := make(map[string]int, len(rows))
	for _, row := range rows {
		labelIndex[row.LabelID] = len(labels)
		labels = append(labels, labelReadModel{
			LabelID: row.LabelID,
			Name:    row.Name,
			Color:   row.Color,
			Deleted: row.Deleted,
			Chats:   []string{},
		})
	}

	chatQuery := `
		SELECT label_id, chat_jid
		FROM label_chat_associations
		WHERE user_id = $1 AND labeled = TRUE
		ORDER BY label_id ASC, chat_jid ASC`
	if s.db.DriverName() == "sqlite" {
		chatQuery = `
			SELECT label_id, chat_jid
			FROM label_chat_associations
			WHERE user_id = ? AND labeled = 1
			ORDER BY label_id ASC, chat_jid ASC`
	}

	chatRows := []persistedLabelChat{}
	if err := s.db.Select(&chatRows, chatQuery, userID); err != nil {
		return nil, fmt.Errorf("failed to list label chat associations: %w", err)
	}

	for _, row := range chatRows {
		index, ok := labelIndex[row.LabelID]
		if !ok {
			continue
		}
		labels[index].Chats = append(labels[index].Chats, row.ChatJID)
	}

	return labels, nil
}

func marshalLabelDataJSON(value interface{}) string {
	dataJSON, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(dataJSON)
}
