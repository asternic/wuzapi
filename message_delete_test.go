package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

func TestParseRevokeSender(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", ""}, {"5491155553934@s.whatsapp.net", "5491155553934@s.whatsapp.net"},
		{"123456789@lid", "123456789@lid"}, {"5491155553934@c.us", "5491155553934@s.whatsapp.net"},
		{"5491155553934:12@s.whatsapp.net", "5491155553934@s.whatsapp.net"},
		{"123456789:12@lid", "123456789@lid"}, {"123456789.1:12@lid", "123456789@lid"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseRevokeSender(tc.input)
			if err != nil || got.String() != tc.want {
				t.Fatalf("got %s, %v; want %s", got, err, tc.want)
			}
		})
	}
	for _, input := range []string{"not-a-jid", "5491155553934", "@s.whatsapp.net", "123@", "123@s.whatsapp.net@evil", "123@g.us", "123@broadcast", "123@newsletter", "123@unknown", "abc@lid", "+123@s.whatsapp.net", " 123@lid", "123@lid ", " ", "123:-1@lid", "123:65536@lid", "123.256:1@lid", "123:abc@lid", "123:1:2@lid", "123.@lid", "123\n@lid"} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseRevokeSender(input); err == nil {
				t.Fatalf("accepted malformed SenderJID %q", input)
			}
		})
	}
}
func deleteTestClient() *whatsmeow.Client {
	own := types.NewJID("5491155553934", types.DefaultUserServer)
	return whatsmeow.NewClient(&store.Device{ID: &own, LID: types.NewJID("123456789", types.HiddenUserServer)}, nil)
}
func TestBuildDeleteMessage(t *testing.T) {
	client := deleteTestClient()
	for _, server := range []string{types.DefaultUserServer, types.HiddenUserServer, types.GroupServer} {
		chat := types.NewJID("999", server)
		for _, sender := range []string{"", "5491155553934@s.whatsapp.net", "5491155553934:12@s.whatsapp.net", "123456789@lid", "5491155553934@c.us"} {
			message, err := buildDeleteMessage(client, chat, sender, "message-id")
			if err != nil {
				t.Fatal(err)
			}
			key := message.GetProtocolMessage().GetKey()
			if !key.GetFromMe() || key.GetParticipant() != "" || key.GetID() != "message-id" || key.GetRemoteJID() != chat.String() {
				t.Fatalf("own-message contract changed: %v", key)
			}
		}
		for _, sender := range []string{"5491199999999@s.whatsapp.net", "987654321:5@lid"} {
			message, err := buildDeleteMessage(client, chat, sender, "message-id")
			if server != types.GroupServer {
				if err == nil {
					t.Fatal("accepted another sender outside a group")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			want, _ := parseRevokeSender(sender)
			key := message.GetProtocolMessage().GetKey()
			if key.GetFromMe() || key.GetParticipant() != want.String() {
				t.Fatalf("group revoke: %v", key)
			}
		}
	}
}
func TestDeleteMessageRejectsInvalidSenderBeforeSending(t *testing.T) {
	const user = "delete-validation-test"
	clientManager.SetWhatsmeowClient(user, deleteTestClient())
	defer clientManager.DeleteWhatsmeowClient(user)
	for _, tc := range []struct{ phone, sender string }{
		{"120363000000@g.us", "not-a-jid"}, {"120363000000@g.us", "123@lid@evil"},
		{"120363000000@g.us", "123@g.us"}, {"120363000000@g.us", "123:65536@lid"},
		{"5491100000000", "987654321@lid"},
	} {
		body, _ := json.Marshal(map[string]string{"Phone": tc.phone, "SenderJID": tc.sender, "Id": "test"})
		req := httptest.NewRequest(http.MethodPost, "/chat/delete", strings.NewReader(string(body)))
		req = req.WithContext(context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": user}}))
		response := httptest.NewRecorder()
		(&server{}).DeleteMessage()(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%q got %d: %s", tc.sender, response.Code, response.Body.String())
		}
		var result struct {
			Success bool
			Error   string
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Success || !strings.Contains(result.Error, "SenderJID") {
			t.Fatalf("unexpected error payload: %s", response.Body.String())
		}
	}
}
