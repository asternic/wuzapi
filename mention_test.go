package main

import (
	"context"
	"errors"
	"go.mau.fi/whatsmeow/types"
	"reflect"
	"testing"
)

func TestExtractMentionJIDs(t *testing.T) {
	got := extractMentionJIDs("Halo @6281234567890, tolong dibaca ya @6289876543210 dan @6281234567890 lagi")
	want := []string{"6281234567890@s.whatsapp.net", "6289876543210@s.whatsapp.net"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := extractMentionJIDs("tidak ada mention"); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestMergeMentionJIDs(t *testing.T) {
	got := mergeMentionJIDs([]string{"628999@s.whatsapp.net", "6281234567890@s.whatsapp.net"}, "Halo @6281234567890")
	want := []string{"628999@s.whatsapp.net", "6281234567890@s.whatsapp.net"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMentionTokenBoundaries(t *testing.T) {
	for _, text := range []string{
		"https://example.com/(@6281234567890)", "www.example.com/(@6281234567890)",
		"user@6281234567890.com", "https://example.com/@6281234567890",
		"https://example.com?q=@6281234567890", "@6281234567890abc",
		"@6281234567890_name", "@6281234567890.example", "@6281234567890/path",
		"@6281234567890@lid", "@6281234567890é", "name@6281234567890",
	} {
		t.Run(text, func(t *testing.T) {
			if got := extractMentionJIDs(text); len(got) != 0 {
				t.Fatalf("unexpected mentions: %v", got)
			}
		})
	}
	for _, text := range []string{
		"@6281234567890", "Hello (@6281234567890)!", "Hello @6281234567890.",
		"Hello @6281234567890. Next sentence", "“@6281234567890”", "\n@6281234567890\t",
	} {
		t.Run(text, func(t *testing.T) {
			want := []string{"6281234567890@s.whatsapp.net"}
			if got := extractMentionJIDs(text); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func TestMergeMentionJIDsPreservesExplicit(t *testing.T) {
	explicit := []string{"123@lid", "123@lid", "6281234567890@s.whatsapp.net"}
	want := []string{"123@lid", "6281234567890@s.whatsapp.net", "6289876543210@s.whatsapp.net"}
	got := mergeMentionJIDs(explicit, "@6289876543210, @6281234567890 user@123.com")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if explicit[1] != "123@lid" {
		t.Fatal("modified caller's mentions")
	}
}

func TestEveryoneToken(t *testing.T) {
	for _, text := range []string{"@everyone", "Hello @everyone!", "(@everyone)"} {
		if !everyoneToken.MatchString(text) {
			t.Errorf("missing token in %q", text)
		}
	}
	for _, text := range []string{"user@everyone.com", "https://example.com/@everyone", "@everyone_else"} {
		if everyoneToken.MatchString(text) {
			t.Errorf("unexpected token in %q", text)
		}
	}
}

func TestResolveAllMentions(t *testing.T) {
	group := types.NewJID("123", types.GroupServer)
	member := types.NewJID("456", types.DefaultUserServer)
	calls := 0
	lookup := func(context.Context, types.JID) (*types.GroupInfo, error) {
		calls++
		return &types.GroupInfo{Participants: []types.GroupParticipant{{JID: member}, {JID: member}}}, nil
	}
	got, err := resolveAllMentions(context.Background(), group, []string{"789@lid"}, "hello", true, lookup)
	if err != nil || !reflect.DeepEqual(got, []string{"789@lid", member.String()}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err = resolveAllMentions(context.Background(), member, nil, "@everyone", false, lookup); err == nil {
		t.Fatal("accepted direct chat")
	}
	if _, err = resolveAllMentions(context.Background(), group, nil, "ordinary text", false, lookup); err != nil || calls != 1 {
		t.Fatal("ordinary send performed lookup")
	}
	_, err = resolveAllMentions(context.Background(), group, nil, "@everyone", false, func(context.Context, types.JID) (*types.GroupInfo, error) { return nil, errors.New("offline") })
	if err == nil {
		t.Fatal("ignored lookup failure")
	}
}
