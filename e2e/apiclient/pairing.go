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

	if err := client.connect(ctx, user); err != nil {
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

func (client *Client) connect(ctx context.Context, user *ScenarioUser) error {
	err := client.UserRequest(ctx, http.MethodPost, "/session/connect", user.Token, map[string]interface{}{
		"Subscribe": []string{"All"},
		"Immediate": true,
	}, nil)
	if err == nil || strings.Contains(err.Error(), "already connected") {
		return nil
	}

	return fmt.Errorf("failed to connect user session %q before requesting the pairing code: %w", user.Name, err)
}
