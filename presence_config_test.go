package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestParseAutomaticPresence(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    types.Presence
		wantErr bool
	}{
		{name: "available", value: "available", want: types.PresenceAvailable},
		{name: "unavailable", value: "unavailable", want: types.PresenceUnavailable},
		{name: "normalizes case and whitespace", value: "  UNAVAILABLE  ", want: types.PresenceUnavailable},
		{name: "rejects unknown value", value: "offline", wantErr: true},
		{name: "rejects empty value", value: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAutomaticPresence(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseAutomaticPresence(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseAutomaticPresence(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
