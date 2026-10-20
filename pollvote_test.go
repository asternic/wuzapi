package main

import (
	"bytes"
	"context"
	"errors"
	"go.mau.fi/whatsmeow/store"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func TestPollVoteHandler(t *testing.T) {
	s := makeTestServer(t)
	for _, fail := range []bool{false, true} {
		calls := 0
		handler := s.pollVoteHandler(func(ctx context.Context, user string, info *types.MessageInfo, options []string) (whatsmeow.SendResponse, error) {
			calls++
			if user != "poll-user" || info.ID != "ABC" || info.Chat.String() != "123@g.us" || info.Sender.String() != "456@lid" || !info.IsGroup || info.IsFromMe || len(options) != 1 || options[0] != "Yes" {
				t.Fatal("incorrect vote dispatch")
			}
			if fail {
				return whatsmeow.SendResponse{}, errors.New("missing secret")
			}
			return whatsmeow.SendResponse{ID: "VOTE-ID", Timestamp: time.Unix(1234, 0)}, nil
		})
		for _, body := range []string{`{"Chat":"123@g.us","Sender":"456@lid","Id":"ABC","FromMe":false,"Options":["Yes"]}`, `{"Chat":"123@g.us","Sender":"456@lid","Id":"ABC","FromMe":false}`, `{"Chat":"123@g.us","Sender":"456@lid","Id":"ABC","FromMe":false,"Options":["Yes","Yes"]}`} {
			req := httptest.NewRequest("POST", "/chat/poll/vote", strings.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": "poll-user"}}))
			rr := httptest.NewRecorder()
			handler(rr, req)
			want := 400
			if strings.Contains(body, `"Options":["Yes"]`) {
				want = 200
				if fail {
					want = 500
				}
			}
			if rr.Code != want {
				t.Fatalf("%d: %s", rr.Code, rr.Body.String())
			}
			if want == 200 && !strings.Contains(rr.Body.String(), "VOTE-ID") {
				t.Fatal("missing vote ID")
			}
		}
		if calls != 1 {
			t.Fatal("invalid request sent")
		}
	}
}

func TestPollVoteValidation(t *testing.T) {
	for _, body := range []string{`{"Chat":"123","Sender":"456","Id":"ABC","FromMe":true,"Options":[]}`, `{"Chat":"123@lid","Sender":"456@lid","Id":"ABC","FromMe":false,"Options":["Yes"]}`} {
		var r pollVoteRequest
		if err := decodeAPIRequest(strings.NewReader(body), &r); err != nil {
			t.Fatal(err)
		}
		if _, err := r.messageInfo(); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`{}`, `{"Chat":"123@g.us","Sender":"456@g.us","Id":"ABC","FromMe":false,"Options":[]}`, `{"Chat":"123@g.us","Sender":"456","Id":"ABC","Options":[]}`, `{"Chat":"123@g.us","Sender":"456","Id":"ABC","FromMe":false,"Options":null}`, `{"Chat":"123@g.us","Sender":"456","Id":"ABC","FromMe":false,"Options":[" "]}`} {
		var r pollVoteRequest
		if err := decodeAPIRequest(strings.NewReader(body), &r); err != nil {
			t.Fatal(err)
		}
		if _, err := r.messageInfo(); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestPollVoteRoute(t *testing.T) {
	testFeatureRoute(t, "/chat/poll/vote", "chat.poll.vote", `{"Chat":"123@g.us","Sender":"456@lid","Id":"ABC","FromMe":false,"Options":["Yes"]}`)
}

// Exercise the pinned Whatsmeow implementation, not a reimplementation of encryption.
type voteSecretStore struct {
	store.MsgSecretStore
	secret       []byte
	chat, sender types.JID
	id           string
}

func (s *voteSecretStore) GetMessageSecret(ctx context.Context, chat, sender types.JID, id string) ([]byte, types.JID, error) {
	s.chat = chat
	s.sender = sender
	s.id = id
	return s.secret, sender, nil
}
func TestPollVoteEncryption(t *testing.T) {
	own := types.NewJID("111", types.DefaultUserServer)
	secrets := &voteSecretStore{secret: bytes.Repeat([]byte{7}, 32)}
	client := &whatsmeow.Client{Store: &store.Device{ID: &own, LID: types.NewJID("222", types.HiddenUserServer), MsgSecrets: secrets}}
	for _, server := range []string{types.DefaultUserServer, types.HiddenUserServer} {
		info := &types.MessageInfo{MessageSource: types.MessageSource{Chat: types.NewJID("123", types.GroupServer), Sender: types.NewJID("456", server), IsGroup: true}, ID: "POLL-ID"}
		for _, options := range [][]string{{"Yes"}, {}} {
			message, err := client.BuildPollVote(context.Background(), info, options)
			if err != nil {
				t.Fatal(err)
			}
			update := message.GetPollUpdateMessage()
			if secrets.chat != info.Chat || secrets.sender != info.Sender || secrets.id != info.ID {
				t.Fatal("wrong secret lookup")
			}
			key := update.GetPollCreationMessageKey()
			if key.GetID() != info.ID || key.GetParticipant() != info.Sender.String() || key.GetRemoteJID() != info.Chat.String() || len(update.GetVote().GetEncPayload()) == 0 || len(update.GetVote().GetEncIV()) != 12 {
				t.Fatal("invalid encrypted vote")
			}
		}
	}
	secrets.secret = nil
	if _, err := client.BuildPollVote(context.Background(), &types.MessageInfo{MessageSource: types.MessageSource{Chat: own, Sender: own}, ID: "missing"}, []string{"Yes"}); !errors.Is(err, whatsmeow.ErrOriginalMessageSecretNotFound) {
		t.Fatalf("missing secret: %v", err)
	}
}
