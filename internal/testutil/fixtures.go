package testutil

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// LoadFixture reads a file from internal/testutil/testdata.
func LoadFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("internal", "testutil", "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

// MustUnmarshalJSON decodes JSON into the provided target or fails the test.
func MustUnmarshalJSON(t *testing.T, data []byte, target any) {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}
