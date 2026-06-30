package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/patrickmn/go-cache"
	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog/log"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// wsWriter serializa todas as escritas no WebSocket para evitar race conditions
// entre sink (audio), ping ticker e frames de controle
type wsWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func newWSWriter(conn *websocket.Conn) *wsWriter { return &wsWriter{conn: conn} }

func (w *wsWriter) WriteMessage(msgType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(msgType, data)
}

// CallWebSocket faz upgrade HTTP→WebSocket e estabelece a ponte de áudio entre
// o browser do agente e a chamada WhatsApp.
// Auth via query param ?token= (WebSocket não suporta headers customizados facilmente).
// Protocolo de frames binários: [uint32LE: nFloats][float32LE × nFloats] (PCM 16kHz mono).
// Frames de controle: JSON text frames {"type":"ended"|"error","reason":"..."}.
func (s *server) CallWebSocket() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("Token")
		}
		if token == "" {
			http.Error(w, "token required", http.StatusUnauthorized)
			return
		}
		userinfo, found := userinfocache.Get(token)
		if !found {
			// Cache vazio (ex: reinício do servidor) — busca no banco igual ao middleware HTTP
			rows, err := s.db.Query("SELECT id,name,webhook,jid,events,proxy_url,qrcode,history,hmac_key IS NOT NULL AND length(hmac_key) > 0,CASE WHEN s3_enabled THEN 'true' ELSE 'false' END,COALESCE(media_delivery, 'base64') FROM users WHERE token=$1 LIMIT 1", token)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			var (
				id, name, webhook, jid, events, proxyURL, qrcode, s3Enabled, mediaDelivery string
				history                                                                     sql.NullInt64
				hasHmac                                                                     bool
			)
			if rows.Next() {
				if err = rows.Scan(&id, &name, &webhook, &jid, &events, &proxyURL, &qrcode, &history, &hasHmac, &s3Enabled, &mediaDelivery); err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				historyStr := "0"
				if history.Valid {
					historyStr = strconv.FormatInt(history.Int64, 10)
				}
				v := Values{map[string]string{
					"Id": id, "Name": name, "Jid": jid, "Webhook": webhook, "Token": token,
					"Proxy": proxyURL, "Events": events, "Qrcode": qrcode, "History": historyStr,
					"HasHmac": strconv.FormatBool(hasHmac), "S3Enabled": s3Enabled, "MediaDelivery": mediaDelivery,
				}}
				userinfocache.Set(token, v, cache.NoExpiration)
				userinfo = v
				log.Info().Str("name", name).Msg("[VOIP-WS] User loaded from DB for WebSocket auth")
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		txtid := userinfo.(Values).Get("Id")

		callID := r.URL.Query().Get("call_id")
		if callID == "" {
			http.Error(w, "call_id required", http.StatusBadRequest)
			return
		}
		entry, ok := callManager.GetEntry(callID)
		if !ok {
			http.Error(w, "call not found", http.StatusNotFound)
			return
		}
		if entry.UserID != txtid {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		call := entry.Call
		isIncoming := entry.IsIncoming

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Str("callId", callID).Msg("[VOIP-WS] Upgrade failed")
			return
		}
		defer conn.Close()
		log.Info().Str("callId", callID).Str("user", txtid).Msg("[VOIP-WS] Agent connected")

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		writer := newWSWriter(conn)

		// Sink: caller → browser (WhatsApp audio frames → WS binary frames)
		sink := newWSSink(writer, callID)
		call.Receive(sink)

		// Source: browser → caller (WS binary frames → WhatsApp audio frames)
		src := newWSSource(ctx)
		call.Play(src)

		// Atender automaticamente apenas em chamadas entrantes
		if isIncoming {
			if err := call.Answer(); err != nil {
				log.Warn().Err(err).Str("callId", callID).Msg("[VOIP-WS] Answer failed (may already be answered)")
			}
		}

		// Quando a chamada terminar pelo lado do WhatsApp:
		// envia frame de controle ANTES de cancelar o contexto para garantir entrega
		call.OnEnd(func(reason string) {
			sendCtrlMsg(writer, "ended", reason)
			cancel()
		})

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			return nil
		})

		// Keepalive ping a cada 10s
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := writer.WriteMessage(websocket.PingMessage, nil); err != nil {
						cancel()
						return
					}
				}
			}
		}()

		// Pump de leitura: browser → caller
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Warn().Err(err).Str("callId", callID).Msg("[VOIP-WS] Read error")
				}
				return
			}
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			if msgType == websocket.BinaryMessage {
				src.push(data)
			}
		}
	}
}

func sendCtrlMsg(w *wsWriter, typ, reason string) {
	b, _ := json.Marshal(map[string]string{"type": typ, "reason": reason})
	_ = w.WriteMessage(websocket.TextMessage, b)
}

// ─────────────────────────────────────────────
// wsSink: meowcaller.AudioSink — caller → browser
// ─────────────────────────────────────────────

type wsSink struct {
	w       *wsWriter
	callID  string
	frames  int
}

func newWSSink(w *wsWriter, callID string) *wsSink { return &wsSink{w: w, callID: callID} }

func (s *wsSink) WriteFrame(frame []float32) error {
	s.frames++
	if s.frames == 1 || s.frames%500 == 0 {
		log.Info().Str("callId", s.callID).Int("frames", s.frames).Msg("[VOIP-WS] sink→browser audio frame")
	}
	buf := make([]byte, 4+len(frame)*4)
	binary.LittleEndian.PutUint32(buf[:4], uint32(len(frame)))
	for i, f := range frame {
		binary.LittleEndian.PutUint32(buf[4+i*4:], math.Float32bits(f))
	}
	return s.w.WriteMessage(websocket.BinaryMessage, buf)
}

func (s *wsSink) Close() error { return nil }

// ─────────────────────────────────────────────
// wsSource: meowcaller.AudioSource — browser → caller
// FrameSamples = 960 (60ms @ 16kHz, conforme meowcaller/audio.go)
// ─────────────────────────────────────────────

type wsSource struct {
	ctx     context.Context
	ch      chan []float32
	pending []float32
}

func newWSSource(ctx context.Context) *wsSource {
	return &wsSource{
		ctx: ctx,
		ch:  make(chan []float32, 64),
	}
}

func (s *wsSource) push(data []byte) {
	if len(data) < 4 {
		return
	}
	n := int(binary.LittleEndian.Uint32(data[:4]))
	if len(data) < 4+n*4 {
		return
	}
	frames := make([]float32, n)
	for i := 0; i < n; i++ {
		frames[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[4+i*4:]))
	}
	select {
	case s.ch <- frames:
	default: // descarta se browser estiver atrasado — prefere perda a backpressure
	}
}

func (s *wsSource) ReadFrame() ([]float32, error) {
	const frameSamples = meowcaller.FrameSamples // 960
	for len(s.pending) < frameSamples {
		select {
		case <-s.ctx.Done():
			if len(s.pending) == 0 {
				return nil, io.EOF
			}
			// zero-pad o último frame incompleto
			pad := make([]float32, frameSamples-len(s.pending))
			s.pending = append(s.pending, pad...)
		case chunk := <-s.ch:
			s.pending = append(s.pending, chunk...)
		}
	}
	frame := make([]float32, frameSamples)
	copy(frame, s.pending)
	s.pending = s.pending[frameSamples:]
	return frame, nil
}

func (s *wsSource) Close() error { return nil }
