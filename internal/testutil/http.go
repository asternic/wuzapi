package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// RequestCapture stores basic details about an HTTP request.
type RequestCapture struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

// NewJSONServer returns an httptest server that records requests and replies
// with the provided status and JSON body.
func NewJSONServer(status int, response any) (*httptest.Server, *RequestLog) {
	log := &RequestLog{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		log.add(RequestCapture{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    body,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	})
	return httptest.NewServer(handler), log
}

// RequestLog is a threadsafe in-memory log of captured requests.
type RequestLog struct {
	mu       sync.Mutex
	Requests []RequestCapture
}

func (l *RequestLog) add(req RequestCapture) {
	l.mu.Lock()
	l.Requests = append(l.Requests, req)
	l.mu.Unlock()
}

// Last returns the most recent request capture.
func (l *RequestLog) Last() (RequestCapture, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.Requests) == 0 {
		return RequestCapture{}, false
	}
	return l.Requests[len(l.Requests)-1], true
}

// Clear removes all captured requests.
func (l *RequestLog) Clear() {
	l.mu.Lock()
	l.Requests = nil
	l.mu.Unlock()
}

// NewRecorder returns a ResponseRecorder and request for handler tests.
func NewRecorder(method, path string, body []byte) (*httptest.ResponseRecorder, *http.Request) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	return rec, req
}

// DecodeJSONBody decodes a JSON body into target and closes the body.
func DecodeJSONBody(r *http.Response, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

// WithContext returns a copy of the request with the provided context.
func WithContext(r *http.Request, ctx context.Context) *http.Request {
	return r.WithContext(ctx)
}
