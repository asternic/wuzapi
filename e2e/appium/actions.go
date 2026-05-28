package appium

import (
	"fmt"
	"net/url"
	"time"
)

type Element struct {
	session *Session
	id      string
}

func (session *Session) WaitAndTap(label string, selectors []Selector, timeout time.Duration) error {
	element, err := session.WaitForElement(label, selectors, timeout)
	if err != nil {
		return err
	}

	return element.Click()
}

func (session *Session) TapIfVisible(label string, selectors []Selector, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		element, err := session.FindFirst(selectors)
		if err == nil {
			return element.Click()
		}

		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("did not find %s: %w", label, lastErr)
}

func (session *Session) WaitForElement(label string, selectors []Selector, timeout time.Duration) (Element, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		element, err := session.FindFirst(selectors)
		if err == nil {
			return element, nil
		}

		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	source, _ := session.PageSource()
	return Element{}, fmt.Errorf("did not find %s: %w. Current screen: %s", label, lastErr, sourceExcerpt(source))
}

func (session *Session) FindFirst(selectors []Selector) (Element, error) {
	var lastErr error

	for _, selector := range selectors {
		element, err := session.FindElement(selector)
		if err == nil {
			return element, nil
		}

		lastErr = err
	}

	return Element{}, lastErr
}

func (session *Session) FindElement(selector Selector) (Element, error) {
	response, err := session.request("POST", session.sessionPath("/element"), map[string]interface{}{
		"using": selector.using,
		"value": selector.value,
	})
	if err != nil {
		return Element{}, err
	}

	elementID, err := parseElementID(response.body)
	if err != nil {
		return Element{}, err
	}

	return Element{
		session: session,
		id:      elementID,
	}, nil
}

func (element Element) Click() error {
	escapedElementID := url.PathEscape(element.id)
	_, err := element.session.request("POST", element.session.sessionPath("/element/"+escapedElementID+"/click"), map[string]interface{}{})
	return err
}

func (element Element) SendKeys(text string) error {
	escapedElementID := url.PathEscape(element.id)
	_, err := element.session.request("POST", element.session.sessionPath("/element/"+escapedElementID+"/value"), map[string]interface{}{
		"text":  text,
		"value": []string{text},
	})

	return err
}
