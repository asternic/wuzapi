package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"google.golang.org/protobuf/proto"
)

func seedClientHistory(t *testing.T, s *server, user, token string, retention int) {
	t.Helper()
	if _, err := s.db.Exec("INSERT INTO users (id,name,token,history) VALUES ($1,$2,$3,$4)", user, user, token, retention); err != nil {
		t.Fatal(err)
	}
}
func clientGET(t *testing.T, s *server, token, path string, code int) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("token", token)
	}
	response := httptest.NewRecorder()
	s.router.ServeHTTP(response, req)
	if response.Code != code {
		t.Fatalf("%s: got %d, want %d: %s", path, response.Code, code, response.Body.String())
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if code == http.StatusOK && response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private history must not be cached")
	}
	data, _ := envelope["data"].(map[string]interface{})
	return data
}
func insertClientMessage(t *testing.T, s *server, user, chat, id, text string, stamp time.Time) int64 {
	t.Helper()
	var key int64
	err := s.db.QueryRow(s.db.Rebind(`INSERT INTO message_history (user_id,chat_jid,sender_jid,message_id,timestamp,message_type,text_content,media_link,datajson) VALUES (?,?,?,?,?,'text',?,'','{}') RETURNING id`), user, chat, chat, id, stamp, text).Scan(&key)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
func TestClientHistoryIsolationAndStablePagination(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db, router: mux.NewRouter()}
		s.routes()
		seedClientHistory(t, s, "client-a", "client-token-a", 100)
		seedClientHistory(t, s, "client-b", "client-token-b", 100)
		stamp := time.Now().UTC().Truncate(time.Second)
		first := insertClientMessage(t, s, "client-a", "123@lid", "a1", "first", stamp)
		second := insertClientMessage(t, s, "client-a", "123@lid", "a2", "second", stamp)
		third := insertClientMessage(t, s, "client-a", "123@lid", "a3", "third", stamp)
		foreign := insertClientMessage(t, s, "client-b", "123@lid", "secret", "private message", stamp.Add(time.Hour))
		insertClientMessage(t, s, "client-b", "999@lid", "secret2", "other private chat", stamp)
		conversations := clientGET(t, s, "client-token-a", "/chat/conversations?limit=1", 200)
		chats := conversations["chats"].([]interface{})
		if len(chats) != 1 || chats[0].(map[string]interface{})["message_id"] != "a3" {
			t.Fatalf("wrong scoped summaries: %v", conversations)
		}
		messages := clientGET(t, s, "client-token-a", "/chat/messages?chat_jid=123@lid&limit=2", 200)
		page := messages["messages"].([]interface{})
		if len(page) != 2 || page[0].(map[string]interface{})["id"] != float64(third) || page[1].(map[string]interface{})["id"] != float64(second) || messages["has_more"] != true {
			t.Fatalf("wrong first page: %v", messages)
		}
		insertClientMessage(t, s, "client-a", "123@lid", "a4", "arrived while paging", stamp.Add(time.Minute))
		older := clientGET(t, s, "client-token-a", fmt.Sprintf("/chat/messages?chat_jid=123@lid&limit=2&before=%d", second), 200)
		previous := older["messages"].([]interface{})
		if len(previous) != 1 || previous[0].(map[string]interface{})["id"] != float64(first) || older["has_more"] != false {
			t.Fatalf("unstable pagination: %v", older)
		}
		clientGET(t, s, "client-token-a", fmt.Sprintf("/chat/messages?chat_jid=123@lid&before=%d", foreign), 404)
		clientGET(t, s, "client-token-a", fmt.Sprintf("/chat/messages?chat_jid=999@lid&before=%d", first), 404)
		empty := clientGET(t, s, "client-token-a", "/chat/messages?chat_jid=999@lid", 200)
		if len(empty["messages"].([]interface{})) != 0 {
			t.Fatal("cross-user messages leaked")
		}
		rpc := executeRequest(t, s, newRequest("client", "chat.messages", map[string]interface{}{"token": "client-token-a", "chat_jid": "123@lid", "limit": 1}).toJSON(t))
		data := assertJSONRPC20Success(t, rpc, "client").(map[string]interface{})
		if len(data["messages"].([]interface{})) != 1 {
			t.Fatal("stdio pagination params not mapped")
		}
	})
}

func TestClientHistoryInputValidationAndDisabledRetention(t *testing.T) {
	s := makeTestServer(t)
	seedClientHistory(t, s, "client-validation", "client-validation-token", 0)
	for _, path := range []string{"/chat/conversations", "/chat/messages?chat_jid=123@lid"} {
		clientGET(t, s, "client-validation-token", path, 501)
		clientGET(t, s, "", path, 401)
	}
	if _, err := s.db.Exec("UPDATE users SET history=100 WHERE id='client-validation'"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/chat/conversations?limit=-1", "/chat/conversations?limit=201", "/chat/conversations?limit=bad&offset=0", "/chat/conversations?offset=-1", "/chat/conversations?offset=bad", "/chat/messages", "/chat/messages?chat_jid=a&limit=101", "/chat/messages?chat_jid=a&before=bad", "/chat/messages?chat_jid=a&before=-2"} {
		clientGET(t, s, "client-validation-token", path, 400)
	}
	empty := clientGET(t, s, "client-validation-token", "/chat/conversations", 200)
	if len(empty["chats"].([]interface{})) != 0 {
		t.Fatal("empty conversation list must be []")
	}
}
func TestOutgoingHistoryKeepsMediaMetadata(t *testing.T) {
	s := makeTestServer(t)
	seedClientHistory(t, s, "client-media", "client-media-token", 10)
	if _, err := s.db.Exec("CREATE TABLE whatsmeow_message_secrets (message_id TEXT)"); err != nil {
		t.Fatal(err)
	}
	message := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{URL: proto.String("https://example.test/encrypted"), MediaKey: []byte{1, 2, 3}, Mimetype: proto.String("audio/ogg")}}
	s.saveOutgoingMessageToHistory("client-media", "123@lid", "voice-id", "audio", "", "", 10, message)
	var raw string
	if err := s.db.Get(&raw, "SELECT datajson FROM message_history WHERE user_id='client-media'"); err != nil {
		t.Fatal(err)
	}
	var event struct {
		Info    struct{ IsFromMe bool }
		Message *waE2E.Message
	}
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatal(err)
	}
	if !event.Info.IsFromMe || event.Message.GetAudioMessage().GetURL() != message.GetAudioMessage().GetURL() {
		t.Fatal("outgoing media cannot be downloaded after refresh")
	}
}
func TestClientRecordingConvertsToOggOpus(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	codecs, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil || !bytes.Contains(codecs, []byte("libopus")) {
		t.Skip("FFmpeg lacks libopus")
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	binary.Write(&wav, binary.LittleEndian, uint32(36+16000))
	wav.WriteString("WAVEfmt ")
	binary.Write(&wav, binary.LittleEndian, uint32(16))
	binary.Write(&wav, binary.LittleEndian, uint16(1))
	binary.Write(&wav, binary.LittleEndian, uint16(1))
	binary.Write(&wav, binary.LittleEndian, uint32(8000))
	binary.Write(&wav, binary.LittleEndian, uint32(16000))
	binary.Write(&wav, binary.LittleEndian, uint16(2))
	binary.Write(&wav, binary.LittleEndian, uint16(16))
	wav.WriteString("data")
	binary.Write(&wav, binary.LittleEndian, uint32(16000))
	wav.Write(make([]byte, 16000))
	sourceWAV := append([]byte(nil), wav.Bytes()...)
	result, err := convertClientRecording(context.Background(), &wav)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	encoded := mediaBytes(t, result)
	if !bytes.HasPrefix(encoded, []byte("OggS")) || !bytes.Contains(encoded, []byte("OpusHead")) {
		t.Fatal("recording must be actual Ogg/Opus, not renamed WebM")
	}
	for _, fixture := range []struct {
		name string
		args []string
	}{
		{"browser-webm", []string{"-c:a", "libopus", "-f", "webm"}},
		{"browser-mp4", []string{"-c:a", "aac", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4"}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if fixture.name == "browser-mp4" && !bytes.Contains(codecs, []byte(" aac ")) {
				t.Skip("FFmpeg lacks AAC")
			}
			args := append([]string{"-nostdin", "-v", "error", "-i", "pipe:0"}, fixture.args...)
			command := exec.Command("ffmpeg", append(args, "pipe:1")...)
			command.Stdin = bytes.NewReader(sourceWAV)
			input, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			converted, err := convertClientRecording(context.Background(), bytes.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			defer converted.Close()
			data := mediaBytes(t, converted)
			if !bytes.HasPrefix(data, []byte("OggS")) || !bytes.Contains(data, []byte("OpusHead")) {
				t.Fatal("browser recording was not converted to Ogg/Opus")
			}
		})
	}
	for _, bad := range []string{"", "#EXTM3U\nhttp://127.0.0.1/private", "not an audio file"} {
		if file, err := convertClientRecording(context.Background(), strings.NewReader(bad)); err == nil {
			file.Close()
			t.Fatal("accepted invalid recording")
		}
	}
}

func TestClientVoiceUploadValidation(t *testing.T) {
	s := makeTestServer(t)
	seedClientHistory(t, s, "client-voice", "client-voice-token", 100)
	clientManager.SetWhatsmeowClient("client-voice", whatsmeow.NewClient(&store.Device{}, nil))
	t.Cleanup(func() { clientManager.DeleteWhatsmeowClient("client-voice") })
	for _, tc := range []struct {
		phone string
		audio bool
		want  int
	}{
		{"US.123456", true, 400}, {"123@lid", false, 400}, {"123@lid", true, 422},
	} {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		if err := form.WriteField("Phone", tc.phone); err != nil {
			t.Fatal(err)
		}
		if tc.audio {
			file, err := form.CreateFormFile("Audio", "fixture.webm")
			if err != nil {
				t.Fatal(err)
			}
			file.Write([]byte("not audio"))
		}
		form.Close()
		req := httptest.NewRequest(http.MethodPost, "/chat/send/voice", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("token", "client-voice-token")
		response := httptest.NewRecorder()
		s.router.ServeHTTP(response, req)
		if response.Code != tc.want {
			t.Fatalf("got %d, want %d: %s", response.Code, tc.want, response.Body.String())
		}
	}
}
