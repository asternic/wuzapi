package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

type fakeMessageLIDStore struct {
	pnForLID map[string]types.JID
	lidForPN map[string]types.JID
	err      error
}

func (f *fakeMessageLIDStore) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	if f.err != nil {
		return types.EmptyJID, f.err
	}
	return f.pnForLID[lid.User], nil
}

func (f *fakeMessageLIDStore) GetLIDForPN(_ context.Context, pn types.JID) (types.JID, error) {
	if f.err != nil {
		return types.EmptyJID, f.err
	}
	return f.lidForPN[pn.User], nil
}

func TestResolveMessageRecipient(t *testing.T) {
	ctx := context.Background()
	knownLID := "162182343999615"
	knownPN := "5491155554444"
	mappedLID := types.NewJID("987654321012345", types.HiddenUserServer)
	store := &fakeMessageLIDStore{
		pnForLID: map[string]types.JID{
			knownLID: types.NewJID("15551234567", types.DefaultUserServer),
		},
		lidForPN: map[string]types.JID{
			knownPN: mappedLID,
		},
	}

	tests := []struct {
		name  string
		input string
		want  types.JID
	}{
		{name: "explicit LID is preserved", input: knownLID + "@lid", want: types.NewJID(knownLID, types.HiddenUserServer)},
		{name: "explicit PN is preserved", input: knownPN + "@s.whatsapp.net", want: types.NewJID(knownPN, types.DefaultUserServer)},
		{name: "bare known LID", input: knownLID, want: types.NewJID(knownLID, types.HiddenUserServer)},
		{name: "bare known PN resolves to LID", input: knownPN, want: mappedLID},
		{name: "unknown bare ID falls back to PN", input: "123456789", want: types.NewJID("123456789", types.DefaultUserServer)},
		{name: "leading plus is removed", input: "+123456789", want: types.NewJID("123456789", types.DefaultUserServer)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveMessageRecipient(ctx, store, tc.input)
			if err != nil {
				t.Fatalf("resolveMessageRecipient() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveMessageRecipient() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestResolveMessageRecipientRejectsAmbiguousMapping(t *testing.T) {
	id := "123456789"
	store := &fakeMessageLIDStore{
		pnForLID: map[string]types.JID{id: types.NewJID("111", types.DefaultUserServer)},
		lidForPN: map[string]types.JID{id: types.NewJID("222", types.HiddenUserServer)},
	}

	_, err := resolveMessageRecipient(context.Background(), store, id)
	if err == nil {
		t.Fatal("resolveMessageRecipient() expected ambiguity error")
	}
}

func TestResolveMessageRecipientReturnsMappingError(t *testing.T) {
	store := &fakeMessageLIDStore{err: errors.New("database unavailable")}
	_, err := resolveMessageRecipient(context.Background(), store, "123456789")
	if err == nil {
		t.Fatal("resolveMessageRecipient() expected mapping error")
	}
}

func TestResolveMessageRecipientPreservesLegacyFormats(t *testing.T) {
	// A failed store proves explicit JIDs bypass PN/LID lookup. Keep the
	// existing server and device semantics instead of adding a server allowlist.
	brokenStore := &fakeMessageLIDStore{err: errors.New("store must not be queried")}
	for _, input := range []string{
		"5491155554444@s.whatsapp.net",
		"987654321012345@lid",
		"5491155554444@c.us",
		"120363000000000000@g.us",
		"123456789-123456789@g.us",
		"status@broadcast",
		"123456789@newsletter",
		"5491155554444:8@s.whatsapp.net",
		"5491155554444.0:8@s.whatsapp.net",
		"123@bot",
		"123@future.whatsapp.server",
		"+5491155554444@s.whatsapp.net",
	} {
		t.Run(input, func(t *testing.T) {
			want, ok := parseJID(input)
			if !ok {
				t.Fatal("invalid compatibility fixture")
			}
			got, err := resolveMessageRecipient(context.Background(), brokenStore, " \t"+input+"\n")
			if err != nil || got != want {
				t.Fatalf("got %s, %v; want %s", got, err, want)
			}
		})
	}
	for _, input := range []string{"1", "000123", "123456789012345678901234567890", "+5491155554444", " \t+5491155554444\n"} {
		t.Run(input, func(t *testing.T) {
			want, ok := parseJID(strings.TrimSpace(input))
			if !ok {
				t.Fatal("invalid compatibility fixture")
			}
			got, err := resolveMessageRecipient(context.Background(), nil, input)
			if err != nil || got != want {
				t.Fatalf("got %s, %v; want %s", got, err, want)
			}
		})
	}
}

func TestResolveMessageRecipientRejectsMalformedDestinations(t *testing.T) {
	for _, input := range []string{
		"", " \t\n", "+", "++123", "alice", "US.13491208655302741918",
		"123-456", "123 456", "(123)456", "123\n456", "１２３", "123\x00456",
		"@alice", "@lid", "123@", "123@lid@extra", "123@@lid",
		"123 @lid", "123@ lid", "123@li\td", "123\x00@lid", "123@li\u00a0d",
	} {
		t.Run(input, func(t *testing.T) {
			// Both paths must reject invalid destinations, including before a
			// mapping lookup could obscure the validation error.
			for _, store := range []messageLIDStore{nil, &fakeMessageLIDStore{err: errors.New("unexpected lookup")}} {
				got, err := resolveMessageRecipient(context.Background(), store, input)
				if err == nil || !got.IsEmpty() {
					t.Fatalf("expected an empty JID and validation error; got %s, %v", got, err)
				}
				if strings.Contains(err.Error(), "unexpected lookup") {
					t.Fatalf("invalid destination reached mapping store: %v", err)
				}
			}
		})
	}
}

// Exercise both transports with a client that cannot send. Invalid destinations
// must yield a payload error before any WhatsApp lookup or network operation.
func TestSendMessageRejectsMalformedDestinationsHTTPAndStdio(t *testing.T) {
	s := makeTestServer(t)
	const user, token = "recipient-validation-user", "recipient-validation-token"
	if _, err := s.db.Exec(`INSERT INTO users (id, name, token) VALUES ($1, $2, $3)`, user, "tester", token); err != nil {
		t.Fatal(err)
	}
	clientManager.SetWhatsmeowClient(user, whatsmeow.NewClient(&store.Device{}, nil))
	t.Cleanup(func() { clientManager.DeleteWhatsmeowClient(user) })
	for _, phone := range []string{"US.13491208655302741918", "@alice", "123@lid@extra", "123@", "123 456"} {
		t.Run(phone, func(t *testing.T) {
			params := map[string]interface{}{"Phone": phone, "Body": "Hello", "token": token}
			body, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/chat/send/text", strings.NewReader(string(body)))
			req.Header.Set("token", token)
			response := httptest.NewRecorder()
			s.router.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Phone") {
				t.Fatalf("HTTP: got %d: %s", response.Code, response.Body.String())
			}
			rpc := executeRequest(t, s, newRequest("invalid-destination", "chat.send.text", params).toJSON(t))
			rpcError := assertJSONRPC20Error(t, rpc, "invalid-destination", float64(http.StatusBadRequest))
			message, _ := rpcError["message"].(string)
			if !strings.Contains(message, "Phone") {
				t.Fatalf("stdio: unexpected error %v", rpcError)
			}
		})
	}
}
