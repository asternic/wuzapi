package appium

import "strings"

func ContainsAnyFold(value string, expectedTexts ...string) bool {
	lowerValue := strings.ToLower(value)
	for _, expectedText := range expectedTexts {
		if strings.Contains(lowerValue, strings.ToLower(expectedText)) {
			return true
		}
	}

	return false
}

func sourceExcerpt(source string) string {
	source = strings.Join(strings.Fields(source), " ")
	if len(source) <= 600 {
		return source
	}

	return source[:600] + "..."
}

func compactBody(body []byte) string {
	compact := strings.Join(strings.Fields(string(body)), " ")
	if len(compact) <= 500 {
		return compact
	}

	return compact[:500] + "..."
}

func escapeUiAutomatorText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func escapeXPathText(value string) string {
	if !strings.Contains(value, `"`) {
		return `"` + value + `"`
	}

	if !strings.Contains(value, `'`) {
		return `'` + value + `'`
	}

	parts := strings.Split(value, `"`)
	quotedParts := make([]string, 0, len(parts)*2-1)
	for index, part := range parts {
		if part != "" {
			quotedParts = append(quotedParts, `"`+part+`"`)
		}

		if index < len(parts)-1 {
			quotedParts = append(quotedParts, `'"'`)
		}
	}

	return "concat(" + strings.Join(quotedParts, ", ") + ")"
}
