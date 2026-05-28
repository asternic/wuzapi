package appium

type Selector struct {
	using string
	value string
}

func ByAccessibilityID(value string) Selector {
	return Selector{using: "accessibility id", value: value}
}

func ByID(value string) Selector {
	return Selector{using: "id", value: value}
}

func ByUiAutomator(value string) Selector {
	return Selector{using: "-android uiautomator", value: value}
}

func ByXPath(value string) Selector {
	return Selector{using: "xpath", value: value}
}

func TextSelectors(text string) []Selector {
	escapedText := escapeUiAutomatorText(text)
	escapedXPathText := escapeXPathText(text)

	return []Selector{
		ByUiAutomator(`new UiSelector().text("` + escapedText + `")`),
		ByUiAutomator(`new UiSelector().textContains("` + escapedText + `")`),
		ByXPath(`//*[@text=` + escapedXPathText + ` or contains(@text, ` + escapedXPathText + `)]`),
	}
}
