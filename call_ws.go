package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog/log"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     func(r *http.Request) bool { return true },
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
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		txtid := userinfo.(Values).Get("Id")

		callID := r.URL.Query().Get("call_id")
		if callID == "" {
			http.Error(w, "call_id required", http.StatusBadRequest)
			return
		}
		call, ownerID, ok := callManager.Get(callID)
		if !ok {
			http.Error(w, "call not found", http.StatusNotFound)
			return
		}
		if ownerID != txtid {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Str("callId", callID).Msg("[VOIP-WS] Upgrade failed")
			return
		}
		defer conn.Close()
		log.Info().Str("callId", callID).Str("user", txtid).Msg("[VOIP-WS] Agent connected")

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Sink: caller → browser (WhatsApp audio frames → WS binary frames)
		sink := newWSSink(conn)
		call.Receive(sink)

		// Source: browser → caller (WS binary frames → WhatsApp audio frames)
		src := newWSSource(ctx)
		call.Play(src)

		// Atender automaticamente ao conectar o WebSocket
		if err := call.Answer(); err != nil {
			log.Warn().Err(err).Str("callId", callID).Msg("[VOIP-WS] Answer failed (may already be answered)")
		}

		// Cancelar contexto quando a chamada terminar (vinda do lado do WhatsApp)
		call.OnEnd(func(reason string) {
			cancel()
			sendCtrlMsg(conn, "ended", reason)
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
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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

func sendCtrlMsg(conn *websocket.Conn, typ, reason string) {
	b, _ := json.Marshal(map[string]string{"type": typ, "reason": reason})
	_ = conn.WriteMessage(websocket.TextMessage, b)
}

// ─────────────────────────────────────────────
// wsSink: meowcaller.AudioSink — caller → browser
// ─────────────────────────────────────────────

type wsSink struct {
	conn *websocket.Conn
}

func newWSSink(conn *websocket.Conn) *wsSink { return &wsSink{conn: conn} }

func (s *wsSink) WriteFrame(frame []float32) error {
	buf := make([]byte, 4+len(frame)*4)
	binary.LittleEndian.PutUint32(buf[:4], uint32(len(frame)))
	for i, f := range frame {
		binary.LittleEndian.PutUint32(buf[4+i*4:], math.Float32bits(f))
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, buf)
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
