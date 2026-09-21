package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
)

func testMedia(t testing.TB, data []byte) *mediaFile {
	t.Helper()
	s, err := getMediaStore()
	if err != nil {
		t.Fatal(err)
	}
	f, out, err := s.create("fixture.png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = out.Write(data); err != nil {
		t.Fatal(err)
	}
	out.Close()
	if err = f.refresh(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}
func mediaBytes(t testing.TB, f *mediaFile) []byte {
	t.Helper()
	b, err := os.ReadFile(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestMediaDeliveryFixtures(t *testing.T) {
	data := []byte{0, 1, 2, 3, 254, 255}
	f := testMedia(t, data)
	original := map[string]interface{}{"base64": base64.StdEncoding.EncodeToString(data), "type": "Message", "fileName": "message.jpeg", "mimeType": "image/png", "event": map[string]interface{}{"text": "quote\" <&> 日本"}}
	reference, _ := json.Marshal(original)
	original["base64"] = f
	event, err := prepareMediaJSON(original)
	if err != nil {
		t.Fatal(err)
	}
	defer event.Close()
	if !bytes.Equal(mediaBytes(t, event), reference) {
		t.Fatal("JSON bytes changed")
	}
	form, err := prepareMediaForm(event, "A & 日本", "u+1")
	if err != nil {
		t.Fatal(err)
	}
	defer form.Close()
	want := url.Values{"jsonData": {string(reference)}, "instanceName": {"A & 日本"}, "userID": {"u+1"}}.Encode()
	if string(mediaBytes(t, form)) != want {
		t.Fatal("form bytes or field placement changed")
	}
	// Error queue's form payload must be strings, without injected metadata in jsonData.
	failure, err := prepareMediaJSON(map[string]interface{}{"payload": map[string]interface{}{"jsonData": fileJSON{event, true}, "userID": "u+1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer failure.Close()
	var decoded map[string]map[string]string
	if err = json.Unmarshal(mediaBytes(t, failure), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["payload"]["jsonData"] != string(reference) {
		t.Fatal("error queue jsonData changed")
	}
	for _, format := range []string{"json", "form"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("WEBHOOK_FORMAT", format)
			oldKey := *globalEncryptionKey
			*globalEncryptionKey = strings.Repeat("a", 32)
			defer func() { *globalEncryptionKey = oldKey }()
			key, err := encryptHMACKey("fixture-secret")
			if err != nil {
				t.Fatal(err)
			}
			body := event
			if format == "form" {
				body = form
			}
			sig, err := mediaSignature(body, key)
			if err != nil {
				t.Fatal(err)
			}
			mac := hmac.New(sha256.New, []byte("fixture-secret"))
			mac.Write(mediaBytes(t, body))
			if sig != hex.EncodeToString(mac.Sum(nil)) {
				t.Fatal("HMAC differs")
			}
		})
	}
}
func TestMediaRedirectRetryProxy(t *testing.T) {
	body := testMedia(t, bytes.Repeat([]byte("signed payload"), 1000))
	for _, status := range []int{307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, _ := io.ReadAll(r.Body)
				if !bytes.Equal(got, mediaBytes(t, body)) || r.ContentLength != body.Size || r.Header.Get("x-hmac-signature") != "signed" {
					t.Error("redirect changed body, length or signature")
				}
				requests.Add(1)
			}))
			defer destination.Close()
			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				http.Redirect(w, r, destination.URL, status)
			}))
			defer redirect.Close()
			client := resty.New().SetRedirectPolicy(resty.FlexibleRedirectPolicy(15)).SetTimeout(time.Second)
			for i := 0; i < 2; i++ {
				if _, err := postMediaBody(context.Background(), client.GetClient(), redirect.URL, "application/json", "signed", body); err != nil {
					t.Fatal(err)
				}
			}
			if requests.Load() != 2 {
				t.Fatal("request not replayed")
			}
		})
	}
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyCalls.Add(1); io.Copy(io.Discard, r.Body) }))
	defer proxy.Close()
	client := resty.New().SetProxy(proxy.URL)
	if _, err := postMediaBody(context.Background(), client.GetClient(), "http://unresolvable.invalid/test", "application/json", "", body); err != nil {
		t.Fatal(err)
	}
	if proxyCalls.Load() != 1 {
		t.Fatal("configured proxy bypassed")
	}
}
func TestMediaApplicationRetriesAndInvalidHMAC(t *testing.T) {
	t.Setenv("WEBHOOK_FORMAT", "json")
	oldEnabled, oldCount, oldDelay := *webhookRetryEnabled, *webhookRetryCount, *webhookRetryDelaySeconds
	*webhookRetryEnabled = true
	*webhookRetryCount = 3
	*webhookRetryDelaySeconds = 0
	defer func() {
		*webhookRetryEnabled = oldEnabled
		*webhookRetryCount = oldCount
		*webhookRetryDelaySeconds = oldDelay
	}()
	event := testMedia(t, []byte(`{"base64":"YQ=="}`))
	var calls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !bytes.Equal(b, mediaBytes(t, event)) {
			t.Error("retry body changed")
		}
		if calls.Add(1) < 3 {
			w.WriteHeader(503)
		}
	}))
	defer endpoint.Close()
	clientManager.SetHTTPClient("media-test", resty.New())
	defer clientManager.DeleteHTTPClient("media-test")
	deliverMediaHook(context.Background(), endpoint.URL, "media-test", nil, event, event, "fixture")
	if calls.Load() != 3 {
		t.Fatal("application retries changed")
	}
	deliverMediaHook(context.Background(), endpoint.URL, "media-test", []byte("invalid key"), event, event, "fixture")
	if calls.Load() != 3 {
		t.Fatal("sent unsigned request after HMAC failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := postMediaBody(ctx, http.DefaultClient, endpoint.URL, "application/json", "", event); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
func TestMediaOwnershipAndCancellation(t *testing.T) {
	base := t.TempDir()
	first, err := newMediaStore(base, 1)
	if err != nil {
		t.Fatal(err)
	}
	f, out, err := first.create("../../external.png", "")
	if err != nil {
		t.Fatal(err)
	}
	out.Close()
	second, err := newMediaStore(base, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer second.shutdown(context.Background())
	if _, err = os.Stat(f.Path); err != nil {
		t.Fatal("another instance reclaimed live file")
	}
	if filepath.Dir(f.Path) != first.dir {
		t.Fatal("external filename controls temporary path")
	}
	st, _ := os.Stat(f.Path)
	if st.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", st.Mode())
	}
	release, err := first.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = first.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("permit wait ignored cancellation")
	}
	release()
	f.Close()
	f.Close()
	abandoned, file, err := first.create("", "")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	// Simulate process death releasing its ownership lock without cleanup.
	first.lock.Close()
	third, err := newMediaStore(base, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer third.shutdown(context.Background())
	if _, err = os.Stat(abandoned.Path); !os.IsNotExist(err) {
		t.Fatal("abandoned directory was not reclaimed")
	}
	abandoned.Close()
	if err = first.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = first.create("", ""); !errors.Is(err, context.Canceled) {
		t.Fatal("accepted files after shutdown")
	}
}
func TestMediaDataURLCompatibility(t *testing.T) {
	values := []string{
		"data:audio/ogg;base64,T2dnUw==", "data:audio/mpeg;base64,SUQz", "data:image/png;base64,aGVsbG8=", "data:;base64,", "data:,a%20b+c", "data:text/plain;title=\"a,b\",hello", "data:application/octet-stream;base64,AAEC/v8=", "data:;base64,YQ==\n", "data:;base64,YQ==Yg==", "data:;base64,YQ==\r\n", "data:;base64,????", "data:;base64,YQ", "data:,bad%2", "data:,raw space", "data:,日本", "data:;charset=utf-8,%FF", "data", "data:foo,bar", "data:;base64,YQ==\nYg==",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			legacy, legacyErr := dataurl.DecodeString(value)
			reader, mime, err := dataURLReader(value)
			var got []byte
			if err == nil {
				got, err = io.ReadAll(reader)
			}
			if (err == nil) != (legacyErr == nil) {
				t.Fatalf("legacy=%v streaming=%v", legacyErr, err)
			}
			if err == nil && (!bytes.Equal(got, legacy.Data) || mime != legacy.ContentType()) {
				t.Fatalf("data or MIME changed")
			}
		})
	}
}
func TestMediaOutgoingVariantsAndLimits(t *testing.T) {
	old := globalHTTPClient
	globalHTTPClient = http.DefaultClient
	defer func() { globalHTTPClient = old }()
	img := image.NewRGBA(image.Rect(0, 0, 144, 100))
	img.Set(0, 0, color.White)
	var pngData, jpegData bytes.Buffer
	png.Encode(&pngData, img)
	jpeg.Encode(&jpegData, img, nil)
	variants := []struct {
		name, mime string
		data       []byte
		limit      int64
	}{
		{"document", "application/pdf", []byte("%PDF-1.7\n"), fetchDocumentMaxBytes},
		{"mp3", "audio/mpeg", []byte("ID3\x04\x00\x00"), fetchAudioMaxBytes},
		{"ogg", "audio/ogg", []byte("OggS\x00\x00"), fetchAudioMaxBytes},
		{"png", "image/png", pngData.Bytes(), fetchImageMaxBytes},
		{"jpeg", "image/jpeg", jpegData.Bytes(), fetchImageMaxBytes},
		{"video", "video/mp4", []byte("\x00\x00\x00\x18ftypmp42"), fetchVideoMaxBytes},
		{"sticker", "image/webp", []byte("RIFF\x04\x00\x00\x00WEBP"), fetchImageMaxBytes},
		{"button", "image/png", pngData.Bytes(), openGraphImageMaxBytes},
	}
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") != "WhatsApp/2.23.20.0" {
					t.Error("fetch headers changed")
				}
				w.Header().Set("Content-Type", v.mime)
				w.Write(v.data)
			}))
			defer endpoint.Close()
			for _, source := range []string{dataurl.New(v.data, v.mime).String(), endpoint.URL} {
				media, err := readOutgoingMedia(context.Background(), source, v.limit)
				if err != nil {
					t.Fatal(err)
				}
				if media.MIME != v.mime || media.Size != int64(len(v.data)) || !bytes.Equal(mediaBytes(t, media), v.data) {
					t.Fatal("outgoing content changed")
				}
				if v.name == "jpeg" || v.name == "png" {
					thumb, err := mediaThumbnail(context.Background(), media)
					if err != nil {
						t.Fatal(err)
					}
					cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
					if err != nil || cfg.Width != 72 || cfg.Height != 50 {
						t.Fatalf("thumbnail: %+v %v", cfg, err)
					}
				}
				path := media.Path
				media.Close()
				if _, err = os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("file leaked")
				}
			}
			if _, err := readOutgoingMedia(context.Background(), endpoint.URL, int64(len(v.data)-1)); err == nil {
				t.Fatal("URL size limit not enforced")
			}
			// Base64 input has never used the URL size restriction.
			media, err := readOutgoingMedia(context.Background(), dataurl.New(v.data, v.mime).String(), 1)
			if err != nil {
				t.Fatal(err)
			}
			media.Close()
		})
	}
}
func TestMediaStdioConcurrentLines(t *testing.T) {
	f := testMedia(t, bytes.Repeat([]byte{255}, 1024*32))
	notification, err := prepareMediaJSON(map[string]interface{}{"jsonrpc": "2.0", "method": "Message", "params": map[string]interface{}{"base64": f}})
	if err != nil {
		t.Fatal(err)
	}
	defer notification.Close()
	var out bytes.Buffer
	ss := &stdioServer{stdout: &out}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := writeMediaNotification(&out, notification); err != nil {
				t.Error(err)
			}
		}()
		go func() { defer wg.Done(); ss.writeResponse(jsonRpcResponse{JSONRPC: "2.0"}) }()
	}
	wg.Wait()
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 40 {
		t.Fatalf("got %d lines", len(lines))
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatal("interleaved output")
		}
	}
}
func TestMediaWriteFailuresAndIndependentReaders(t *testing.T) {
	s, err := newMediaStore(t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.shutdown(context.Background())
	os.RemoveAll(s.dir)
	if _, _, err = s.create("", ""); err == nil {
		t.Fatal("ignored missing directory")
	}
	f := testMedia(t, bytes.Repeat([]byte("abcd"), 65536))
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.Open()
			if err != nil {
				t.Error(err)
				return
			}
			defer r.Close()
			n, err := io.Copy(io.Discard, r)
			if err != nil || n != f.Size {
				t.Errorf("independent reader: %d %v", n, err)
			}
		}()
	}
	wg.Wait()
	injected := errors.New("disk full")
	before, _ := os.ReadDir(f.store.dir)
	if _, err := prepareMediaBody(func(w io.Writer) error { w.Write([]byte("partial")); return injected }); !errors.Is(err, injected) {
		t.Fatal("write error lost")
	}
	after, _ := os.ReadDir(f.store.dir)
	if len(before) != len(after) {
		t.Fatal("failed preparation leaked file")
	}
}
func TestMediaS3DeliveryModes(t *testing.T) {
	data := []byte("%PDF-1.7\nfixture")
	var calls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if !bytes.Equal(got, data) || r.ContentLength != int64(len(data)) {
			t.Errorf("S3 content or size changed: %q %d", got, r.ContentLength)
		}
		for header, value := range map[string]string{"Content-Type": "application/pdf", "Cache-Control": "public, max-age=3600", "Content-Disposition": "inline", "X-Amz-Acl": "public-read"} {
			if r.Header.Get(header) != value {
				t.Errorf("%s = %s", header, r.Header.Get(header))
			}
		}
		if r.Header.Get("Expires") == "" {
			t.Error("lost retention")
		}
		calls.Add(1)
		w.Header().Set("ETag", `"fixture"`)
	}))
	defer endpoint.Close()
	manager := GetS3Manager()
	config := &S3Config{Enabled: true, Endpoint: endpoint.URL, Region: "us-east-1", Bucket: "fixtures", AccessKey: "key", SecretKey: "secret", PathStyle: true, RetentionDays: 7}
	if err := manager.InitializeS3Client("media-s3", config); err != nil {
		t.Fatal(err)
	}
	defer manager.RemoveClient("media-s3")
	client := &MyClient{userID: "media-s3"}
	for _, mode := range []string{"base64", "s3", "both"} {
		t.Run(mode, func(t *testing.T) {
			media := testMedia(t, data)
			media.Name = "message.pdf"
			event := map[string]interface{}{}
			retained := client.attachDownloadedMedia(context.Background(), media, "application/pdf", "chat", "message", true, mediaS3Config{"true", mode}, event)
			_, hasBase64 := event["base64"]
			_, hasS3 := event["s3"]
			if hasBase64 != (mode != "s3") || hasS3 != (mode != "base64") || retained != hasBase64 {
				t.Fatalf("mode fields: %#v", event)
			}
			if hasS3 {
				metadata := event["s3"].(map[string]interface{})
				if metadata["size"] != int64(len(data)) || metadata["fileName"] != "message.pdf" || metadata["bucket"] != "fixtures" || metadata["mimeType"] != "application/pdf" {
					t.Fatalf("metadata: %#v", metadata)
				}
			}
		})
	}
	if calls.Load() != 2 {
		t.Fatalf("S3 calls: %d", calls.Load())
	}
}
func TestMediaStickerFileCompatibility(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	encoders, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil || !bytes.Contains(encoders, []byte("libwebp")) {
		t.Skip("ffmpeg lacks libwebp encoder")
	}
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	img.Set(0, 0, color.White)
	var input bytes.Buffer
	png.Encode(&input, img)
	legacy, mime, err := processStickerData(dataurl.New(input.Bytes(), "image/png").String(), "", "pack", "name", "publisher", []string{"😀"})
	if err != nil {
		t.Fatal(err)
	}
	source := testMedia(t, input.Bytes())
	file, err := processStickerFile(context.Background(), source, "", "pack", "name", "publisher", []string{"😀"})
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.MIME != mime || !bytes.Equal(mediaBytes(t, file), legacy) {
		t.Fatal("sticker conversion or EXIF differs from legacy bytes")
	}
	// Existing WebP with and without metadata follows the same chunk rewriting.
	source2 := testMedia(t, legacy)
	again, err := processStickerFile(context.Background(), source2, "", "other", "new", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	want := embedStickerEXIF(legacy, "other", "new", "", nil)
	if !bytes.Equal(mediaBytes(t, again), want) {
		t.Fatal("existing EXIF replacement changed")
	}
}
func TestMediaExistingWebPEXIF(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	source := testMedia(t, data)
	exif := buildWhatsAppEXIF(buildStickerMetadata("pack", "name", "publisher", []string{"😀"}))
	expected, err := injectWebPEXIF(data, exif)
	if err != nil {
		t.Fatal(err)
	}
	file, err := injectMediaEXIF(context.Background(), source, exif)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if !bytes.Equal(mediaBytes(t, file), expected) {
		t.Fatal("EXIF or WebP chunk layout changed")
	}
	updated, err := injectMediaEXIF(context.Background(), file, exif)
	if err != nil {
		t.Fatal(err)
	}
	defer updated.Close()
	if !bytes.Equal(mediaBytes(t, updated), expected) {
		t.Fatal("EXIF replacement changed")
	}
}
func TestMediaUserAndGlobalWebhooks(t *testing.T) {
	for _, format := range []string{"json", "form"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("WEBHOOK_FORMAT", format)
			keyBefore, hookBefore, globalKeyBefore := *globalEncryptionKey, *globalWebhook, globalHMACKeyEncrypted
			*globalEncryptionKey = strings.Repeat("b", 32)
			defer func() {
				*globalEncryptionKey = keyBefore
				*globalWebhook = hookBefore
				globalHMACKeyEncrypted = globalKeyBefore
			}()
			userKey, err := encryptHMACKey("user-secret")
			if err != nil {
				t.Fatal(err)
			}
			globalHMACKeyEncrypted, err = encryptHMACKey("global-secret")
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 2)
			receiver := func(secret string) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						done <- err
						return
					}
					mac := hmac.New(sha256.New, []byte(secret))
					mac.Write(body)
					if r.Header.Get("x-hmac-signature") != hex.EncodeToString(mac.Sum(nil)) {
						done <- fmt.Errorf("signature mismatch")
						return
					}
					var event map[string]interface{}
					if format == "form" {
						values, err := url.ParseQuery(string(body))
						if err != nil {
							done <- err
							return
						}
						if len(values) != 3 || values.Get("userID") != "media-user" || values.Get("instanceName") != "instance" {
							done <- fmt.Errorf("form envelope changed")
							return
						}
						err = json.Unmarshal([]byte(values.Get("jsonData")), &event)
					} else {
						err = json.Unmarshal(body, &event)
					}
					if err == nil && event["base64"] != "AAEC/w==" {
						err = fmt.Errorf("attachment changed")
					}
					if format == "form" && event["userID"] != nil {
						err = fmt.Errorf("injected metadata in form")
					}
					if format == "json" && (event["userID"] != "media-user" || event["instanceName"] != "instance") {
						err = fmt.Errorf("JSON envelope changed")
					}
					done <- err
				}))
			}
			user := receiver("user-secret")
			defer user.Close()
			global := receiver("global-secret")
			defer global.Close()
			*globalWebhook = global.URL
			userinfocache.Set("media-token", Values{m: map[string]string{"Name": "instance", "Events": "All", "Webhook": user.URL, "HmacKeyEncrypted": base64.StdEncoding.EncodeToString(userKey)}}, 0)
			defer userinfocache.Delete("media-token")
			clientManager.SetHTTPClient("media-user", resty.New())
			defer clientManager.DeleteHTTPClient("media-user")
			f := testMedia(t, []byte{0, 1, 2, 255})
			event := map[string]interface{}{"base64": f, "type": "Message"}
			sendEventWithWebHook(&MyClient{userID: "media-user", token: "media-token"}, event, "")
			for i := 0; i < 2; i++ {
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("webhook not delivered")
				}
			}
			// Wait for transport completion before restoring globals used by delivery.
			clientManager.GetHTTPClient("media-user").GetClient().CloseIdleConnections()
		})
	}
}

type fakeMediaUploader struct {
	t      *testing.T
	source *mediaFile
}

func (f fakeMediaUploader) UploadReader(ctx context.Context, r io.Reader, scratch io.ReadWriteSeeker, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	plain, err := io.ReadAll(r)
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	if _, err = scratch.Write([]byte("encrypted scratch")); err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	if !bytes.Equal(plain, mediaBytes(f.t, f.source)) {
		f.t.Error("encryption scratch overwrote plaintext")
	}
	return whatsmeow.UploadResponse{FileLength: uint64(len(plain))}, nil
}
func TestMediaUploadScratchLifecycle(t *testing.T) {
	source := testMedia(t, []byte("plaintext needed for thumbnail"))
	before, _ := os.ReadDir(source.store.dir)
	response, err := uploadMedia(context.Background(), fakeMediaUploader{t, source}, source, whatsmeow.MediaImage)
	if err != nil || response.FileLength != uint64(source.Size) {
		t.Fatalf("upload: %+v %v", response, err)
	}
	after, _ := os.ReadDir(source.store.dir)
	if len(before) != len(after) {
		t.Fatal("scratch file leaked")
	}
}
func TestMediaEndpointInvalidInputs(t *testing.T) {
	client := whatsmeow.NewClient(&store.Device{}, nil)
	clientManager.SetWhatsmeowClient("media-endpoint", client)
	defer clientManager.DeleteWhatsmeowClient("media-endpoint")
	s := &server{}
	for _, endpoint := range []struct {
		name, field string
		handler     http.HandlerFunc
	}{
		{"document", "Document", s.SendDocument()}, {"audio", "Audio", s.SendAudio()}, {"image", "Image", s.SendImage()}, {"video", "Video", s.SendVideo()}, {"sticker", "Sticker", s.SendSticker()},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			for _, input := range []string{"invalid", "data:" + endpoint.name + "/bad;base64,?"} {
				body, _ := json.Marshal(map[string]interface{}{"Phone": "123@g.us", "Id": "fixture", "FileName": "name.pdf", endpoint.field: input})
				request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
				request = request.WithContext(context.WithValue(request.Context(), "userinfo", Values{m: map[string]string{"Id": "media-endpoint"}}))
				response := httptest.NewRecorder()
				endpoint.handler(response, request)
				if response.Code != 400 {
					t.Fatalf("input %q: status %d: %s", input, response.Code, response.Body.String())
				}
			}
		})
	}
	// Optional button image decoding remains best effort: invalid image data must
	// reach the send operation rather than reject an otherwise valid request.
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"Phone":"123@g.us","Id":"fixture","Body":"hello","Image":"data:image/png;base64,?","Buttons":[{"Title":"OK"}]}`))
	request = request.WithContext(context.WithValue(request.Context(), "userinfo", Values{m: map[string]string{"Id": "media-endpoint"}}))
	response := httptest.NewRecorder()
	s.SendButtons()(response, request)
	if response.Code == 400 {
		t.Fatalf("optional button image became mandatory: %s", response.Body.String())
	}
}
func TestMediaURLSSRFAndCancellation(t *testing.T) {
	previous := globalHTTPClient
	globalHTTPClient = newSafeHTTPClient()
	defer func() { globalHTTPClient = previous }()
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("SSRF protection bypassed") }))
	defer endpoint.Close()
	if _, err := readOutgoingMedia(context.Background(), endpoint.URL, 1024); err == nil {
		t.Fatal("accepted loopback URL")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readOutgoingMedia(ctx, "data:;base64,YQ==", 1024); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
func TestMediaShutdownWaitsForOwners(t *testing.T) {
	store, err := newMediaStore(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	file, out, err := store.create("", "")
	if err != nil {
		t.Fatal(err)
	}
	out.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err = store.shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("removed live owner's files: %v", err)
	}
	if _, err = os.Stat(file.Path); err != nil {
		t.Fatal("removed file still owned by delivery")
	}
	file.Close()
	if err = store.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(store.dir); !os.IsNotExist(err) {
		t.Fatal("shutdown left process directory")
	}
}
func TestMediaRabbitDisabledDoesNotReadFile(t *testing.T) {
	restoreRabbitState(t)
	setRabbitConnection(nil, nil, false)
	if err := publishMediaToRabbit(&mediaFile{Path: "/does-not-exist"}); err != nil {
		t.Fatal("disabled publisher opened attachment")
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = publishMediaToRabbit(&mediaFile{Path: "/does-not-exist"}) }()
		go func() { defer wg.Done(); setRabbitConnection(nil, nil, false) }()
	}
	wg.Wait()
}

func TestMediaIncomingNames(t *testing.T) {
	for _, variant := range []struct{ mime, fallback string }{{"image/png", ".jpg"}, {"image/jpeg", ".jpg"}, {"audio/mpeg", ".ogg"}, {"audio/ogg", ".ogg"}, {"application/x-wuzapi-unknown", ".custom"}} {
		// Fixture uses the pre-patch naming rule, including MIME's precedence over
		// the original document extension and platform-specific extension ordering.
		extension := variant.fallback
		if choices, _ := mime.ExtensionsByType(variant.mime); len(choices) > 0 {
			extension = choices[0]
		}
		if got := incomingMediaName("message", variant.mime, variant.fallback); got != "message"+extension {
			t.Fatalf("%s filename: %s", variant.mime, got)
		}
	}
}
func TestMediaStickerConversionVariants(t *testing.T) {
	encoders, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil || !bytes.Contains(encoders, []byte("libwebp")) {
		t.Skip("FFmpeg with libwebp required")
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	img.Set(0, 0, color.White)
	var jpegData, gifData bytes.Buffer
	if err = jpeg.Encode(&jpegData, img, nil); err != nil {
		t.Fatal(err)
	}
	frame := image.NewPaletted(image.Rect(0, 0, 32, 24), color.Palette{color.Black, color.White})
	frame.SetColorIndex(0, 0, 1)
	if err = gif.EncodeAll(&gifData, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []struct {
		mime string
		data []byte
	}{{"image/jpeg", jpegData.Bytes()}, {"image/gif", gifData.Bytes()}} {
		t.Run(variant.mime, func(t *testing.T) {
			expected, mimeType, err := processStickerData(dataurl.New(variant.data, variant.mime).String(), "", "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			input := testMedia(t, variant.data)
			converted, err := processStickerFile(context.Background(), input, "", "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer converted.Close()
			if converted.MIME != mimeType || !bytes.Equal(mediaBytes(t, converted), expected) {
				t.Fatal("conversion bytes differ")
			}
		})
	}
}

func TestMediaFailedDownloadCleansFile(t *testing.T) {
	manager, err := getMediaStore()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(manager.dir)
	client := &MyClient{WAClient: whatsmeow.NewClient(&store.Device{}, nil)}
	event := map[string]interface{}{}
	client.processMedia(&waE2E.ImageMessage{}, "image/png", ".jpg", time.Second, true, "chat", "id", mediaS3Config{MediaDelivery: "base64"}, event, nil)
	if len(event) != 0 {
		t.Fatal("failed download produced media fields")
	}
	after, _ := os.ReadDir(manager.dir)
	if len(before) != len(after) {
		t.Fatal("failed download leaked file")
	}
}
