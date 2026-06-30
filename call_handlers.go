package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog/log"
)

func (s *server) AnswerCall() http.HandlerFunc {
	type req struct {
		CallID string `json:"call_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CallID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("call_id is required"))
			return
		}
		call, ownerID, ok := callManager.Get(body.CallID)
		if !ok {
			s.Respond(w, r, http.StatusNotFound, errors.New("call not found"))
			return
		}
		if ownerID != txtid {
			s.Respond(w, r, http.StatusForbidden, errors.New("call belongs to another user"))
			return
		}
		if err := call.Answer(); err != nil {
			log.Error().Err(err).Str("callId", body.CallID).Msg("[VOIP] Answer failed")
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		log.Info().Str("callId", body.CallID).Msg("[VOIP] Call answered")
		s.Respond(w, r, http.StatusOK, map[string]interface{}{"success": true, "call_id": body.CallID})
	}
}

func (s *server) HangupCall() http.HandlerFunc {
	type req struct {
		CallID string `json:"call_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CallID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("call_id is required"))
			return
		}
		call, ownerID, ok := callManager.Get(body.CallID)
		if !ok {
			s.Respond(w, r, http.StatusNotFound, errors.New("call not found"))
			return
		}
		if ownerID != txtid {
			s.Respond(w, r, http.StatusForbidden, errors.New("call belongs to another user"))
			return
		}
		if err := call.Hangup(); err != nil {
			log.Error().Err(err).Str("callId", body.CallID).Msg("[VOIP] Hangup failed")
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		callManager.Delete(body.CallID)
		log.Info().Str("callId", body.CallID).Msg("[VOIP] Call hung up")
		s.Respond(w, r, http.StatusOK, map[string]interface{}{"success": true})
	}
}

// PlayAudio faz download do audio_url para um arquivo temp e toca para o caller.
func (s *server) PlayAudio() http.HandlerFunc {
	type req struct {
		CallID   string `json:"call_id"`
		AudioURL string `json:"audio_url"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CallID == "" || body.AudioURL == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("call_id and audio_url are required"))
			return
		}
		call, ownerID, ok := callManager.Get(body.CallID)
		if !ok {
			s.Respond(w, r, http.StatusNotFound, errors.New("call not found"))
			return
		}
		if ownerID != txtid {
			s.Respond(w, r, http.StatusForbidden, errors.New("call belongs to another user"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, body.AudioURL, nil)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			s.Respond(w, r, http.StatusBadGateway, err)
			return
		}
		defer resp.Body.Close()

		ext := filepath.Ext(body.AudioURL)
		if ext == "" {
			ext = ".mp3"
		}
		tmp, err := os.CreateTemp("", "wuzapi-voip-*"+ext)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)

		if _, err := io.Copy(tmp, resp.Body); err != nil {
			tmp.Close()
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		tmp.Close()

		var src meowcaller.AudioSource
		switch ext {
		case ".wav":
			src, err = meowcaller.WAVFile(tmpPath)
		case ".opus":
			src, err = meowcaller.OpusFile(tmpPath)
		default:
			src, err = meowcaller.MP3File(tmpPath)
		}
		if err != nil {
			s.Respond(w, r, http.StatusUnprocessableEntity, err)
			return
		}

		call.Play(src)
		log.Info().Str("callId", body.CallID).Str("url", body.AudioURL).Msg("[VOIP] Playing audio")
		s.Respond(w, r, http.StatusOK, map[string]interface{}{"success": true})
	}
}

// ActiveCalls lista as chamadas ativas do usuário autenticado.
func (s *server) ActiveCalls() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		entries := callManager.ListByUser(txtid)
		type callInfo struct {
			CallID string `json:"call_id"`
			Caller string `json:"caller"`
			State  string `json:"state"`
		}
		result := make([]callInfo, 0, len(entries))
		for _, e := range entries {
			result = append(result, callInfo{
				CallID: e.Call.ID(),
				Caller: e.Call.Peer().User,
				State:  fmt.Sprintf("%d", e.Call.State()),
			})
		}
		s.Respond(w, r, http.StatusOK, map[string]interface{}{"calls": result})
	}
}
