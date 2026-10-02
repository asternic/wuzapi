package main

import (
	"context"
	"errors"
	"testing"

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
