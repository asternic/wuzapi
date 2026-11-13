package main

import "testing"

func TestCopyButtonParams(t *testing.T) {
	got := copyButtonParams("Copy", "copy-id", "code")
	if got["display_text"] != "Copy" || got["id"] != "copy-id" || got["copy_code"] != "code" {
		t.Fatalf("unexpected params: %v", got)
	}
}
