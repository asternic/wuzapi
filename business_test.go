package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestBusinessProfileHandler(t *testing.T) {
	s := makeTestServer(t)
	for _, outcome := range []string{"success", "error", "missing"} {
		calls := 0
		key := struct{}{}
		handler := s.businessProfileHandler(func(ctx context.Context, user string, jid types.JID) (*types.BusinessProfile, error) {
			calls++
			if user != "business-user" || jid.String() != "5491112345678@s.whatsapp.net" || ctx.Value(key) != "preserved" {
				t.Fatal("incorrect lookup dispatch")
			}
			if outcome == "error" {
				return nil, errors.New("lookup failed")
			}
			if outcome == "missing" {
				return nil, nil
			}
			return &types.BusinessProfile{JID: jid, Address: "Main Street", Email: "support@example.com", BusinessHoursTimeZone: "America/Argentina/Buenos_Aires", ProfileOptions: map[string]string{"website": "https://example.com"}}, nil
		})
		for _, body := range []string{`{"Phone":"5491112345678"}`, `{}`, `{"Phone":"123@g.us"}`, `{"Phone":"123@newsletter"}`, `{"Phone":"5491112345678"} {}`, `{"Phone":"5491112345678","Unknown":true}`} {
			req := httptest.NewRequest("POST", "/user/business-profile", strings.NewReader(body))
			ctx := context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": "business-user"}})
			ctx = context.WithValue(ctx, key, "preserved")
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()
			handler(rr, req)
			want := 400
			if body == `{"Phone":"5491112345678"}` {
				want = 200
				if outcome == "error" {
					want = 500
				}
				if outcome == "missing" {
					want = 404
				}
			}
			if rr.Code != want {
				t.Fatalf("%s: %d %s", outcome, rr.Code, rr.Body.String())
			}
			if want == 200 {
				var response struct{ Data types.BusinessProfile }
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Data.Email != "support@example.com" || response.Data.Address != "Main Street" || response.Data.ProfileOptions["website"] != "https://example.com" {
					t.Fatalf("lost profile fields: %s", rr.Body.String())
				}
			}
		}
		if calls != 1 {
			t.Fatal("invalid request reached lookup")
		}
	}
}

func TestBusinessProfileRoute(t *testing.T) {
	testFeatureRoute(t, "/user/business-profile", "user.business-profile", `{"Phone":"5491112345678"}`)
}
