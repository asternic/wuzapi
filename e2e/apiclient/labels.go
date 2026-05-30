package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

type Label struct {
	LabelID string   `json:"labelId"`
	Name    string   `json:"name"`
	Color   int32    `json:"color"`
	Deleted bool     `json:"deleted"`
	Chats   []string `json:"chats"`
}

func (client *Client) EditLabel(ctx context.Context, user *ScenarioUser, labelID, name string, color int32, deleted bool) error {
	if user == nil {
		return errors.New("no user was created for this scenario")
	}

	if err := client.UserRequest(ctx, http.MethodPost, "/label/edit", user.Token, map[string]interface{}{
		"labelId": labelID,
		"name":    name,
		"color":   color,
		"deleted": deleted,
	}, nil); err != nil {
		return fmt.Errorf("failed to edit label %q: %w", name, err)
	}

	return nil
}

func (client *Client) LabelChat(ctx context.Context, user *ScenarioUser, jid, labelID string, labeled bool) error {
	if user == nil {
		return errors.New("no user was created for this scenario")
	}

	if err := client.UserRequest(ctx, http.MethodPost, "/label/chat", user.Token, map[string]interface{}{
		"jid":     jid,
		"labelId": labelID,
		"labeled": labeled,
	}, nil); err != nil {
		return fmt.Errorf("failed to update chat label association: %w", err)
	}

	return nil
}

func (client *Client) ListLabels(ctx context.Context, user *ScenarioUser) ([]Label, error) {
	if user == nil {
		return nil, errors.New("no user was created for this scenario")
	}

	var response struct {
		Labels []Label `json:"labels"`
	}
	if err := client.UserRequest(ctx, http.MethodGet, "/label/list", user.Token, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	return response.Labels, nil
}
