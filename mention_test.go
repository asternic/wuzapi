package main

import (
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
