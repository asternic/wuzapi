package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	adminToken string
	httpClient http.Client
}

type envelope struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Success bool            `json:"success"`
}

func New(baseURL string, adminToken string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		adminToken: adminToken,
		httpClient: http.Client{Timeout: 30 * time.Second},
	}
}

func (client *Client) AdminRequest(ctx context.Context, method string, path string, payload interface{}, target interface{}) error {
	return client.request(ctx, method, path, payload, target, func(request *http.Request) {
		request.Header.Set("Authorization", client.adminToken)
	})
}

func (client *Client) UserRequest(ctx context.Context, method string, path string, token string, payload interface{}, target interface{}) error {
	return client.request(ctx, method, path, payload, target, func(request *http.Request) {
		request.Header.Set("token", token)
	})
}

func (client *Client) request(ctx context.Context, method string, path string, payload interface{}, target interface{}, configure func(*http.Request)) error {
	if payload == nil {
		payload = map[string]interface{}{}
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	configure(request)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	var apiEnvelope envelope
	if err := json.Unmarshal(body, &apiEnvelope); err != nil {
		return fmt.Errorf("the API returned invalid JSON for %s %s: %w. Body: %s", method, path, err, compactBody(body))
	}

	if response.StatusCode < 200 || response.StatusCode > 299 || !apiEnvelope.Success {
		apiError := apiEnvelope.Error
		if apiError == "" {
			apiError = compactBody(body)
		}
		return fmt.Errorf("API returned HTTP %d for %s %s: %s", response.StatusCode, method, path, apiError)
	}

	if target == nil {
		return nil
	}

	if len(apiEnvelope.Data) == 0 || string(apiEnvelope.Data) == "null" {
		return nil
	}

	if err := json.Unmarshal(apiEnvelope.Data, target); err != nil {
		return fmt.Errorf("failed to read the response data field for %s %s: %w. Data: %s", method, path, err, compactBody(apiEnvelope.Data))
	}

	return nil
}
