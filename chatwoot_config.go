package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wuzapi/internal/chatwoot"
)

var newChatwootClient = chatwoot.NewClient

type chatwootConfigPayload struct {
	ChatwootBaseURL         string `json:"chatwoot_base_url"`
	AccountID               int    `json:"account_id"`
	APIToken                string `json:"api_token"`
	InboxIdentifier         string `json:"inbox_identifier"`
	InboxName               string `json:"inbox_name"`
	InboxID                 int    `json:"inbox_id"`
	CallbackSecret          string `json:"callback_secret"`
	HMACSecret              string `json:"hmac_secret"`
	Enabled                 bool   `json:"enabled"`
	SignMessages            bool   `json:"sign_messages"`
	SignatureText           string `json:"signature_text"`
	ReopenConversations     bool   `json:"reopen_conversations"`
	SetConversationsPending bool   `json:"set_conversations_pending"`
	IgnoreGroups            bool   `json:"ignore_groups"`
	EnableTypingIndicator   bool   `json:"enable_typing_indicator"`
	IgnoredNumbers          string `json:"ignored_numbers"`
}

type chatwootTestPayload struct {
	ChatwootBaseURL string `json:"chatwoot_base_url"`
	AccountID       int    `json:"account_id"`
	APIToken        string `json:"api_token"`
}

type chatwootInboxPayload struct {
	ChatwootBaseURL string `json:"chatwoot_base_url"`
	AccountID       int    `json:"account_id"`
	APIToken        string `json:"api_token"`
	InboxName       string `json:"inbox_name"`
	InboxIdentifier string `json:"inbox_identifier"`
	InboxID         int    `json:"inbox_id"`
	CallbackSecret  string `json:"callback_secret"`
	CallbackURL     string `json:"callback_url"`
}

func (s *server) GetChatwootConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		cfg, err := s.GetChatwootConfigByUserID(userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				s.Respond(w, r, http.StatusNotFound, errors.New("chatwoot config not found"))
				return
			}
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		payload := chatwootConfigPayload{
			ChatwootBaseURL:         cfg.ChatwootBaseURL,
			AccountID:               cfg.AccountID,
			APIToken:                cfg.APIToken,
			InboxIdentifier:         cfg.InboxIdentifier,
			InboxName:               cfg.InboxName,
			InboxID:                 cfg.InboxID,
			CallbackSecret:          cfg.CallbackSecret,
			HMACSecret:              cfg.HMACSecret,
			Enabled:                 cfg.Enabled,
			SignMessages:            cfg.SignMessages,
			SignatureText:           cfg.SignatureText,
			ReopenConversations:     cfg.ReopenConversations,
			SetConversationsPending: cfg.SetConversationsPending,
			IgnoreGroups:            cfg.IgnoreGroups,
			EnableTypingIndicator:   cfg.EnableTypingIndicator,
			IgnoredNumbers:          cfg.IgnoredNumbers,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(body))
	}
}

func (s *server) SaveChatwootConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value("userinfo").(Values).Get("Id")

		var payload chatwootConfigPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		if err := validateChatwootConfigPayload(payload); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		existing, err := s.GetChatwootConfigByUserID(userID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		cfg := &ChatwootConfig{
			WuzapiUserID:            userID,
			ChatwootBaseURL:         normalizeChatwootBaseURL(payload.ChatwootBaseURL),
			AccountID:               payload.AccountID,
			APIToken:                strings.TrimSpace(payload.APIToken),
			InboxIdentifier:         strings.TrimSpace(payload.InboxIdentifier),
			InboxName:               strings.TrimSpace(payload.InboxName),
			InboxID:                 payload.InboxID,
			CallbackSecret:          strings.TrimSpace(payload.CallbackSecret),
			HMACSecret:              strings.TrimSpace(payload.HMACSecret),
			Enabled:                 payload.Enabled,
			SignMessages:            payload.SignMessages,
			SignatureText:           strings.TrimSpace(payload.SignatureText),
			ReopenConversations:     payload.ReopenConversations,
			SetConversationsPending: payload.SetConversationsPending,
			IgnoreGroups:            payload.IgnoreGroups,
			EnableTypingIndicator:   payload.EnableTypingIndicator,
			IgnoredNumbers:          strings.TrimSpace(payload.IgnoredNumbers),
		}

		if existing != nil {
			cfg.SystemContactIdentifier = existing.SystemContactIdentifier
			cfg.SystemConversationID = existing.SystemConversationID
		}

		if err := s.UpsertChatwootConfig(cfg); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		if err := s.ensureChatwootOnboarding(r.Context(), cfg); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		response := map[string]interface{}{
			"Details": "Chatwoot configuration saved successfully",
			"Config":  payload,
		}
		body, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(body))
	}
}

func (s *server) TestChatwootConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload chatwootTestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		if err := validateChatwootBasePayload(payload.ChatwootBaseURL, payload.AccountID, payload.APIToken); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		client, err := newChatwootClient(
			normalizeChatwootBaseURL(payload.ChatwootBaseURL),
			payload.AccountID,
			strings.TrimSpace(payload.APIToken),
		)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := client.TestConnection(ctx); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		response := map[string]interface{}{
			"Details": "Chatwoot connection test successful",
		}
		body, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(body))
	}
}

func (s *server) ProvisionChatwootInbox() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload chatwootInboxPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		if err := validateChatwootBasePayload(payload.ChatwootBaseURL, payload.AccountID, payload.APIToken); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(payload.InboxName) == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("inbox_name is required"))
			return
		}

		callbackToken := strings.TrimSpace(payload.CallbackSecret)
		if len(callbackToken) < 16 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("callback_secret must be at least 16 characters"))
			return
		}

		callbackURL, err := buildChatwootCallbackURL(r, callbackToken, payload.CallbackURL)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		client, err := newChatwootClient(
			normalizeChatwootBaseURL(payload.ChatwootBaseURL),
			payload.AccountID,
			strings.TrimSpace(payload.APIToken),
		)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		var inbox *chatwoot.Inbox
		action := "created"
		if payload.InboxID > 0 {
			action = "updated"
			inbox, err = client.UpdateAPIInbox(ctx, payload.InboxID, strings.TrimSpace(payload.InboxName), callbackURL)
		} else {
			inbox, err = client.CreateAPIInbox(ctx, strings.TrimSpace(payload.InboxName), callbackURL)
		}
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		inboxIdentifier := strings.TrimSpace(inbox.InboxIdentifier)
		if inboxIdentifier == "" {
			inboxIdentifier = strings.TrimSpace(payload.InboxIdentifier)
		}

		response := map[string]interface{}{
			"Details":          fmt.Sprintf("Chatwoot inbox %s successfully", action),
			"inbox_id":         inbox.ID,
			"inbox_identifier": inboxIdentifier,
			"callback_url":     callbackURL,
		}
		body, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(body))
	}
}

func validateChatwootBasePayload(baseURL string, accountID int, apiToken string) error {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return errors.New("chatwoot_base_url is required")
	}
	if !isHTTPURL(trimmed) {
		return errors.New("chatwoot_base_url must be a valid http/https URL")
	}
	if accountID <= 0 {
		return errors.New("account_id must be greater than 0")
	}
	if strings.TrimSpace(apiToken) == "" {
		return errors.New("api_token is required")
	}
	return nil
}

func validateChatwootConfigPayload(payload chatwootConfigPayload) error {
	if err := validateChatwootBasePayload(payload.ChatwootBaseURL, payload.AccountID, payload.APIToken); err != nil {
		return err
	}
	if strings.TrimSpace(payload.InboxName) == "" {
		return errors.New("inbox_name is required")
	}
	if strings.TrimSpace(payload.InboxIdentifier) == "" {
		return errors.New("inbox_identifier is required")
	}
	if payload.InboxID <= 0 {
		return errors.New("inbox_id must be greater than 0")
	}
	if len(strings.TrimSpace(payload.CallbackSecret)) < 16 {
		return errors.New("callback_secret must be at least 16 characters")
	}
	return nil
}

func normalizeChatwootBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func buildChatwootCallbackURL(r *http.Request, callbackSecret, override string) (string, error) {
	secret := strings.TrimSpace(callbackSecret)
	if secret == "" {
		return "", errors.New("callback_secret is required")
	}

	if override != "" {
		trimmed := strings.TrimSpace(override)
		if !isHTTPURL(trimmed) {
			return "", errors.New("callback_url must be a valid http/https URL")
		}
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("invalid callback_url: %w", err)
		}
		query := parsed.Query()
		if query.Get("token") == "" {
			query.Set("token", secret)
			parsed.RawQuery = query.Encode()
		}
		return parsed.String(), nil
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	if scheme != "http" && scheme != "https" {
		return "", errors.New("invalid callback url scheme")
	}

	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	if host == "" {
		return "", errors.New("missing host for callback url")
	}

	parsed := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   "/integrations/chatwoot/callback",
	}
	query := parsed.Query()
	query.Set("token", secret)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
