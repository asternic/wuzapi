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

type SessionStatus struct {
	Connected bool `json:"connected"`
	LoggedIn  bool `json:"loggedIn"`
}

func (client *Client) EnsureUser(ctx context.Context, name string) (*ScenarioUser, error) {
	users, err := client.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		if user.Name == name {
			return user, nil
		}
	}

	return client.CreateUser(ctx, name)
}

func (client *Client) ListUsers(ctx context.Context) ([]*ScenarioUser, error) {
	if client.baseURL == "" {
		return nil, errors.New("the e2e HTTP server is not configured; run the e2e package so the test server can be started")
	}

	var users []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Token  string `json:"token"`
		OSName string `json:"osname"`
	}
	if err := client.AdminRequest(ctx, http.MethodGet, "/admin/users", nil, &users); err != nil {
		return nil, fmt.Errorf("failed to list scenario users: %w", err)
	}

	scenarioUsers := make([]*ScenarioUser, 0, len(users))
	for _, user := range users {
		scenarioUsers = append(scenarioUsers, &ScenarioUser{
			ID:     user.ID,
			Name:   user.Name,
			Token:  user.Token,
			OSName: firstNonEmpty(user.OSName, user.Name),
		})
	}

	return scenarioUsers, nil
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

func (client *Client) Connect(ctx context.Context, user *ScenarioUser) error {
	if user == nil {
		return errors.New("no user was created for this scenario")
	}

	err := client.UserRequest(ctx, http.MethodPost, "/session/connect", user.Token, map[string]interface{}{
		"Subscribe": []string{"All"},
		"Immediate": true,
	}, nil)
	if err == nil || strings.Contains(err.Error(), "already connected") {
		return nil
	}

	return fmt.Errorf("failed to connect user session %q: %w", user.Name, err)
}

func (client *Client) Status(ctx context.Context, user *ScenarioUser) (SessionStatus, error) {
	if user == nil {
		return SessionStatus{}, errors.New("no user was created for this scenario")
	}

	var status SessionStatus
	if err := client.UserRequest(ctx, http.MethodGet, "/session/status", user.Token, nil, &status); err != nil {
		return SessionStatus{}, fmt.Errorf("failed to read session status for %q: %w", user.Name, err)
	}

	return status, nil
}

func (client *Client) SetWebhook(ctx context.Context, user *ScenarioUser, webhookURL string, events []string) error {
	if user == nil {
		return errors.New("no user was created for this scenario")
	}

	if err := client.UserRequest(ctx, http.MethodPost, "/webhook", user.Token, map[string]interface{}{
		"webhookurl": webhookURL,
		"events":     events,
	}, nil); err != nil {
		return fmt.Errorf("failed to configure webhook for %q: %w", user.Name, err)
	}

	return nil
}
