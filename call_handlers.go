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
	"github.com/purpshell/meowcaller/signaling"
	"github.com/rs/zerolog/log"
)

func respondJSON(s *server, w http.ResponseWriter, r *http.Request, status int, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to serialize response"))
		return
	}
	s.Respond(w, r, status, string(b))
}

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
		respondJSON(s, w, r, http.StatusOK, map[string]interface{}{"success": true, "call_id": body.CallID})
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

		// Caminho principal: chamada gerenciada pelo meowcaller
		call, ownerID, ok := callManager.Get(body.CallID)
		if ok {
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
			log.Info().Str("callId", body.CallID).Msg("[VOIP] Call hung up via meowcaller")
			respondJSON(s, w, r, http.StatusOK, map[string]interface{}{"success": true})
			return
		}

		// Fallback: chamada entrante não capturada pelo meowcaller (ex: <silence reason="privacy"/>)
		pending, hasPending := callManager.GetPending(body.CallID)
		if !hasPending {
			s.Respond(w, r, http.StatusNotFound, errors.New("call not found"))
			return
		}
		if pending.UserID != txtid {
			s.Respond(w, r, http.StatusForbidden, errors.New("call belongs to another user"))
			return
		}
		waClient := clientManager.GetWhatsmeowClient(txtid)
		if waClient == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no active session"))
			return
		}
		// Para chamadas silence/privacy: envia <reject> via DangerousInternals.
		// Tenta primeiro o JID de telefone (@s.whatsapp.net), depois o LID (@lid),
		// pois o servidor pode rotear de forma diferente para cada formato.
		routingJID := pending.CallerJID
		if !pending.CallerAltJID.IsEmpty() {
			routingJID = pending.CallerAltJID
		}
		rej := signaling.BuildReject(body.CallID, routingJID, pending.CallerJID)
		rej.Attrs["id"] = waClient.GenerateMessageID()
		if err := waClient.DangerousInternals().SendNode(context.Background(), rej); err != nil {
			log.Error().Err(err).Str("callId", body.CallID).Str("routingJID", routingJID.String()).Msg("[VOIP] reject fallback failed")
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		callManager.Delete(body.CallID)
		log.Info().Str("callId", body.CallID).Str("routingJID", routingJID.String()).Msg("[VOIP] Call rejected via signaling fallback")
		respondJSON(s, w, r, http.StatusOK, map[string]interface{}{"success": true})
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
		respondJSON(s, w, r, http.StatusOK, map[string]interface{}{"success": true})
	}
}

// DialCall inicia uma chamada de saída para o número informado.
func (s *server) DialCall() http.HandlerFunc {
	type req struct {
		Phone string `json:"phone"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("phone is required"))
			return
		}

		meowClient := clientManager.GetMeowcallerClient(txtid)
		if meowClient == nil {
			s.Respond(w, r, http.StatusServiceUnavailable, errors.New("whatsapp session not connected"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		call, err := meowClient.Call(ctx, body.Phone)
		if err != nil {
			log.Error().Err(err).Str("phone", body.Phone).Msg("[VOIP] Dial failed")
			s.Respond(w, r, http.StatusBadGateway, err)
			return
		}

		callID := call.ID()

		// Registra preSink antes de armazenar no callManager para garantir
		// que frames do contato não sejam perdidos antes do WebSocket conectar.
		pre := newPreSink(callID)
		call.Receive(pre)

		entry := &CallEntry{Call: call, UserID: txtid, IsIncoming: false, PreSink: pre}
		callManager.mu.Lock()
		callManager.calls[callID] = entry
		callManager.mu.Unlock()

		call.OnEnd(func(reason string) {
			pre.Close()
			callManager.Delete(callID)
			mycli := clientManager.GetMyClient(txtid)
			if mycli != nil {
				endMap := map[string]interface{}{
					"type":   "CallTerminate",
					"callId": callID,
					"caller": body.Phone,
					"reason": reason,
				}
				sendEventWithWebHook(mycli, endMap, "")
			}
		})

		call.OnPeerAccept(func() {
			mycli := clientManager.GetMyClient(txtid)
			if mycli != nil {
				acceptMap := map[string]interface{}{
					"type":   "CallAccept",
					"callId": callID,
					"caller": body.Phone,
				}
				sendEventWithWebHook(mycli, acceptMap, "")
			}
			log.Info().Str("callId", callID).Msg("[VOIP] Peer accepted outgoing call")
		})

		log.Info().Str("callId", callID).Str("phone", body.Phone).Msg("[VOIP] Outgoing call placed")
		respondJSON(s, w, r, http.StatusOK, map[string]interface{}{"success": true, "call_id": callID})
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
		respondJSON(s, w, r, http.StatusOK, map[string]interface{}{"calls": result})
	}
}
