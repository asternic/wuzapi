package main

import "testing"

func TestParseOGFetchProxy(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "unset disables the proxy", raw: "", want: ""},
		{name: "blanks disable the proxy", raw: "   ", want: ""},
		{name: "http proxy", raw: "http://10.0.0.5:3128", want: "http://10.0.0.5:3128"},
		{name: "hostname proxy", raw: "http://proxy.internal:8080", want: "http://proxy.internal:8080"},
		{name: "credentials are preserved", raw: "http://user:pass@10.0.0.5:3128", want: "http://user:pass@10.0.0.5:3128"},
		{name: "surrounding blanks are trimmed", raw: "  http://10.0.0.5:3128  ", want: "http://10.0.0.5:3128"},
		{name: "missing scheme is rejected", raw: "10.0.0.5:3128", wantErr: true},
		{name: "missing host is rejected", raw: "http://", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOGFetchProxy(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %v", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
			if tc.want == "" {
				if got != nil {
					t.Fatalf("expected no proxy for %q, got %v", tc.raw, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected proxy %q, got none", tc.want)
			}
			if got.String() != tc.want {
				t.Fatalf("expected proxy %q, got %q", tc.want, got.String())
			}
		})
	}
}

// A malformed value must not take the server down: startup keeps going with
// previews fetched directly, which is the pre-existing behaviour.
func TestParseOGFetchProxyRejectsWithoutPanicking(t *testing.T) {
	got, err := parseOGFetchProxy("://not-a-url")
	if err == nil {
		t.Fatalf("expected an error, got %v", got)
	}
	if got != nil {
		t.Fatalf("expected no proxy alongside the error, got %v", got)
	}
}
