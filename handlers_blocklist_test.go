package main

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestResolveBlocklistPNJIDKeepsPhoneNumberJID(t *testing.T) {
	input := types.NewJID("5491155554445", types.DefaultUserServer)

	got, err := resolveBlocklistPNJID(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("resolveBlocklistPNJID returned error: %v", err)
	}
	if got != input {
		t.Fatalf("expected %s, got %s", input, got)
	}
}

func TestResolveBlocklistPNJIDNormalizesLegacyJID(t *testing.T) {
	input := types.NewJID("5491155554445", types.LegacyUserServer)
	expected := types.NewJID("5491155554445", types.DefaultUserServer)

	got, err := resolveBlocklistPNJID(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("resolveBlocklistPNJID returned error: %v", err)
	}
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestResolveBlocklistPNJIDRejectsUnsupportedServer(t *testing.T) {
	input := types.NewJID("123456", types.GroupServer)

	if _, err := resolveBlocklistPNJID(context.Background(), nil, input); err == nil {
		t.Fatal("expected unsupported server error")
	}
}
