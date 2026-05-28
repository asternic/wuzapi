package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ScenarioUser struct {
	ID     string
	Name   string
	Token  string
	OSName string
}

func (client *Client) CreateUser(ctx context.Context, osName string) (*ScenarioUser, error) {
	if client.baseURL == "" {
		return nil, errors.New("the e2e HTTP server is not configured; run the e2e package so the test server can be started")
	}

	if client.adminToken == "" {
		return nil, errors.New("the e2e HTTP admin token is not configured; set E2E_WUZAPI_ADMIN_TOKEN")
	}

	osName = strings.TrimSpace(osName)
	if osName == "" {
		return nil, errors.New("the scenario user osname cannot be empty")
	}

	userToken := fmt.Sprintf("e2e-wuzapi-%d", time.Now().UnixNano())
	var createdUser struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Token  string `json:"token"`
		OSName string `json:"osname"`
	}

	err := client.AdminRequest(ctx, http.MethodPost, "/admin/users", map[string]interface{}{
		"name":   osName,
		"token":  userToken,
		"osname": osName,
	}, &createdUser)
	if err != nil {
		return nil, fmt.Errorf("failed to create user %q for the scenario: %w", osName, err)
	}

	return &ScenarioUser{
		ID:     firstNonEmpty(createdUser.ID, userToken),
		Name:   firstNonEmpty(createdUser.Name, osName),
		Token:  firstNonEmpty(createdUser.Token, userToken),
		OSName: firstNonEmpty(createdUser.OSName, osName),
	}, nil
}
