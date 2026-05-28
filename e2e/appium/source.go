package appium

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"
)

func (session *Session) WaitForText(timeout time.Duration, expectedTexts ...string) error {
	deadline := time.Now().Add(timeout)
	var lastSource string
	var lastErr error

	for time.Now().Before(deadline) {
		source, err := session.PageSource()
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		lastSource = source
		if ContainsAnyFold(source, expectedTexts...) {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("did not find any of these texts: %s. Current screen: %s", strings.Join(expectedTexts, ", "), sourceExcerpt(lastSource))
}

func (session *Session) WaitForPackage(timeout time.Duration, appPackage string) error {
	deadline := time.Now().Add(timeout)
	var lastSource string
	var lastErr error

	for time.Now().Before(deadline) {
		source, err := session.PageSource()
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		lastSource = source
		if strings.Contains(source, `package="`+appPackage+`"`) {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("did not find package %s in the current screen. Current screen: %s", appPackage, sourceExcerpt(lastSource))
}

func (session *Session) WaitUntilTextGone(timeout time.Duration, unexpectedTexts ...string) error {
	deadline := time.Now().Add(timeout)
	var lastSource string
	var lastErr error

	for time.Now().Before(deadline) {
		source, err := session.PageSource()
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		lastSource = source
		if !ContainsAnyFold(source, unexpectedTexts...) {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("still found one of these texts: %s. Current screen: %s", strings.Join(unexpectedTexts, ", "), sourceExcerpt(lastSource))
}

func (session *Session) PageSource() (string, error) {
	response, err := session.request("GET", session.sessionPath("/source"), nil)
	if err != nil {
		return "", err
	}

	var body struct {
		Value interface{} `json:"value"`
	}
	if err := json.Unmarshal(response.body, &body); err != nil {
		return "", err
	}

	switch value := body.Value.(type) {
	case string:
		return html.UnescapeString(value), nil
	case nil:
		return "", errors.New("Appium returned an empty screen")
	default:
		encodedValue, err := json.Marshal(value)
		if err != nil {
			return "", err
		}

		return html.UnescapeString(string(encodedValue)), nil
	}
}
