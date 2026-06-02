package main

import (
	"strings"
	"testing"
	"time"
)

// TestGenerateS3KeyDateFormat verifies the fix for the S3 key date path: the
// year/month/day segments must use valid Go layout tokens ("2006"/"01"/"02").
// The previous code used "2025"/"05"/"25", which are not valid tokens ("05" is
// the seconds field), producing a 5-digit "year" and nonsensical month/day.
func TestGenerateS3KeyDateFormat(t *testing.T) {
	m := &S3Manager{}
	key := m.GenerateS3Key("user-1", "123456@s.whatsapp.net", "MSGID", "image/jpeg", true)

	// Layout: users/{userID}/{direction}/{contactJID}/{year}/{month}/{day}/{mediaType}/{messageID}{ext}
	parts := strings.Split(key, "/")
	if len(parts) != 9 {
		t.Fatalf("unexpected key shape %q (%d segments, want 9)", key, len(parts))
	}
	year, month, day := parts[4], parts[5], parts[6]

	now := time.Now()
	if want := now.Format("2006"); year != want {
		t.Errorf("year segment = %q, want %q (key=%q)", year, want, key)
	}
	if want := now.Format("01"); month != want {
		t.Errorf("month segment = %q, want %q (key=%q)", month, want, key)
	}
	if want := now.Format("02"); day != want {
		t.Errorf("day segment = %q, want %q (key=%q)", day, want, key)
	}

	// Regression guard independent of the current date: the old bug produced a
	// 5-digit "year" and a "month" rendered from seconds. Enforce exact widths.
	if len(year) != 4 {
		t.Errorf("year segment %q must be exactly 4 digits (key=%q)", year, key)
	}
	if len(month) != 2 || len(day) != 2 {
		t.Errorf("month/day segments must be 2 digits, got %q/%q (key=%q)", month, day, key)
	}
}
