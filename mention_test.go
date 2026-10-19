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
