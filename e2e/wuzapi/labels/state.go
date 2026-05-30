package labels

import (
	"context"

	"wuzapi/e2e/apiclient"
	"wuzapi/e2e/support"
)

type stateKey struct{}

type labelState struct {
	webhookRecorder *support.WebhookRecorder
	operationErr    error
	currentLabel    string
	labelIDs        map[string]string
	labelNames      map[string]string
	lastEvent       support.WebhookPayload
	labelList       []apiclient.Label
	labelDeleted    *bool
	chatLabeled     *bool
}

func newLabelStateContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateKey{}, newLabelState())
}

func labelStateFromContext(ctx context.Context) *labelState {
	labelsState, _ := ctx.Value(stateKey{}).(*labelState)
	return labelsState
}

func newLabelState() *labelState {
	return &labelState{
		labelIDs:   map[string]string{},
		labelNames: map[string]string{},
	}
}

func (state *labelState) Close() error {
	if state.webhookRecorder != nil {
		state.webhookRecorder.Close()
	}

	return nil
}

func (state *labelState) recorder() *support.WebhookRecorder {
	return state.webhookRecorder
}

func (state *labelState) setWebhookRecorder(webhookRecorder *support.WebhookRecorder) {
	state.webhookRecorder = webhookRecorder
}

func (state *labelState) operationError() error {
	return state.operationErr
}

func (state *labelState) setOperationError(err error) {
	state.operationErr = err
}

func (state *labelState) setLabel(alias, labelID, actualName string) {
	state.labelIDs[alias] = labelID
	state.labelNames[alias] = actualName
	state.currentLabel = alias
}

func (state *labelState) labelID(alias string) string {
	return state.labelIDs[alias]
}

func (state *labelState) labelName(alias string) string {
	if labelName := state.labelNames[alias]; labelName != "" {
		return labelName
	}
	return alias
}

func (state *labelState) currentLabelAlias() string {
	return state.currentLabel
}

func (state *labelState) lastWebhookEvent() support.WebhookPayload {
	return state.lastEvent
}

func (state *labelState) setLastWebhookEvent(payload support.WebhookPayload) {
	state.lastEvent = payload
}

func (state *labelState) listedLabels() []apiclient.Label {
	return state.labelList
}

func (state *labelState) setListedLabels(labels []apiclient.Label) {
	state.labelList = labels
}

func (state *labelState) expectedLabelDeleted() (bool, bool) {
	if state.labelDeleted == nil {
		return false, false
	}
	return *state.labelDeleted, true
}

func (state *labelState) setExpectedLabelDeleted(deleted bool) {
	state.labelDeleted = &deleted
}

func (state *labelState) expectedChatLabeled() (bool, bool) {
	if state.chatLabeled == nil {
		return false, false
	}
	return *state.chatLabeled, true
}

func (state *labelState) setExpectedChatLabeled(labeled bool) {
	state.chatLabeled = &labeled
}
