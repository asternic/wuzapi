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

// Chat state and contact events are forwarded to the user webhook when subscribed, and full-sync
// snapshots are not.
func TestChatStateEventsAreDelivered(t *testing.T) {
	cases := []struct {
		want string
		evt  interface{}
		skip bool
	}{
		{"Archive", &events.Archive{}, false},
		{"Pin", &events.Pin{}, false},
		{"Mute", &events.Mute{}, false},
		{"Star", &events.Star{}, false},
		{"DeleteChat", &events.DeleteChat{}, false},
		{"ClearChat", &events.ClearChat{}, false},
		{"MarkChatAsRead", &events.MarkChatAsRead{}, false},
		{"DeleteForMe", &events.DeleteForMe{}, false},
		{"Contact", &events.Contact{}, false},
		{"PushName", &events.PushName{}, false},
		{"BusinessName", &events.BusinessName{}, false},
		{"Pin", &events.Pin{FromFullSync: true}, true},
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
			userinfocache.Set("appstate-token", Values{m: map[string]string{"Name": "i", "Events": "Archive,Pin,Mute,Star,DeleteChat,ClearChat,MarkChatAsRead,DeleteForMe,Contact,PushName,BusinessName", "Webhook": srv.URL}}, 0)
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
