package main

import "testing"

// TestResolveMediaFileName covers the filename surfaced in webhook/S3 payloads
// for incoming media (issue #287). Documents carry a real FileName that must be
// preferred over the messageID-based temp name; other media fall back to the
// temp base; and any directory component a sender embeds must be stripped.
func TestResolveMediaFileName(t *testing.T) {
	cases := []struct {
		name     string
		original string
		fallback string
		want     string
	}{
		{"document keeps real name", "Quarterly Report.xlsx", "/tmp/user_x/3EB0C942.xlsx", "Quarterly Report.xlsx"},
		{"empty falls back to temp base", "", "/tmp/user_x/3EB0C942.xlsx", "3EB0C942.xlsx"},
		{"path components are stripped", "../../etc/passwd", "/tmp/x/abc.bin", "passwd"},
		{"windows path components stripped", `..\..\secret.txt`, "/tmp/x/abc.bin", "secret.txt"},
		{"plain name with empty fallback", "plain.pdf", "", "plain.pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveMediaFileName(c.original, c.fallback); got != c.want {
				t.Errorf("resolveMediaFileName(%q, %q) = %q, want %q", c.original, c.fallback, got, c.want)
			}
		})
	}
}
