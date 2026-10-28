package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"time"
)

// These read endpoints deliberately do not use the legacy cross-instance
// history index. Every query, including a pagination anchor, is tenant-scoped.
func clientPageLimit(r *http.Request, fallback, maximum int) (int, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > maximum {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return n, nil
}

func (s *server) clientHistoryEnabled(w http.ResponseWriter, r *http.Request) bool {
	var retention int
	err := s.db.GetContext(r.Context(), &retention, s.db.Rebind("SELECT COALESCE(history, 0) FROM users WHERE id = ?"), r.Context().Value("userinfo").(Values).Get("Id"))
	if err != nil {
		s.Respond(w, r, http.StatusInternalServerError, errors.New("could not read history settings"))
		return false
	}
	if retention <= 0 {
		s.Respond(w, r, http.StatusNotImplemented, errors.New("message history is disabled for this user"))
		return false
	}
	return true
}

func (s *server) ListConversations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		limit, err := clientPageLimit(r, 100, 200)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			offset, err = strconv.Atoi(value)
			if offset < 0 {
				err = errors.New("offset must be nonnegative")
			}
		}
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if !s.clientHistoryEnabled(w, r) {
			return
		}
		type conversation struct {
			ChatJID     string    `db:"chat_jid" json:"chat_jid"`
			MessageID   string    `db:"message_id" json:"message_id"`
			SenderJID   string    `db:"sender_jid" json:"sender_jid"`
			Timestamp   time.Time `db:"timestamp" json:"timestamp"`
			MessageType string    `db:"message_type" json:"message_type"`
			TextContent string    `db:"text_content" json:"text_content"`
		}
		chats := []conversation{}
		query := s.db.Rebind(`SELECT chat_jid, message_id, sender_jid, timestamp, message_type, text_content FROM (
    SELECT chat_jid, message_id, sender_jid, timestamp, message_type, text_content, id,
      ROW_NUMBER() OVER (PARTITION BY chat_jid ORDER BY timestamp DESC, id DESC) AS position
    FROM message_history WHERE user_id = ?
   ) AS ranked WHERE position = 1 ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?`)
		if err := s.db.SelectContext(r.Context(), &chats, query, r.Context().Value("userinfo").(Values).Get("Id"), limit+1, offset); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not list conversations"))
			return
		}
		more := len(chats) > limit
		if more {
			chats = chats[:limit]
		}
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{"chats": chats, "has_more": more, "next_offset": offset + len(chats)}})
	}
}

func (s *server) ListChatMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		limit, err := clientPageLimit(r, 50, 100)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		chat := r.URL.Query().Get("chat_jid")
		if chat == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("chat_jid is required"))
			return
		}
		if !s.clientHistoryEnabled(w, r) {
			return
		}
		user := r.Context().Value("userinfo").(Values).Get("Id")
		query := `SELECT id, user_id, chat_jid, sender_jid, message_id, timestamp, sent_at, message_type, text_content, media_link,
    COALESCE(quoted_message_id, '') AS quoted_message_id, COALESCE(datajson, '') AS datajson
    FROM message_history WHERE user_id = ? AND chat_jid = ?`
		args := []interface{}{user, chat}
		if value := r.URL.Query().Get("before"); value != "" {
			id, parseErr := strconv.Atoi(value)
			if parseErr != nil || id < 1 {
				s.Respond(w, r, http.StatusBadRequest, errors.New("invalid before cursor"))
				return
			}
			var anchor HistoryMessage
			err = s.db.GetContext(r.Context(), &anchor, s.db.Rebind("SELECT id, timestamp FROM message_history WHERE user_id = ? AND chat_jid = ? AND id = ?"), user, chat, id)
			if err == sql.ErrNoRows {
				s.Respond(w, r, http.StatusNotFound, errors.New("history cursor is no longer available; refresh the chat"))
				return
			}
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("could not read history cursor"))
				return
			}
			query += " AND (timestamp < ? OR (timestamp = ? AND id < ?))"
			args = append(args, anchor.Timestamp, anchor.Timestamp, anchor.ID)
		}
		query += " ORDER BY timestamp DESC, id DESC LIMIT ?"
		args = append(args, limit+1)
		messages := []HistoryMessage{}
		if err = s.db.SelectContext(r.Context(), &messages, s.db.Rebind(query), args...); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not load messages"))
			return
		}
		more := len(messages) > limit
		if more {
			messages = messages[:limit]
		}
		next := 0
		if len(messages) > 0 {
			next = messages[len(messages)-1].ID
		}
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{"messages": messages, "has_more": more, "next_before": next}})
	}
}

const clientRecordingMaxBytes = 16 << 20

func convertClientRecording(parent context.Context, input io.Reader) (*mediaFile, error) {
	ctx, cancel, err := mediaContext(parent)
	if err != nil {
		return nil, err
	}
	defer cancel()
	ctx, timeoutCancel := context.WithTimeout(ctx, 45*time.Second)
	defer timeoutCancel()
	store, err := getMediaStore()
	if err != nil {
		return nil, err
	}
	release, err := store.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	source, file, err := store.create("", "application/octet-stream")
	if err != nil {
		return nil, err
	}
	defer source.Close()
	n, copyErr := io.Copy(file, io.LimitReader(input, clientRecordingMaxBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if n == 0 || n > clientRecordingMaxBytes {
		return nil, errors.New("recording must contain between 1 byte and 16 MiB")
	}
	output, out, err := store.create("voice.ogg", "audio/ogg; codecs=opus")
	if err != nil {
		return nil, err
	}
	out.Close()
	// Restrict demuxers and protocols so uploaded recordings cannot act as
	// playlists or trigger network fetches. The generated paths are private.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-protocol_whitelist", "file,pipe",
		"-format_whitelist", "matroska,webm,ogg,mov,mp4,m4a,3gp,3g2,mj2,wav,mp3,flac,aac",
		"-i", source.Path, "-map", "0:a:0", "-vn", "-t", "300", "-ac", "1", "-ar", "48000", "-c:a", "libopus", "-b:a", "24k", "-fs", "2097152", "-f", "ogg", output.Path)
	var diagnostics cappedMediaLog
	cmd.Stderr = &diagnostics
	if err = cmd.Run(); err != nil {
		output.Close()
		return nil, errors.New("could not convert recording; install FFmpeg with libopus and use a valid audio recording")
	}
	if err = output.refresh(); err != nil {
		output.Close()
		return nil, err
	}
	if output.Size == 0 {
		output.Close()
		return nil, errors.New("empty converted recording")
	}
	return output, nil
}

// SendVoiceRecording converts browser WebM/MP4 recordings before delegating to
// the established audio handler. Existing JSON audio clients are unchanged.
func (s *server) SendVoiceRecording() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, clientRecordingMaxBytes+(1<<20))
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			if r.MultipartForm != nil {
				r.MultipartForm.RemoveAll()
			}
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid or oversized recording (16 MiB maximum)"))
			return
		}
		defer r.MultipartForm.RemoveAll()
		phone := r.FormValue("Phone")
		seconds := uint64(0)
		if value := r.FormValue("Seconds"); value != "" {
			var err error
			seconds, err = strconv.ParseUint(value, 10, 32)
			if err != nil || seconds > 300 {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Seconds must be between 0 and 300"))
				return
			}
		}
		client := clientManager.GetWhatsmeowClient(r.Context().Value("userinfo").(Values).Get("Id"))
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}
		if _, err := validateMessageFields(r.Context(), client, phone, nil, nil); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		file, _, err := r.FormFile("Audio")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Audio recording is required"))
			return
		}
		defer file.Close()
		recording, err := convertClientRecording(r.Context(), file)
		if err != nil {
			s.Respond(w, r, http.StatusUnprocessableEntity, err)
			return
		}
		defer recording.Close()
		reader, err := recording.Open()
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not open recording"))
			return
		}
		defer reader.Close()
		var audio bytes.Buffer
		audio.WriteString("data:audio/ogg;base64,")
		encoder := base64.NewEncoder(base64.StdEncoding, &audio)
		if _, err = io.Copy(encoder, reader); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("could not encode recording"))
			return
		}
		encoder.Close()
		body, err := json.Marshal(map[string]interface{}{"Phone": phone, "Audio": audio.String(), "ptt": true, "mimetype": "audio/ogg; codecs=opus", "Seconds": seconds, "Id": r.FormValue("Id")})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		request := r.Clone(r.Context())
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		request.Header = r.Header.Clone()
		request.Header.Set("Content-Type", "application/json")
		s.SendAudio()(w, request)
	}
}
