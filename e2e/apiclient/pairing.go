package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (client *Client) RequestPairCode(ctx context.Context, user *ScenarioUser, phone string) (string, error) {
	if user == nil {
		return "", errors.New("no user was created for this scenario")
	}

	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "", errors.New("set E2E_PAIR_PHONE to the primary WhatsApp number without a leading + before requesting the pairing code")
	}

	if err := client.Connect(ctx, user); err != nil {
		return "", err
	}

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		var pairResponse struct {
			LinkingCode string `json:"LinkingCode"`
		}

		err := client.UserRequest(ctx, http.MethodPost, "/session/pairphone", user.Token, map[string]interface{}{
			"Phone": phone,
		}, &pairResponse)
		if err == nil && strings.TrimSpace(pairResponse.LinkingCode) != "" {
			return pairResponse.LinkingCode, nil
		}

		if err == nil {
			lastErr = errors.New("the API responded without LinkingCode")
		} else {
			lastErr = err
		}

		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("failed to request the pairing code for user %q: %w", user.Name, lastErr)
}
