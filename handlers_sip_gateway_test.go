package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminUserSIPGateway(t *testing.T) {
	s := makeTestServer(t)
	if _, err := s.db.Exec("INSERT INTO users (id, name, token) VALUES ('sip-test', 'SIP', 'sip-test-token')"); err != nil {
		t.Fatal(err)
	}

	const target = "sip:wa-in@pbx.example.com:5060"
	for _, tc := range []struct {
		method, token, payload, want string
		status                       int
	}{
		{"GET", "", "", "", 401},
		{"GET", *adminToken, "", `"enabled":false`, 200},
		{"PUT", *adminToken, `{"enabled":true,"target":"https://example.com"}`, "", 400},
		{"PUT", *adminToken, `{"enabled":true,"target":"` + target + `"}`, target, 200},
		{"GET", *adminToken, "", `"maxCalls":2`, 200},
		{"PUT", *adminToken, `{"enabled":true,"target":"` + target + `","maxCalls":33}`, "", 400},
		{"PUT", *adminToken, `{"enabled":true,"target":"` + target + `","maxCalls":8}`, `"maxCalls":8`, 200},
		{"PUT", *adminToken, `{"enabled":true,"target":"` + target + `","allowOutbound":true}`, `"allowOutbound":true`, 200},
		{"GET", *adminToken, "", `"allowOutbound":true`, 200},
		{"GET", *adminToken, "", `"maxCalls":8`, 200},
		{"PUT", *adminToken, `{"enabled":false}`, `"enabled":false`, 200},
		{"GET", *adminToken, "", `"allowOutbound":false`, 200},
	} {
		req := httptest.NewRequest(tc.method, "/admin/users/sip-test/sip-gateway", strings.NewReader(tc.payload))
		req.Header.Set("Authorization", tc.token)
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s %s: got %d %s", tc.method, tc.payload, rec.Code, rec.Body.String())
		}
	}
}
