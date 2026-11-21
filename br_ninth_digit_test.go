package main

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestBrazilianVariants(t *testing.T) {
	cases := map[string][]string{
		"5511987654321": {"5511987654321", "551187654321"},
		"551187654321":  {"551187654321", "5511987654321"},
		"551132221234":  nil, // landline
		"5511932221234": nil, // 9 + landline-like digit
		"14155550100":   nil,
		"5501987654321": nil,
	}
	for in, want := range cases {
		got := brazilianVariants(in)
		if len(got) != len(want) || (len(got) == 2 && (got[0] != want[0] || got[1] != want[1])) {
			t.Errorf("%s: got %v want %v", in, got, want)
		}
	}
}

func TestResolveBrazilianNumber(t *testing.T) {
	brNumberCache.Flush()
	calls := 0
	onlyOld := func(_ context.Context, q []string) ([]types.IsOnWhatsAppResponse, error) {
		calls++
		var out []types.IsOnWhatsAppResponse
		for _, v := range q {
			user := v[1:]
			out = append(out, types.IsOnWhatsAppResponse{Query: v, JID: types.NewJID(user, types.DefaultUserServer), IsIn: user == "551187654321"})
		}
		return out, nil
	}
	got := resolveBrazilianNumber(context.Background(), onlyOld, types.NewJID("5511987654321", types.DefaultUserServer))
	if got.User != "551187654321" {
		t.Fatalf("13-digit form not corrected: %s", got)
	}
	// Both forms are now cached: no further queries.
	resolveBrazilianNumber(context.Background(), onlyOld, types.NewJID("551187654321", types.DefaultUserServer))
	resolveBrazilianNumber(context.Background(), onlyOld, types.NewJID("5511987654321", types.DefaultUserServer))
	if calls != 1 {
		t.Fatalf("expected 1 query, got %d", calls)
	}

	brNumberCache.Flush()
	failing := func(context.Context, []string) ([]types.IsOnWhatsAppResponse, error) {
		return nil, errors.New("offline")
	}
	in := types.NewJID("5511999990000", types.DefaultUserServer)
	if got := resolveBrazilianNumber(context.Background(), failing, in); got != in {
		t.Fatalf("failure must keep the given number, got %s", got)
	}
	none := func(_ context.Context, q []string) ([]types.IsOnWhatsAppResponse, error) { return nil, nil }
	if got := resolveBrazilianNumber(context.Background(), none, in); got != in {
		t.Fatalf("unknown number must be kept, got %s", got)
	}
	// Current WhatsApp behavior: a single answer carrying only the LID.
	brNumberCache.Flush()
	accountLID := types.NewJID("100000000000001", types.HiddenUserServer)
	lidOnly := func(_ context.Context, q []string) ([]types.IsOnWhatsAppResponse, error) {
		return []types.IsOnWhatsAppResponse{{Query: q[0], JID: accountLID, IsIn: true}}, nil
	}
	if got := resolveBrazilianNumber(context.Background(), lidOnly, types.NewJID("5511987654321", types.DefaultUserServer)); got != accountLID {
		t.Fatalf("LID answer must be used, got %s", got)
	}
	if got := resolveBrazilianNumber(context.Background(), lidOnly, types.NewJID("551187654321", types.DefaultUserServer)); got != accountLID {
		t.Fatalf("cached LID must serve the other form, got %s", got)
	}
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	if got := resolveBrazilianNumber(context.Background(), onlyOld, lid); got != lid {
		t.Fatalf("LID must be untouched, got %s", got)
	}
}
