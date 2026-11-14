package main

import "testing"

func TestVoiceCallParams(t *testing.T) {
	got := voiceCallParams("Call")
	if len(got) != 1 || got["display_text"] != "Call" {
		t.Fatalf("unexpected params: %v", got)
	}
}
