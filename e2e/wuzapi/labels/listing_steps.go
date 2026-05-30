package labels

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"wuzapi/e2e/support"
	"wuzapi/e2e/whatsapp/scenario"
	"wuzapi/e2e/wuzapi/users"
)

func registerListingSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^I list labels$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		labels, err := state.API().ListLabels(ctx, users.Current(ctx))
		if err != nil {
			return ctx, err
		}

		labelsState.setListedLabels(labels)
		return ctx, nil
	})

	scenarioContext.Step(`^the label list contains "([^"]*)"$`, func(ctx context.Context, alias string) (context.Context, error) {
		labelsState := labelStateFromContext(ctx)
		if findLabelByID(labelsState.listedLabels(), labelsState.labelID(alias)) == nil {
			return ctx, fmt.Errorf("label list does not contain %q", alias)
		}

		return ctx, nil
	})

	scenarioContext.Step(`^the label list does not contain deleted labels$`, func(ctx context.Context) (context.Context, error) {
		labelsState := labelStateFromContext(ctx)
		for _, label := range labelsState.listedLabels() {
			if !isScenarioLabel(labelsState, label) {
				continue
			}
			if label.Deleted {
				return ctx, fmt.Errorf("label list contains deleted label %q", label.Name)
			}
		}

		return ctx, nil
	})

	scenarioContext.Step(`^the chat label list contains label "([^"]*)"$`, func(ctx context.Context, alias string) (context.Context, error) {
		labelsState := labelStateFromContext(ctx)
		if findLabelByID(labelsState.listedLabels(), labelsState.labelID(alias)) == nil {
			return ctx, fmt.Errorf("chat label list does not contain label %q", alias)
		}

		return ctx, nil
	})

	scenarioContext.Step(`^the test contact chat appears under label "([^"]*)"(?: in the label list)?$`, func(ctx context.Context, alias string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		label, err := findListedLabel(ctx, state, labelsState, alias)
		if err != nil {
			return ctx, err
		}
		if !support.ContainsStringFold(label.Chats, state.Config().TestContactJID) {
			return ctx, fmt.Errorf("test contact chat %q was not listed under label %q", state.Config().TestContactJID, alias)
		}

		return ctx, nil
	})

	scenarioContext.Step(`^the test contact chat does not appear under label "([^"]*)"(?: in the label list)?$`, func(ctx context.Context, alias string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		labelsState := labelStateFromContext(ctx)
		label, err := findListedLabel(ctx, state, labelsState, alias)
		if err != nil {
			return ctx, err
		}
		if support.ContainsStringFold(label.Chats, state.Config().TestContactJID) {
			return ctx, fmt.Errorf("test contact chat %q was still listed under label %q", state.Config().TestContactJID, alias)
		}

		return ctx, nil
	})
}
