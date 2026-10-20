package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

func carouselImage(t *testing.T, width, height int, jpegFormat bool) string {
	t.Helper()
	var out bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	picture.Set(0, 0, color.RGBA{R: 200, A: 255})
	format := "png"
	var err error
	if jpegFormat {
		format, err = "jpeg", jpeg.Encode(&out, picture, nil)
	} else {
		err = png.Encode(&out, picture)
	}
	if err != nil {
		t.Fatal(err)
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

func carouselFixture(t *testing.T) carouselRequest {
	t.Helper()
	return carouselRequest{Phone: "15555550100", Id: "carousel-test-id", Body: "Choose a destination", Cards: []carouselCard{
		{Body: "Summit", Image: carouselImage(t, 3, 2, false), Buttons: []carouselButton{{Title: "Choose", ID: "summit"}}},
		{Body: "Coast", Image: carouselImage(t, 4, 2, true), Buttons: []carouselButton{{Type: "reply", Title: "Choose", ID: "coast"}}},
	}}
}

func TestCarouselValidation(t *testing.T) {
	cases := map[string]func(*carouselRequest){
		"empty body":         func(r *carouselRequest) { r.Body = " " },
		"long body":          func(r *carouselRequest) { r.Body = strings.Repeat("é", 1025) },
		"long id":            func(r *carouselRequest) { r.Id = strings.Repeat("a", 129) },
		"id whitespace":      func(r *carouselRequest) { r.Id = "a b" },
		"id control":         func(r *carouselRequest) { r.Id = "a\x00" },
		"id nonascii":        func(r *carouselRequest) { r.Id = "日本" },
		"one card":           func(r *carouselRequest) { r.Cards = r.Cards[:1] },
		"too many cards":     func(r *carouselRequest) { r.Cards = make([]carouselCard, 11) },
		"empty card body":    func(r *carouselRequest) { r.Cards[1].Body = "" },
		"long card body":     func(r *carouselRequest) { r.Cards[1].Body = strings.Repeat("é", 161) },
		"missing image":      func(r *carouselRequest) { r.Cards[1].Image = "" },
		"missing buttons":    func(r *carouselRequest) { r.Cards[1].Buttons = nil },
		"too many buttons":   func(r *carouselRequest) { r.Cards[1].Buttons = make([]carouselButton, 4) },
		"empty title":        func(r *carouselRequest) { r.Cards[1].Buttons[0].Title = " " },
		"long title":         func(r *carouselRequest) { r.Cards[1].Buttons[0].Title = strings.Repeat("é", 21) },
		"padded long title":  func(r *carouselRequest) { r.Cards[1].Buttons[0].Title = strings.Repeat("é", 20) + " " },
		"control title":      func(r *carouselRequest) { r.Cards[1].Buttons[0].Title = "\nChoose" },
		"missing reply id":   func(r *carouselRequest) { r.Cards[1].Buttons[0].ID = "" },
		"duplicate reply id": func(r *carouselRequest) { r.Cards[1].Buttons[0].ID = "summit" },
		"long reply id":      func(r *carouselRequest) { r.Cards[1].Buttons[0].ID = strings.Repeat("a", 201) },
		"control reply id":   func(r *carouselRequest) { r.Cards[1].Buttons[0].ID = "a\n" },
		"unsupported action": func(r *carouselRequest) { r.Cards[1].Buttons[0].Type = "copy" },
		"uppercase action":   func(r *carouselRequest) { r.Cards[1].Buttons[0].Type = "REPLY" },
		"action whitespace":  func(r *carouselRequest) { r.Cards[1].Buttons[0].Type = " reply " },
		"mixed action":       func(r *carouselRequest) { r.Cards[1].Buttons[0].URL = "https://example.com" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := carouselFixture(t)
			mutate(&r)
			if _, err := validateCarousel(r); err == nil {
				t.Fatal("invalid carousel admitted")
			}
		})
	}
	r := carouselFixture(t)
	r.Id, r.Body, r.Cards[1].Body = "", strings.Repeat("é", 1024), strings.Repeat("界", 160)
	r.Cards[1].Buttons[0].Title = strings.Repeat("é", 20)
	if _, err := validateCarousel(r); err != nil {
		t.Fatalf("valid Unicode boundary or omitted Id rejected: %v", err)
	}
	for _, link := range []string{"", "/relative", "javascript:alert(1)", "https://", "https://user:secret@example.com", "https://example.com/a b", "https://example.com/\n", "https://example.com/" + strings.Repeat("a", 2048)} {
		r := carouselFixture(t)
		r.Cards[0].Buttons = []carouselButton{{Type: "cta_url", Title: "Open", URL: link}}
		if _, err := validateCarousel(r); err == nil {
			t.Fatalf("invalid link admitted: %q", link)
		}
	}
	for _, phone := range []string{"", "+0", "call me", "+1555 5550100", "1555555010012345"} {
		r := carouselFixture(t)
		r.Cards[0].Buttons = []carouselButton{{Type: "cta_call", Title: "Call", PhoneNumber: phone}}
		if _, err := validateCarousel(r); err == nil {
			t.Fatalf("invalid call number admitted: %q", phone)
		}
	}
}

func TestCarouselRecipients(t *testing.T) {
	mappedLID := types.NewJID("987654321", types.HiddenUserServer)
	mapping := &fakeMessageLIDStore{lidForPN: map[string]types.JID{"15555550100": mappedLID}}
	jid, err := resolveMessageRecipient(context.Background(), mapping, "15555550100")
	if err != nil || jid != mappedLID || validateCarouselRecipient(jid) != nil {
		t.Fatalf("mapped PN/LID rejected: %v %v", jid, err)
	}
	mapping.pnForLID = map[string]types.JID{"15555550100": types.NewJID("15555550101", types.DefaultUserServer)}
	if _, err := resolveMessageRecipient(context.Background(), mapping, "15555550100"); err == nil {
		t.Fatal("ambiguous PN/LID admitted")
	}
	for _, value := range []string{"15555550100", "+15555550100", "15555550100@s.whatsapp.net", "123456789@lid"} {
		jid, err := resolveMessageRecipient(context.Background(), nil, value)
		if err != nil || validateCarouselRecipient(jid) != nil {
			t.Fatalf("valid recipient rejected: %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "text", "123@g.us", "status@broadcast", "123@newsletter", "123@bot", "13135550000@s.whatsapp.net", "123@hosted", "123@hosted.lid", "123@c.us", "123@msgr", "123@interop", "123:2@s.whatsapp.net", "123_1@s.whatsapp.net", "name@lid"} {
		jid, err := resolveMessageRecipient(context.Background(), nil, value)
		if err == nil && validateCarouselRecipient(jid) == nil {
			t.Fatalf("unsupported recipient admitted: %q %+v", value, jid)
		}
	}
}

type carouselFakeSender struct {
	uploads, sends int
	failUpload     int
	sendError      error
	cancel         context.CancelFunc
	cancelAtUpload int
	cancelAtSend   context.CancelFunc
	message        *waE2E.Message
	extra          whatsmeow.SendRequestExtra
	recipient      types.JID
}

func (*carouselFakeSender) IsConnected() bool                    { return true }
func (*carouselFakeSender) ConnectContext(context.Context) error { return nil }
func (f *carouselFakeSender) UploadReader(ctx context.Context, reader io.Reader, scratch io.ReadWriteSeeker, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	f.uploads++
	if err := ctx.Err(); err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	if f.uploads == f.failUpload {
		return whatsmeow.UploadResponse{}, errors.New("private /tmp/internal detail")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	if _, err = scratch.Write(data); err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	if f.cancel != nil && f.uploads == f.cancelAtUpload {
		f.cancel()
	}
	digest := sha256.Sum256(data)
	return whatsmeow.UploadResponse{URL: "https://example.com/media", DirectPath: "/media", MediaKey: bytes.Repeat([]byte{1}, 32), FileEncSHA256: bytes.Repeat([]byte{2}, 32), FileSHA256: digest[:], FileLength: uint64(len(data))}, nil
}
func (f *carouselFakeSender) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	f.sends++
	f.message, f.extra, f.recipient = message, extra[0], to
	if f.cancelAtSend != nil {
		f.cancelAtSend()
	}
	if err := ctx.Err(); err != nil {
		return whatsmeow.SendResponse{}, err
	}
	return whatsmeow.SendResponse{ID: extra[0].ID, Timestamp: time.Unix(1700000000, 0)}, f.sendError
}

func TestCarouselSingleMessage(t *testing.T) {
	r := carouselFixture(t)
	r.Cards[0].Buttons[0].Title = " Choose "
	r.Cards[1].Buttons = []carouselButton{
		{Type: "cta_url", Title: "Details", URL: "https://example.com/?a=1&b=2"},
		{Type: "cta_call", Title: "Call", PhoneNumber: "+15555550100"},
	}
	cards, err := prepareCarousel(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCarouselMedia(cards)
	client := &carouselFakeSender{}
	to := types.NewJID("15555550100", types.DefaultUserServer)
	response, message, err := sendCarousel(context.Background(), "test", client, to, r.Id, r.Body, cards)
	if err != nil || client.uploads != 2 || client.sends != 1 || response.ID != r.Id || client.extra.ID != r.Id || client.recipient != to {
		t.Fatalf("incorrect dispatch: uploads=%d sends=%d response=%+v err=%v", client.uploads, client.sends, response, err)
	}
	carousel := message.GetInteractiveMessage().GetCarouselMessage()
	if carousel == nil || len(carousel.Cards) != 2 || carousel.GetCarouselCardType() != waE2E.InteractiveMessage_CarouselMessage_HSCROLL_CARDS || message.GetInteractiveMessage().GetBody().GetText() != r.Body {
		t.Fatal("native carousel or main text missing")
	}
	for i, card := range carousel.Cards {
		img := card.GetHeader().GetImageMessage()
		if card.GetBody().GetText() != r.Cards[i].Body || !card.GetHeader().GetHasMediaAttachment() || len(img.GetMediaKey()) != 32 || len(img.GetFileSHA256()) != 32 || img.GetFileLength() != uint64(cards[i].media.Size) || len(img.GetJPEGThumbnail()) == 0 || img.GetWidth() != uint32(i+3) || img.GetHeight() != 2 {
			t.Fatalf("card %d lost text or image metadata", i)
		}
	}
	if carousel.Cards[0].GetHeader().GetImageMessage().GetMimetype() != "image/png" || carousel.Cards[1].GetHeader().GetImageMessage().GetMimetype() != "image/jpeg" {
		t.Fatal("image MIME changed")
	}
	for i, expected := range []struct {
		card, button       int
		name, field, value string
	}{
		{0, 0, "quick_reply", "id", "summit"}, {1, 0, "cta_url", "url", "https://example.com/?a=1&b=2"}, {1, 1, "cta_call", "phone_number", "+15555550100"},
	} {
		button := carousel.Cards[expected.card].GetNativeFlowMessage().GetButtons()[expected.button]
		var params map[string]string
		if button.GetName() != expected.name || json.Unmarshal([]byte(button.GetButtonParamsJSON()), &params) != nil || params[expected.field] != expected.value || params["display_text"] != r.Cards[expected.card].Buttons[expected.button].Title {
			t.Fatalf("button %d lost action parameters", i)
		}
	}
	if client.extra.Timeout != carouselSendTimeout || client.extra.AdditionalNodes == nil || len(*client.extra.AdditionalNodes) != 1 {
		t.Fatal("missing timeout or biz node")
	}
	biz := (*client.extra.AdditionalNodes)[0]
	native := biz.GetChildByTag("interactive", "native_flow")
	if biz.Tag != "biz" || native.Attrs["name"] != "mixed" || native.Attrs["v"] != "9" {
		t.Fatal("incorrect native-flow envelope")
	}
}

func TestCarouselFailureAndCancellation(t *testing.T) {
	r := carouselFixture(t)
	cards, err := prepareCarousel(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCarouselMedia(cards)
	for _, failure := range []struct {
		name    string
		upload  int
		sendErr error
	}{
		{"first image", 1, nil}, {"last image", 2, nil}, {"ambiguous send", 0, errors.New("ack lost")},
	} {
		t.Run(failure.name, func(t *testing.T) {
			client := &carouselFakeSender{failUpload: failure.upload, sendError: failure.sendErr}
			_, message, err := sendCarousel(context.Background(), "test", client, types.NewJID("15555550100", types.DefaultUserServer), r.Id, r.Body, cards)
			if err == nil || (failure.upload > 0 && client.sends != 0) || client.sends > 1 || strings.Contains(err.Error(), "/tmp") {
				t.Fatalf("invalid failure result: %v, sends=%d", err, client.sends)
			}
			w := httptest.NewRecorder()
			new(server).respondCarouselFailure(w, httptest.NewRequest(http.MethodPost, "/chat/send/carousel", nil), err, message != nil)
			body := w.Body.String()
			if w.Code != http.StatusInternalServerError || strings.Contains(body, "/tmp") {
				t.Fatalf("invalid failure response: %d %s", w.Code, body)
			}
			if failure.upload > 0 {
				if message != nil || !strings.Contains(body, "Cards[") || !strings.Contains(body, "upload failed; message not sent") || strings.Contains(body, "unconfirmed") {
					t.Fatalf("pre-dispatch failure reported as ambiguous: %s", body)
				}
			} else if message == nil || !strings.Contains(body, "delivery unconfirmed") || strings.Contains(body, "message not sent") {
				t.Fatalf("dispatched failure reported as not sent: %s", body)
			}
		})
	}
	for _, afterUpload := range []int{0, 1, 2} {
		ctx, cancel := context.WithCancel(context.Background())
		client := &carouselFakeSender{cancel: cancel, cancelAtUpload: afterUpload}
		if afterUpload == 0 {
			cancel()
		}
		_, _, err := sendCarousel(ctx, "test", client, types.NewJID("15555550100", types.DefaultUserServer), r.Id, r.Body, cards)
		cancel()
		if err == nil || client.sends != 0 || client.uploads != afterUpload {
			t.Fatalf("cancelled preparation dispatched: uploads=%d sends=%d", client.uploads, client.sends)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := &carouselFakeSender{cancelAtSend: cancel}
	response, message, err := sendCarousel(ctx, "test", client, types.NewJID("15555550100", types.DefaultUserServer), r.Id, r.Body, cards)
	if err != nil || ctx.Err() == nil || response.ID != r.Id || message == nil || client.sends != 1 {
		t.Fatalf("HTTP cancellation discarded dispatched result: %v", err)
	}
}

func TestCarouselMediaAdmissionAndCleanup(t *testing.T) {
	store, err := getMediaStore()
	if err != nil {
		t.Fatal(err)
	}
	count := func() int {
		entries, err := os.ReadDir(store.dir)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	baseline := count()
	// The dimensions pass the per-side limit but exceed the aggregate pixel limit.
	pixels, _ := base64.StdEncoding.DecodeString(strings.SplitN(carouselImage(t, 3, 2, false), ",", 2)[1])
	binary.BigEndian.PutUint32(pixels[16:20], 4096)
	binary.BigEndian.PutUint32(pixels[20:24], 3907)
	binary.BigEndian.PutUint32(pixels[29:33], crc32.ChecksumIEEE(pixels[12:29]))
	for name, bad := range map[string]string{
		"empty image":  "data:image/png;base64,",
		"not an image": "data:image/png;base64,bm90IGFuIGltYWdl",
		"dimensions":   carouselImage(t, 4097, 1, false),
		"megapixels":   "data:image/png;base64," + base64.StdEncoding.EncodeToString(pixels),
		"encoding":     "data:image/png;base64,?",
		"decoded size": "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, fetchImageMaxBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			r := carouselFixture(t)
			r.Cards[1].Image = bad
			_, err := prepareCarousel(context.Background(), r)
			if err == nil {
				t.Fatal("invalid media admitted")
			}
			if name == "empty image" && !strings.Contains(err.Error(), "empty image") {
				t.Fatalf("empty image reported as size overflow: %v", err)
			}
			if name == "megapixels" && !strings.Contains(err.Error(), "16 megapixels") {
				t.Fatalf("pixel budget not checked before full decode: %v", err)
			}
			if count() != baseline {
				t.Fatal("failed admission leaked media files")
			}
		})
	}
	r := carouselFixture(t)
	data, _ := base64.StdEncoding.DecodeString(strings.SplitN(r.Cards[1].Image, ",", 2)[1])
	r.Cards[1].Image = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data[:len(data)-20])
	if _, err := prepareCarousel(context.Background(), r); err == nil {
		t.Fatal("truncated image admitted")
	}
	if count() != baseline {
		t.Fatal("truncated image leaked media files")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := prepareCarousel(ctx, carouselFixture(t)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired preparation admitted: %v", err)
	}
	if count() != baseline {
		t.Fatal("expired preparation leaked media files")
	}
}

func TestCarouselURLAndValidationOrder(t *testing.T) {
	fixture := carouselFixture(t)
	data, _ := base64.StdEncoding.DecodeString(strings.SplitN(fixture.Cards[0].Image, ",", 2)[1])
	var fetches atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fetches.Add(1); w.Write(data) }))
	defer endpoint.Close()
	previous := globalHTTPClient
	globalHTTPClient = endpoint.Client()
	defer func() { globalHTTPClient = previous }()
	fixture.Cards[0].Image, fixture.Cards[1].Image = endpoint.URL, endpoint.URL
	fixture.Cards[1].Buttons[0].ID = "summit"
	if _, err := prepareCarousel(context.Background(), fixture); err == nil || fetches.Load() != 0 {
		t.Fatal("invalid later action triggered early media fetch")
	}
	fixture.Cards[1].Buttons[0].ID = "coast"
	cards, err := prepareCarousel(context.Background(), fixture)
	if err != nil || fetches.Load() != 2 {
		t.Fatalf("URL media failed: %v, fetches=%d", err, fetches.Load())
	}
	closeCarouselMedia(cards)
	globalHTTPClient = newSafeHTTPClient()
	if _, err := prepareCarousel(context.Background(), fixture); err == nil || fetches.Load() != 2 {
		t.Fatal("carousel bypassed existing SSRF protection")
	}
}

func TestCarouselAggregateImageBudget(t *testing.T) {
	r := carouselFixture(t)
	data, _ := base64.StdEncoding.DecodeString(strings.SplitN(r.Cards[0].Image, ",", 2)[1])
	// A valid PNG may have trailing bytes. It must still count toward the limit.
	large := append(data, make([]byte, fetchImageMaxBytes-len(data)-1)...)
	r.Cards[0].Image = "data:image/png;base64," + base64.StdEncoding.EncodeToString(large)
	store, err := getMediaStore()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(store.dir)
	if _, err := prepareCarousel(context.Background(), r); err == nil || !strings.Contains(err.Error(), "16 MiB") {
		t.Fatalf("aggregate data-URL budget not enforced: %v", err)
	}
	after, _ := os.ReadDir(store.dir)
	if len(before) != len(after) {
		t.Fatal("aggregate failure leaked media files")
	}
	var transferred atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		transferred.Add(1)
		w.Write(data)
	}))
	defer endpoint.Close()
	previous := globalHTTPClient
	globalHTTPClient = endpoint.Client()
	defer func() { globalHTTPClient = previous }()
	r.Cards[1].Image = endpoint.URL
	if _, err := prepareCarousel(context.Background(), r); err == nil || !strings.Contains(err.Error(), "could not read image") || transferred.Load() != 1 {
		t.Fatalf("URL read did not enforce remaining one-byte budget: %v", err)
	}
	after, _ = os.ReadDir(store.dir)
	if len(before) != len(after) {
		t.Fatal("URL budget failure leaked media files")
	}
}

type carouselFailingLIDStore struct {
	store.LIDStore
	err error
}

func (s carouselFailingLIDStore) GetPNForLID(context.Context, types.JID) (types.JID, error) {
	return types.EmptyJID, s.err
}

func TestCarouselHTTPAndStdio(t *testing.T) {
	s := makeTestServer(t)
	const userID, token = "carousel-http", "carousel-token"
	if _, err := s.db.Exec(`INSERT INTO users (id, name, token, connected) VALUES ($1,$2,$3,$4)`, userID, "Carousel", token, 0); err != nil {
		t.Fatal(err)
	}
	client := whatsmeow.NewClient(&store.Device{}, nil)
	clientManager.SetWhatsmeowClient(userID, client)
	defer clientManager.DeleteWhatsmeowClient(userID)
	for _, test := range []struct {
		body, token string
		status      int
		contains    string
	}{
		{`{}`, "", 401, ""},
		{`{`, token, 400, "decode"},
		{`{"Phone":"15555550100","Body":"Hi","Cards":[],"Footer":"unsupported"}`, token, 400, "unknown field"},
		{`{} {}`, token, 400, "one JSON object"},
		{`{"Phone":"15555550100","Body":"Hi","Cards":[],"token":"ignored","adminToken":"ignored"}`, token, 400, "Cards"},
		{`{"Phone":"15555550100","Body":"` + strings.Repeat("x", carouselMaxRequestBytes) + `"}`, token, 413, ""},
	} {
		req := httptest.NewRequest(http.MethodPost, "/chat/send/carousel", strings.NewReader(test.body))
		req.Header.Set("token", test.token)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		if w.Code != test.status || !strings.Contains(w.Body.String(), test.contains) {
			t.Fatalf("got %d %s; want %d containing %q", w.Code, w.Body.String(), test.status, test.contains)
		}
	}
	request := newRequest("carousel-stdio", "chat.send.carousel", map[string]interface{}{"token": token, "Phone": "15555550100", "Body": "Hi", "Cards": []interface{}{}}).toJSON(t)
	response := executeRequest(t, s, request)
	errObj, ok := response["error"].(map[string]interface{})
	if !ok || errObj["code"] != float64(400) || !strings.Contains(errObj["message"].(string), "Cards") || strings.Contains(errObj["message"].(string), "unknown field") {
		t.Fatalf("stdio did not reach carousel validation: %+v", response)
	}
	payload, err := json.Marshal(carouselFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		want := http.StatusGatewayTimeout
		if canceled {
			cancel()
			ctx, cancel = context.WithCancel(context.Background())
			cancel()
			want = http.StatusServiceUnavailable
		}
		req := httptest.NewRequest(http.MethodPost, "/chat/send/carousel", bytes.NewReader(payload)).WithContext(ctx)
		req.Header.Set("token", token)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		cancel()
		if w.Code != want || !strings.Contains(w.Body.String(), "preparation canceled") {
			t.Fatalf("preparation context error: got %d %s; want %d", w.Code, w.Body.String(), want)
		}
	}
	for _, lookup := range []struct {
		err    error
		status int
	}{{context.Canceled, http.StatusServiceUnavailable}, {context.DeadlineExceeded, http.StatusGatewayTimeout}} {
		client.Store.LIDs = carouselFailingLIDStore{err: lookup.err}
		req := httptest.NewRequest(http.MethodPost, "/chat/send/carousel", bytes.NewReader(payload))
		req.Header.Set("token", token)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		if w.Code != lookup.status {
			t.Fatalf("resolver context failure: got %d %s; want %d", w.Code, w.Body.String(), lookup.status)
		}
	}
	client.Store.LIDs = nil
	clientManager.DeleteWhatsmeowClient(userID)
	req := httptest.NewRequest(http.MethodPost, "/chat/send/carousel", bytes.NewReader(payload))
	req.Header.Set("token", token)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "no session") {
		t.Fatalf("missing session response: %d %s", w.Code, w.Body.String())
	}
}
