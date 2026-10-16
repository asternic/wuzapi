package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRespondEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		status int
		data   interface{}
		want   map[string]interface{}
	}{
		{
			name:   "group photo failure preserves cause",
			status: http.StatusInternalServerError,
			data:   errors.New("failed to set group photo: photo is too large"),
			want: map[string]interface{}{
				"code": float64(500), "success": false,
				"error": "failed to set group photo: photo is too large",
			},
		},
		{
			name:   "bad request",
			status: http.StatusBadRequest,
			data:   errors.New("invalid payload"),
			want: map[string]interface{}{
				"code": float64(400), "success": false, "error": "invalid payload",
			},
		},
		{
			name:   "successful object",
			status: http.StatusOK,
			data:   `{"id":"photo-id"}`,
			want: map[string]interface{}{
				"code": float64(200), "success": true,
				"data": map[string]interface{}{"id": "photo-id"},
			},
		},
		{
			name:   "successful array",
			status: http.StatusOK,
			data:   `["photo-id"]`,
			want: map[string]interface{}{
				"code": float64(200), "success": true,
				"data": []interface{}{"photo-id"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s := &server{}
			s.Respond(w, httptest.NewRequest(http.MethodPost, "/group/photo", nil), tt.status, tt.data)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d", w.Code, tt.status)
			}
			if got := w.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			var got map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("invalid response JSON: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("response = %#v, want %#v", got, tt.want)
			}
		})
	}
}
