package labels

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"wuzapi/e2e/support"
	"wuzapi/e2e/whatsapp/scenario"
	"wuzapi/e2e/wuzapi/users"
)

func registerLifecycleSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^I create a label named "([^"]*)" with color (\d+)$`, func(ctx context.Context, alias string, color int) (context.Context, error) {
		state := scenario.FromContext(ctx)
		createLabel(ctx, state, labelStateFromContext(ctx), alias, int32(color))
		return ctx, nil
	})

	scenarioContext.Step(`^a label named "([^"]*)" exists$`, func(ctx context.Context, alias string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		createLabel(ctx, state, labelsState, alias, 5)
		return ctx, labelsState.operationError()
	})

	scenarioContext.Step(`^I rename that label to "([^"]*)"$`, func(ctx context.Context, alias string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labelID := currentLabelID(labelsState)
		if labelID == "" {
			return ctx, fmt.Errorf("no current label is available to rename")
		}

		actualName := uniqueLabelName(alias)
		err := state.API().EditLabel(ctx, users.Current(ctx), labelID, actualName, 5, false)
		labelsState.setOperationError(err)
		labelsState.setExpectedLabelDeleted(false)
		if err == nil {
			labelsState.setLabel(alias, labelID, actualName)
		}

		return ctx, nil
	})

	scenarioContext.Step(`^I delete that label$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labelID := currentLabelID(labelsState)
		if labelID == "" {
			return ctx, fmt.Errorf("no current label is available to delete")
		}

		actualName := labelsState.labelName(labelsState.currentLabelAlias())
		err := state.API().EditLabel(ctx, users.Current(ctx), labelID, actualName, 5, true)
		labelsState.setOperationError(err)
		labelsState.setExpectedLabelDeleted(true)
		return ctx, nil
	})

	scenarioContext.Step(`^the label operation succeeds$`, func(ctx context.Context) (context.Context, error) {
		if err := labelStateFromContext(ctx).operationError(); err != nil {
			return ctx, err
		}

		return ctx, nil
	})

	scenarioContext.Step(`^a "([^"]*)" event is received for label "([^"]*)"$`, func(ctx context.Context, eventType, alias string) (context.Context, error) {
		labelsState := labelStateFromContext(ctx)
		actualName := labelsState.labelName(alias)
		payload, err := labelsState.recorder().WaitForMatch(ctx, webhookEventTimeout, func(payload support.WebhookPayload) bool {
			return payload.StringField("type") == eventType && payload.StringField("name") == actualName
		})
		if err != nil {
			return ctx, err
		}

		labelsState.setLastWebhookEvent(payload)
		return ctx, nil
	})

	scenarioContext.Step(`^a "([^"]*)" event is received for that label$`, func(ctx context.Context, eventType string) (context.Context, error) {
		labelsState := labelStateFromContext(ctx)
		labelID := currentLabelID(labelsState)
		payload, err := labelsState.recorder().WaitForMatch(ctx, webhookEventTimeout, func(payload support.WebhookPayload) bool {
			if payload.StringField("type") != eventType || payload.StringField("labelId") != labelID {
				return false
			}
			if eventType == "LabelEdit" {
				expected, ok := labelsState.expectedLabelDeleted()
				return !ok || payload.BoolField("deleted") == expected
			}
			return true
		})
		if err != nil {
			return ctx, err
		}

		labelsState.setLastWebhookEvent(payload)
		return ctx, nil
	})

	scenarioContext.Step(`^the event says the label color is (\d+)$`, func(ctx context.Context, color int) (context.Context, error) {
		eventColor, ok := labelStateFromContext(ctx).lastWebhookEvent().IntField("color")
		if !ok || eventColor != color {
			return ctx, fmt.Errorf("expected label color %d, got %d", color, eventColor)
		}

		return ctx, nil
	})

	scenarioContext.Step(`^the event says the label is not deleted$`, func(ctx context.Context) (context.Context, error) {
		return ctx, labelStateFromContext(ctx).lastWebhookEvent().AssertBoolField("deleted", false)
	})

	scenarioContext.Step(`^the event says the label is deleted$`, func(ctx context.Context) (context.Context, error) {
		return ctx, labelStateFromContext(ctx).lastWebhookEvent().AssertBoolField("deleted", true)
	})

	scenarioContext.Step(`^the label "([^"]*)" appears in the label list$`, func(ctx context.Context, alias string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labels, err := state.API().ListLabels(ctx, users.Current(ctx))
		if err != nil {
			return ctx, err
		}
		labelsState.setListedLabels(labels)

		if findLabelByID(labels, labelsState.labelID(alias)) == nil {
			return ctx, fmt.Errorf("label %q was not found in the label list", alias)
		}

		return ctx, nil
	})

	scenarioContext.Step(`^that label appears as deleted in the label list$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labels, err := state.API().ListLabels(ctx, users.Current(ctx))
		if err != nil {
			return ctx, err
		}
		labelsState.setListedLabels(labels)

		label := findLabelByID(labels, currentLabelID(labelsState))
		if label == nil {
			return ctx, fmt.Errorf("the current label was not found in the label list")
		}
		if !label.Deleted {
			return ctx, fmt.Errorf("the current label is not marked as deleted in the label list")
		}

		return ctx, nil
	})
}
