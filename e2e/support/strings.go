package support

import "strings"

func ContainsStringFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}

	return false
}
