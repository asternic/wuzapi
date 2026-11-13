package main

import "testing"

func TestNativeButtonParams(t *testing.T) {
	params := `{"payment_settings":[{"type":"example"}]}`
	name, got, err := nativeButtonParams("payment_info", params)
	if err != nil || name != "payment_info" || got != params {
		t.Fatalf("unexpected result: %q %q %v", name, got, err)
	}
	for _, input := range []struct{ name, params string }{{"", `{}`}, {"x", `{`}, {"x", `null`}, {"x", `[]`}} {
		if _, _, err := nativeButtonParams(input.name, input.params); err == nil {
			t.Fatalf("accepted invalid input: %+v", input)
		}
	}
}
