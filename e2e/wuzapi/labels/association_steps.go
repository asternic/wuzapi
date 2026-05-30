package labels

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"wuzapi/e2e/support"
	"wuzapi/e2e/whatsapp/scenario"
	"wuzapi/e2e/wuzapi/users"
)

func registerAssociationSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^I apply that label to the test contact chat$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labelsState.setExpectedChatLabeled(true)
		return ctx, state.API().LabelChat(ctx, users.Current(ctx), state.Config().TestContactJID, currentLabelID(labelsState), true)
	})

	scenarioContext.Step(`^I remove that label from the test contact chat$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labelsState.setExpectedChatLabeled(false)
		return ctx, state.API().LabelChat(ctx, users.Current(ctx), state.Config().TestContactJID, currentLabelID(labelsState), false)
	})

	scenarioContext.Step(`^a "([^"]*)" event is received for that chat and label$`, func(ctx context.Context, eventType string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labelID := currentLabelID(labelsState)
		chatJID := state.Config().TestContactJID
		payload, err := labelsState.recorder().WaitForMatch(ctx, webhookEventTimeout, func(payload support.WebhookPayload) bool {
			if payload.StringField("type") != eventType ||
				payload.StringField("labelId") != labelID ||
				payload.StringField("jid") != chatJID {
				return false
			}

			expected, ok := labelsState.expectedChatLabeled()
			return !ok || payload.BoolField("labeled") == expected
		})
		if err != nil {
			return ctx, err
		}

		labelsState.setLastWebhookEvent(payload)
		return ctx, nil
	})

	scenarioContext.Step(`^the event says the chat is labeled$`, func(ctx context.Context) (context.Context, error) {
		return ctx, labelStateFromContext(ctx).lastWebhookEvent().AssertBoolField("labeled", true)
	})

	scenarioContext.Step(`^the event says the chat is not labeled$`, func(ctx context.Context) (context.Context, error) {
		return ctx, labelStateFromContext(ctx).lastWebhookEvent().AssertBoolField("labeled", false)
	})

	scenarioContext.Step(`^the test contact chat has the label "([^"]*)"$`, func(ctx context.Context, alias string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		if err := state.API().LabelChat(ctx, users.Current(ctx), state.Config().TestContactJID, labelsState.labelID(alias), true); err != nil {
			return ctx, err
		}

		_, err := labelsState.recorder().WaitForMatch(ctx, webhookEventTimeout, func(payload support.WebhookPayload) bool {
			return payload.StringField("type") == "LabelAssociationChat" &&
				payload.StringField("labelId") == labelsState.labelID(alias) &&
				payload.StringField("jid") == state.Config().TestContactJID &&
				payload.BoolField("labeled")
		})
		if err != nil {
			return ctx, fmt.Errorf("expected test contact chat to receive label %q: %w", alias, err)
		}
		return ctx, nil
	})
}
