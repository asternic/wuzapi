package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"go.mau.fi/whatsmeow/types/events"
)

// Label events are forwarded to the user webhook when subscribed, and full-sync
// snapshots are not.
func TestLabelEventsAreDelivered(t *testing.T) {
	cases := []struct {
		want string
		evt  interface{}
		skip bool
	}{
		{"LabelEdit", &events.LabelEdit{LabelID: "1"}, false},
		{"LabelAssociationChat", &events.LabelAssociationChat{LabelID: "1"}, false},
		{"LabelAssociationMessage", &events.LabelAssociationMessage{LabelID: "1", MessageID: "m"}, false},
		{"LabelEdit", &events.LabelEdit{LabelID: "1", FromFullSync: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw := ""
				if r.Header.Get("Content-Type") == "application/json" {
					body, _ := io.ReadAll(r.Body)
					var outer map[string]interface{}
					_ = json.Unmarshal(body, &outer)
					raw, _ = outer["jsonData"].(string)
					if raw == "" {
						raw = string(body)
					}
				} else {
					_ = r.ParseForm()
					raw = r.PostForm.Get("jsonData")
				}
				var m map[string]interface{}
				_ = json.Unmarshal([]byte(raw), &m)
				typ, _ := m["type"].(string)
				got <- typ
			}))
			defer srv.Close()
			userinfocache.Set("appstate-token", Values{m: map[string]string{"Name": "i", "Events": "LabelEdit,LabelAssociationChat,LabelAssociationMessage", "Webhook": srv.URL}}, 0)
			defer userinfocache.Delete("appstate-token")
			clientManager.SetHTTPClient("appstate-user", resty.New())
			defer clientManager.DeleteHTTPClient("appstate-user")
			(&MyClient{userID: "appstate-user", token: "appstate-token"}).myEventHandler(tc.evt)
			select {
			case typ := <-got:
				if tc.skip {
					t.Fatalf("full sync snapshot delivered as %s", typ)
				}
				if typ != tc.want {
					t.Fatalf("type = %q, want %q", typ, tc.want)
				}
			case <-time.After(1500 * time.Millisecond):
				if !tc.skip {
					t.Fatal("event not delivered")
				}
			}
		})
	}
}
