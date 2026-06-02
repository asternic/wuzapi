package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSetProfileNameValidation covers the request-validation and session-guard
// paths of the #281 profile-name endpoint without a live WhatsApp session (the
// actual SendAppState round-trip needs a paired client and can't be unit-tested
// here). It mirrors how the authalice middleware injects "userinfo".
func TestSetProfileNameValidation(t *testing.T) {
	s := makeTestServer(t)
	handler := s.SetProfileName()

	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/user/profile/name", strings.NewReader(body))
		ctx := context.WithValue(req.Context(), "userinfo", Values{map[string]string{"Id": "profile-test-user"}})
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	t.Run("empty name is rejected", func(t *testing.T) {
		if rr := call(`{"Name":""}`); rr.Code != http.StatusBadRequest {
			t.Errorf("empty name: got %d, want 400 (body=%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("whitespace-only name is rejected", func(t *testing.T) {
		if rr := call(`{"Name":"   "}`); rr.Code != http.StatusBadRequest {
			t.Errorf("whitespace name: got %d, want 400", rr.Code)
		}
	})

	t.Run("malformed JSON is rejected", func(t *testing.T) {
		if rr := call(`{not json`); rr.Code != http.StatusBadRequest {
			t.Errorf("bad json: got %d, want 400", rr.Code)
		}
	})

	t.Run("valid name without a live session returns no-session error", func(t *testing.T) {
		rr := call(`{"Name":"Alice"}`)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("no session: got %d, want 500 (body=%s)", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "session") {
			t.Errorf("expected a session-related error, got body=%s", rr.Body.String())
		}
	})
}
