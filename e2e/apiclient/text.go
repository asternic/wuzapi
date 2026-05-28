package apiclient

import "strings"

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func compactBody(body []byte) string {
	compact := strings.Join(strings.Fields(string(body)), " ")
	if len(compact) <= 500 {
		return compact
	}

	return compact[:500] + "..."
}
