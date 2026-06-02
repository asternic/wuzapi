package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRespondEnvelope covers the Respond fixes: a non-string payload must not
// panic (was an unchecked data.(string)), and a plain non-JSON string passed on
// an error status must report success=false with the message preserved (was
// success=true with the message dropped). Valid-JSON success responses are
// unchanged.
func TestRespondEnvelope(t *testing.T) {
	s := makeTestServer(t)

	respond := func(status int, data interface{}) map[string]interface{} {
		rr := httptest.NewRecorder()
		s.Respond(rr, httptest.NewRequest(http.MethodGet, "/", nil), status, data)
		var env map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("response is not valid JSON: %s", rr.Body.String())
		}
		return env
	}

	t.Run("error value -> success=false with error", func(t *testing.T) {
		env := respond(http.StatusInternalServerError, errors.New("boom"))
		if env["success"] != false || env["error"] != "boom" {
			t.Errorf("got %v, want success=false error=boom", env)
		}
	})

	t.Run("non-JSON string on error status -> success=false, message kept", func(t *testing.T) {
		env := respond(http.StatusInternalServerError, "failed to set group name")
		if env["success"] != false {
			t.Errorf("success = %v, want false (env=%v)", env["success"], env)
		}
		if env["error"] == nil && env["data"] == nil {
			t.Errorf("error message was dropped: %v", env)
		}
	})

	t.Run("valid JSON string on 200 -> success=true, data parsed", func(t *testing.T) {
		env := respond(http.StatusOK, `{"Details":"ok"}`)
		if env["success"] != true {
			t.Errorf("success = %v, want true", env["success"])
		}
		d, ok := env["data"].(map[string]interface{})
		if !ok || d["Details"] != "ok" {
			t.Errorf("data not parsed: %v", env)
		}
	})

	t.Run("non-string, non-error value -> no panic, data set", func(t *testing.T) {
		env := respond(http.StatusOK, map[string]string{"k": "v"}) // previously panicked
		if env["success"] != true || env["data"] == nil {
			t.Errorf("got %v, want success=true with data", env)
		}
	})
}
