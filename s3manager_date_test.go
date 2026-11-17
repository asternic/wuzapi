package main

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateS3KeyDate(t *testing.T) {
	old := s3KeyTime
	s3KeyTime = func() time.Time { return time.Date(2024, time.March, 17, 12, 0, 0, 0, time.UTC) }
	defer func() { s3KeyTime = old }()
	key := (&S3Manager{}).GenerateS3Key("user", "contact", "message", "image/jpeg", true)
	if !strings.Contains(key, "/2024/03/17/") {
		t.Fatalf("key %q does not contain expected date", key)
	}
}
