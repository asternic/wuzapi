package labels

import (
	"context"
	"fmt"
	"time"

	"wuzapi/e2e/apiclient"
	"wuzapi/e2e/whatsapp/scenario"
	"wuzapi/e2e/wuzapi/users"
)

const webhookEventTimeout = 60 * time.Second

func createLabel(ctx context.Context, state *scenario.State, labelsState *labelState, alias string, color int32) {
	labelID := fmt.Sprintf("%d", time.Now().UnixNano())
	actualName := uniqueLabelName(alias)
	err := state.API().EditLabel(ctx, users.Current(ctx), labelID, actualName, color, false)
	labelsState.setOperationError(err)
	labelsState.setExpectedLabelDeleted(false)
	if err == nil {
		labelsState.setLabel(alias, labelID, actualName)
	}
}

func uniqueLabelName(alias string) string {
	return fmt.Sprintf("%s %d", alias, time.Now().UnixNano()%1_000_000)
}

func currentLabelID(labelsState *labelState) string {
	return labelsState.labelID(labelsState.currentLabelAlias())
}

func findListedLabel(ctx context.Context, state *scenario.State, labelsState *labelState, alias string) (*apiclient.Label, error) {
	labels, err := state.API().ListLabels(ctx, users.Current(ctx))
	if err != nil {
		return nil, err
	}
	labelsState.setListedLabels(labels)

	label := findLabelByID(labels, labelsState.labelID(alias))
	if label == nil {
		return nil, fmt.Errorf("label %q was not found in the label list", alias)
	}

	return label, nil
}

func findLabelByID(labels []apiclient.Label, labelID string) *apiclient.Label {
	for index := range labels {
		if labels[index].LabelID == labelID {
			return &labels[index]
		}
	}

	return nil
}

func isScenarioLabel(labelsState *labelState, label apiclient.Label) bool {
	for _, labelID := range []string{
		labelsState.labelID("Leads"),
		labelsState.labelID("Follow Up"),
		labelsState.labelID("Hot Leads"),
	} {
		if labelID != "" && label.LabelID == labelID {
			return true
		}
	}

	return false
}
