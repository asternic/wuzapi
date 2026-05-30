package support

import (
	"fmt"
	"math"
)

func (payload WebhookPayload) StringField(key string) string {
	value, _ := payload[key].(string)
	return value
}

func (payload WebhookPayload) BoolField(key string) bool {
	value, _ := payload[key].(bool)
	return value
}

func (payload WebhookPayload) IntField(key string) (int, bool) {
	switch value := payload[key].(type) {
	case float64:
		if math.Trunc(value) != value {
			return 0, false
		}
		return int(value), true
	case int:
		return value, true
	default:
		return 0, false
	}
}

func (payload WebhookPayload) AssertBoolField(key string, expected bool) error {
	actual := payload.BoolField(key)
	if actual != expected {
		return fmt.Errorf("expected event field %s to be %t, got %t", key, expected, actual)
	}

	return nil
}
