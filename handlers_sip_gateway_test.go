package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminUserSIPGateway(t *testing.T) {
	s := makeTestServer(t)
	if _, err := s.db.Exec("INSERT INTO users (id, name, token) VALUES ('sip-test', 'SIP', 'sip-test-token')"); err != nil {
		t.Fatal(err)
	}

	previousKey := *globalEncryptionKey
	*globalEncryptionKey = "01234567890123456789012345678901"
	t.Cleanup(func() { *globalEncryptionKey = previousKey })

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
		{"PUT", *adminToken, `{"enabled":true,"target":"` + target + `","authUsername":"wa-test","authPassword":"test-sip-password"}`, `"hasAuthPassword":true`, 200},
		{"PUT", *adminToken, `{"enabled":true,"target":"` + target + `"}`, `"authUsername":"wa-test"`, 200},
		{"GET", *adminToken, "", `"hasAuthPassword":true`, 200},
		{"GET", *adminToken, "", `"maxCalls":8`, 200},
		{"PUT", *adminToken, `{"enabled":false}`, `"enabled":false`, 200},
		{"GET", *adminToken, "", `"allowOutbound":false`, 200},
	} {
		req := httptest.NewRequest(tc.method, "/admin/users/sip-test/sip-gateway", strings.NewReader(tc.payload))
		req.Header.Set("Authorization", tc.token)
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "test-sip-password") {
			t.Fatal("SIP password exposed in response")
		}
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s %s: got %d %s", tc.method, tc.payload, rec.Code, rec.Body.String())
		}
	}
}

func TestSIPAuthRoundTrip(t *testing.T) {
	s := makeTestServer(t)
	old := *globalEncryptionKey
	*globalEncryptionKey = "01234567890123456789012345678901"
	defer func() { *globalEncryptionKey = old }()
	_, err := s.db.Exec("INSERT INTO users (id, name, token) VALUES ('sip-auth', 'SIP', 'test-token')")
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{
		`{"enabled":false,"authUsername":"wa-test","authPassword":"test-sip-password"}`,
		`{"enabled":false}`,
		`{"enabled":false,"authUsername":"","authPassword":""}`,
	} {
		req := httptest.NewRequest("PUT", "/admin/users/sip-auth/sip-gateway", strings.NewReader(payload))
		req.Header.Set("Authorization", *adminToken)
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
		auth, err := s.sipAuth(context.Background(), "sip-auth")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload, `"authUsername":""`) {
			if auth.Username != "" || auth.Password != "" {
				t.Fatal("credentials not cleared")
			}
		} else if auth.Username != "wa-test" || auth.Password != "test-sip-password" {
			t.Fatal("credentials not preserved")
		}
		var stored string
		if err := s.db.Get(&stored, "SELECT sip_gateway_auth_password FROM users WHERE id = 'sip-auth'"); err != nil {
			t.Fatal(err)
		}
		if stored == "test-sip-password" {
			t.Fatal("password stored without encryption")
		}
	}
}
