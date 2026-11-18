package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const chatwootRequestTimeout = 15 * time.Second

type chatwootConfigRequest struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url"`
	AccountID int64  `json:"account_id"`
	Token     string `json:"token"`
	InboxID   int64  `json:"inbox_id"`
	InboxName string `json:"inbox_name"`
}

type chatwootConfigRecord struct {
	Enabled   bool   `db:"enabled"`
	URL       string `db:"url"`
	AccountID int64  `db:"account_id"`
	Token     []byte `db:"token"`
	InboxID   int64  `db:"inbox_id"`
	InboxName string `db:"inbox_name"`
}

func (s *server) ConfigureChatwoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID := r.Context().Value("userinfo").(Values).Get("Id")
		var request chatwootConfigRequest
		if err := decodeAPIRequest(r.Body, &request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if !globalEncryptionKeyPersistent {
			s.Respond(w, r, http.StatusBadRequest, errors.New("WUZAPI_GLOBAL_ENCRYPTION_KEY must be configured before Chatwoot can be enabled"))
			return
		}
		baseURL, err := validateChatwootURL(request.URL)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if request.AccountID <= 0 || strings.TrimSpace(request.Token) == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("account_id and token are required"))
			return
		}
		if request.InboxName == "" {
			request.InboxName = "WuzAPI " + instanceID
		}

		client := newChatwootClient(baseURL, request.AccountID, request.Token)
		if err := client.verifyAccount(r.Context()); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not validate the Chatwoot account or token"))
			return
		}
		inboxID := request.InboxID
		if inboxID == 0 {
			inboxID, err = client.createAPIInbox(r.Context(), request.InboxName)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not create the Chatwoot inbox"))
				return
			}
		}
		encryptedToken, err := encryptHMACKey(request.Token)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not encrypt the Chatwoot token"))
			return
		}
		query := s.db.Rebind(`INSERT INTO chatwoot_config (instance_id, enabled, url, account_id, token, inbox_id, inbox_name, updated_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
            ON CONFLICT (instance_id) DO UPDATE SET enabled=EXCLUDED.enabled, url=EXCLUDED.url,
            account_id=EXCLUDED.account_id, token=EXCLUDED.token, inbox_id=EXCLUDED.inbox_id,
            inbox_name=EXCLUDED.inbox_name, updated_at=CURRENT_TIMESTAMP`)
		if _, err := s.db.Exec(query, instanceID, request.Enabled, baseURL, request.AccountID, encryptedToken, inboxID, request.InboxName); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not save the Chatwoot configuration"))
			return
		}
		if request.Enabled {
			s.ensureChatwootWorker()
		}
		s.Respond(w, r, http.StatusOK, fmt.Sprintf(`{"Details":"Chatwoot configuration saved","inbox_id":%d}`, inboxID))
	}
}

func (s *server) GetChatwootConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID := r.Context().Value("userinfo").(Values).Get("Id")
		var config chatwootConfigRecord
		if err := s.db.Get(&config, s.db.Rebind(`SELECT enabled, url, account_id, token, inbox_id, inbox_name FROM chatwoot_config WHERE instance_id=?`), instanceID); err != nil {
			s.Respond(w, r, http.StatusNotFound, errors.New("Chatwoot is not configured"))
			return
		}
		response, err := json.Marshal(map[string]any{
			"enabled":          config.Enabled,
			"url":              config.URL,
			"account_id":       config.AccountID,
			"inbox_id":         config.InboxID,
			"inbox_name":       config.InboxName,
			"token_configured": len(config.Token) > 0,
		})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not serialize the Chatwoot configuration"))
			return
		}
		s.Respond(w, r, http.StatusOK, string(response))
	}
}

func (s *server) GetChatwootStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID := r.Context().Value("userinfo").(Values).Get("Id")
		var status struct {
			Pending int    `json:"pending"`
			Dead    int    `json:"dead"`
			LastErr string `json:"last_error" db:"last_error"`
		}
		query := s.db.Rebind(`SELECT
            COALESCE(SUM(CASE WHEN state=? THEN 1 ELSE 0 END), 0) AS pending,
            COALESCE(SUM(CASE WHEN state=? THEN 1 ELSE 0 END), 0) AS dead,
            COALESCE((SELECT last_error FROM chatwoot_outbox WHERE instance_id=? AND state=? ORDER BY id DESC LIMIT 1), '') AS last_error
            FROM chatwoot_outbox WHERE instance_id=?`)
		if err := s.db.Get(&status, query, chatwootOutboxPending, chatwootOutboxDead, instanceID, chatwootOutboxDead, instanceID); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not read the Chatwoot queue status"))
			return
		}
		response, err := json.Marshal(status)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not serialize the Chatwoot queue status"))
			return
		}
		s.Respond(w, r, http.StatusOK, string(response))
	}
}

func (s *server) DisableChatwoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID := r.Context().Value("userinfo").(Values).Get("Id")
		result, err := s.db.Exec(s.db.Rebind(`UPDATE chatwoot_config SET enabled=?, updated_at=CURRENT_TIMESTAMP WHERE instance_id=?`), false, instanceID)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not disable Chatwoot"))
			return
		}
		changed, err := result.RowsAffected()
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not disable Chatwoot"))
			return
		}
		if changed == 0 {
			s.Respond(w, r, http.StatusNotFound, errors.New("Chatwoot is not configured"))
			return
		}
		s.Respond(w, r, http.StatusOK, `{"Details":"Chatwoot disabled; remote inbox preserved"}`)
	}
}

type chatwootClient struct {
	baseURL   string
	accountID int64
	token     string
	http      *http.Client
}

func newChatwootClient(baseURL string, accountID int64, token string) *chatwootClient {
	return &chatwootClient{baseURL: baseURL, accountID: accountID, token: token, http: &http.Client{Timeout: chatwootRequestTimeout}}
}

func (c *chatwootClient) verifyAccount(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodGet, "/api/v1/accounts/"+strconv.FormatInt(c.accountID, 10), nil, nil)
}

func (c *chatwootClient) createAPIInbox(ctx context.Context, name string) (int64, error) {
	var response struct {
		ID int64 `json:"id"`
	}
	payload := map[string]any{"name": name, "channel": map[string]any{"type": "api"}}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(c.accountID, 10)+"/inboxes", payload, &response); err != nil {
		return 0, err
	}
	if response.ID <= 0 {
		return 0, errors.New("Chatwoot did not return an inbox id")
	}
	return response.ID, nil
}

func (c *chatwootClient) doJSON(ctx context.Context, method, path string, payload, target any) error {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Chatwoot returned HTTP %d", resp.StatusCode)
	}
	if target != nil {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
		if err != nil {
			return err
		}
		if len(body) > 1<<20 {
			return errors.New("Chatwoot response exceeds the allowed size")
		}
		return json.Unmarshal(body, target)
	}
	return nil
}

func validateChatwootURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid Chatwoot URL")
	}
	if parsed.Scheme == "https" {
		if hostResolvesPrivate(parsed.Hostname()) && !chatwootInternalHostAllowed(parsed.Hostname()) {
			return "", errors.New("Chatwoot URL resolves to a private host that is not allowlisted")
		}
		return strings.TrimRight(parsed.String(), "/"), nil
	}
	if parsed.Scheme == "http" && chatwootInternalHostAllowed(parsed.Hostname()) {
		return strings.TrimRight(parsed.String(), "/"), nil
	}
	return "", errors.New("Chatwoot URL must use HTTPS unless its host is allowlisted internally")
}

func hostResolvesPrivate(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return isPrivateOrLoopback(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if isPrivateOrLoopback(ip) {
			return true
		}
	}
	return false
}

func chatwootInternalHostAllowed(host string) bool {
	for _, allowed := range strings.Split(os.Getenv("WUZAPI_CHATWOOT_INTERNAL_HOSTS"), ",") {
		if strings.EqualFold(strings.TrimSpace(allowed), host) {
			return true
		}
	}
	return net.ParseIP(host) != nil && false
}
