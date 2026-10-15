package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestParseGroupMemberAddMode(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		allowEmpty bool
		want       types.GroupMemberAddMode
		wantOK     bool
	}{
		{name: "admin only", mode: "admin_add", want: types.GroupMemberAddModeAdmin, wantOK: true},
		{name: "all members", mode: "all_member_add", want: types.GroupMemberAddModeAllMember, wantOK: true},
		{name: "empty allowed on create", mode: "", allowEmpty: true, want: "", wantOK: true},
		{name: "empty rejected on update", mode: "", allowEmpty: false, want: "", wantOK: false},
		{name: "unknown value", mode: "admins", allowEmpty: true, want: "", wantOK: false},
		{name: "case sensitive", mode: "ADMIN_ADD", allowEmpty: true, want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseGroupMemberAddMode(tt.mode, tt.allowEmpty)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("parseGroupMemberAddMode(%q, %v) = (%q, %v), want (%q, %v)",
					tt.mode, tt.allowEmpty, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
