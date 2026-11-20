package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestChatwootAttachmentURLRequiresAllowlistedHTTPSHost(t *testing.T) {
	allowed := map[string]struct{}{"storage.example.test": {}}
	for _, value := range []string{
		"http://storage.example.test/file",
		"https://other.example.test/file",
		"https://storage.example.test@other.example.test/file",
		"data:application/pdf;base64,AA==",
	} {
		if _, err := validateChatwootAttachmentURL(value, allowed); err == nil {
			t.Fatalf("validateChatwootAttachmentURL(%q) unexpectedly succeeded", value)
		}
	}
	parsed, err := validateChatwootAttachmentURL("https://storage.example.test/media/file.pdf", allowed)
	if err != nil || parsed.Hostname() != "storage.example.test" {
		t.Fatalf("allowlisted URL = %v, %v", parsed, err)
	}
}

func TestChatwootWhatsAppMediaPayloadRoundTripsWithoutFileContent(t *testing.T) {
	message := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		Mimetype: proto.String("application/pdf"),
		FileName: proto.String("fixture.pdf"),
		URL:      proto.String("https://mmg.example.test/file"),
	}}
	media, err := chatwootWhatsAppMediaFromMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if media == nil || media.Kind != "document" || media.FileName != "fixture.pdf" || media.MimeType != "application/pdf" || len(media.Payload) == 0 {
		t.Fatalf("media = %+v", media)
	}
	decoded := &waE2E.Message{}
	if err := proto.Unmarshal(media.Payload, decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetDocumentMessage().GetURL() != "https://mmg.example.test/file" {
		t.Fatalf("decoded document = %+v", decoded.GetDocumentMessage())
	}
}

func TestChatwootAttachmentHostsIncludeChatwootAndConfiguredStorage(t *testing.T) {
	config := chatwootConfigRecord{URL: "https://chat.example.test", AttachmentHosts: `["storage.example.test","cdn.example.test"]`}
	hosts := chatwootAttachmentHostSet(config)
	for _, host := range []string{"chat.example.test", "storage.example.test", "cdn.example.test"} {
		if _, ok := hosts[host]; !ok {
			t.Fatalf("missing allowed host %q: %v", host, hosts)
		}
	}
}

func TestChatwootAttachmentLimitMatchesMediaKind(t *testing.T) {
	if got := chatwootAttachmentLimit("image"); got != fetchImageMaxBytes {
		t.Fatalf("image limit = %d", got)
	}
	if got := chatwootAttachmentLimit("audio"); got != fetchAudioMaxBytes {
		t.Fatalf("audio limit = %d", got)
	}
	if got := chatwootAttachmentLimit("video"); got != fetchVideoMaxBytes {
		t.Fatalf("video limit = %d", got)
	}
	if got := chatwootAttachmentLimit("file"); got != fetchDocumentMaxBytes {
		t.Fatalf("document limit = %d", got)
	}
}

func TestChatwootUploadsMediaAsStreamingMultipart(t *testing.T) {
	chatwoot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "multipart/form-data;") {
			t.Fatalf("Content-Type = %q", contentType)
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("content") != "caption" || r.FormValue("message_type") != "incoming" || r.FormValue("private") != "false" || r.FormValue("content_attributes[wuzapi_source]") != "true" {
			t.Fatalf("unexpected multipart fields: %#v", r.MultipartForm.Value)
		}
		files := r.MultipartForm.File["attachments[]"]
		if len(files) != 1 || files[0].Filename != "fixture.txt" {
			t.Fatalf("files = %+v", files)
		}
		file, err := files[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil || string(body) != "fixture media" {
			t.Fatalf("media = %q, %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":301}`))
	}))
	defer chatwoot.Close()
	allowChatwootTestHost(t, chatwoot.URL)
	ctx := context.Background()
	store, _ := getMediaStore()
	media, file, err := store.create("fixture.txt", "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("fixture media"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := media.refresh(); err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	client := newChatwootClient(chatwoot.URL, 1, "fixture-token")
	id, err := client.createConversationMediaMessage(ctx, 201, "caption", "incoming", "WAID:fixture", media)
	if err != nil || id != 301 {
		t.Fatalf("createConversationMediaMessage() = %d, %v", id, err)
	}
}
