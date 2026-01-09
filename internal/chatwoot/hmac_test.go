package chatwoot

import "testing"

func TestIdentifierHash(t *testing.T) {
	got := IdentifierHash("5516997927255", "4yBZPWYMnHfbFTUHzvLJ77Dj")
	want := "71baff44f50fbfa0f9673ca990bfa4327053308c5a86a6cd2ff6838aedc798cf"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
