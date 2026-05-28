package appium

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nichotined/appium-go-client/webdriver"
)

type response struct {
	statusCode int
	body       []byte
}

func (session *Session) request(method string, path string, body map[string]interface{}) (*response, error) {
	return request(session.driver, method, path, body)
}

func request(driver *webdriver.Driver, method string, path string, body map[string]interface{}) (*response, error) {
	if driver == nil || driver.Client == nil {
		return nil, errors.New("Appium driver was not started")
	}

	if body == nil {
		body = map[string]interface{}{}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	driverResponse, err := driver.Client.MakeRequest(method, &payload, path)
	if err != nil {
		return nil, err
	}

	var responseBody []byte
	if driverResponse.Body != nil {
		responseBody = *driverResponse.Body
	}

	appiumResponse := &response{
		statusCode: driverResponse.StatusCode,
		body:       responseBody,
	}

	if driverResponse.StatusCode < 200 || driverResponse.StatusCode > 299 {
		return appiumResponse, fmt.Errorf("Appium returned HTTP %d for %s %s: %s", driverResponse.StatusCode, method, path, compactBody(responseBody))
	}

	return appiumResponse, nil
}

func parseSessionID(body []byte) (string, error) {
	var payload struct {
		SessionID string          `json:"sessionId"`
		Value     json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	if payload.SessionID != "" {
		return payload.SessionID, nil
	}

	var value struct {
		SessionID string `json:"sessionId"`
	}
	if len(payload.Value) > 0 {
		if err := json.Unmarshal(payload.Value, &value); err == nil && value.SessionID != "" {
			return value.SessionID, nil
		}
	}

	return "", fmt.Errorf("the Appium response did not include sessionId: %s", compactBody(body))
}

func parseElementID(body []byte) (string, error) {
	var payload struct {
		Value map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	for _, key := range []string{"element-6066-11e4-a52e-4f735466cecf", "ELEMENT"} {
		if elementID, ok := payload.Value[key].(string); ok && elementID != "" {
			return elementID, nil
		}
	}

	return "", fmt.Errorf("the Appium response did not include the element id: %s", compactBody(body))
}
