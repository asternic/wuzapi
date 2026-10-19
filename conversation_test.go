package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/appstate"
)

func TestConversationPatches(t *testing.T) {
	for _, body := range []string{`{"Chat":"123@g.us","Mute":true}`, `{"Chat":"456@lid","Mute":false}`, `{"Chat":"123","Mute":true,"DurationSeconds":3600}`} {
		patch, err := mutePatch(strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		action := patch.Mutations[0].Value.GetMuteAction()
		if strings.Contains(body, `3600`) && action.GetMuteEndTimestamp() < time.Now().Add(59*time.Minute).UnixMilli() {
			t.Fatal("wrong mute expiry")
		}
		if body == `{"Chat":"123@g.us","Mute":true}` && action.GetMuteEndTimestamp() != -1 {
			t.Fatal("not indefinite")
		}
	}
	for _, unread := range []string{"true", "false"} {
		patch, err := unreadPatch(strings.NewReader(`{"Chat":"123@g.us","Unread":` + unread + `}`))
		if err != nil {
			t.Fatal(err)
		}
		if patch.Mutations[0].Value.GetMarkChatAsReadAction().GetRead() != (unread == "false") {
			t.Fatal("inverted unread state")
		}
	}
	for _, body := range []string{`null`, `{}`, `{"Chat":"123@newsletter","Mute":true}`, `{"Chat":"123@g.us","Mute":true,"DurationSeconds":-1}`, `{"Chat":"123@g.us","Mute":true,"DurationSeconds":9223372036854775807}`, `{"Chat":"123@g.us","Mute":false,"DurationSeconds":1}`, `{"Chat":"123@g.us","Mute":true} {}`, `{"Chat":"123@g.us","Mute":true,"Typo":1}`} {
		if _, err := mutePatch(strings.NewReader(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	if _, err := unreadPatch(strings.NewReader(`{"Chat":"123@g.us"}`)); err == nil {
		t.Fatal("missing Unread accepted")
	}
}

func TestAppStateHandler(t *testing.T) {
	s := makeTestServer(t)
	for _, fail := range []bool{false, true} {
		calls := 0
		handler := s.appStateHandler(mutePatch, func(ctx context.Context, user string, patch appstate.PatchInfo) error {
			calls++
			if user != "test-user" || patch.Mutations[0].Index[1] != "123@g.us" {
				t.Fatal("incorrect dispatch")
			}
			if fail {
				return errors.New("send failed")
			}
			return nil
		})
		for _, body := range []string{`{"Chat":"123@g.us","Mute":true}`, `{}`} {
			req := httptest.NewRequest("POST", "/chat/mute", strings.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": "test-user"}}))
			rr := httptest.NewRecorder()
			handler(rr, req)
			want := 200
			if fail {
				want = 500
			}
			if body == `{}` {
				want = 400
			}
			if rr.Code != want {
				t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
			}
		}
		if calls != 1 {
			t.Fatal("invalid request dispatched")
		}
	}
}

func testFeatureRoute(t *testing.T, path, method, body string) {
	t.Helper()
	s := makeTestServer(t)
	if _, err := s.db.Exec(`INSERT INTO users (id,name,token,connected) VALUES ('feature-user','tester','feature-token',0)`); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "feature-token"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("token", token)
		rr := httptest.NewRecorder()
		s.router.ServeHTTP(rr, req)
		if token == "" {
			if rr.Code != 401 {
				t.Fatalf("auth: %d %s", rr.Code, rr.Body.String())
			}
		} else if rr.Code != 500 || !strings.Contains(rr.Body.String(), "no session") {
			t.Fatalf("route: %d %s", rr.Code, rr.Body.String())
		}
	}
	params := map[string]interface{}{}
	if err := json.Unmarshal([]byte(body), &params); err != nil {
		t.Fatal(err)
	}
	params["token"] = "feature-token"
	response := executeRequest(t, s, newRequest("feature", method, params).toJSON(t))
	if response["error"] == nil || !strings.Contains(response["error"].(map[string]interface{})["message"].(string), "no session") {
		t.Fatalf("stdio mapping: %v", response)
	}

}

func TestConversationRoutes(t *testing.T) {
	testFeatureRoute(t, "/chat/mute", "chat.mute", `{"Chat":"123@g.us","Mute":true}`)
	testFeatureRoute(t, "/chat/unread", "chat.unread", `{"Chat":"123@g.us","Unread":true}`)
}
