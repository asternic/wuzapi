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

// wsWriter serializa todas as escritas no WebSocket para evitar race conditions entre
// sink (áudio), sink de vídeo, ping ticker e frames de controle. Áudio/controle tem
// prioridade: um frame de vídeo grande nunca fica na frente de um frame de áudio que já
// esteja pronto pra sair — só disputam a escrita se coincidirem no exato mesmo instante,
// e nesse caso o loop abaixo sempre esvazia a fila de prioridade primeiro.
type wsWriter struct {
	conn      *websocket.Conn
	priority  chan wsWriteMsg
	secondary chan wsWriteMsg
	done      chan struct{}
}

type wsWriteMsg struct {
	msgType int
	data    []byte
}

func newWSWriter(conn *websocket.Conn) *wsWriter {
	w := &wsWriter{
		conn:      conn,
		priority:  make(chan wsWriteMsg, 64),
		secondary: make(chan wsWriteMsg, 8),
		done:      make(chan struct{}),
	}
	go w.pump()
	return w
}

func (w *wsWriter) pump() {
	for {
		select {
		case msg := <-w.priority:
			_ = w.conn.WriteMessage(msg.msgType, msg.data)
			continue
		default:
		}
		select {
		case msg := <-w.priority:
			_ = w.conn.WriteMessage(msg.msgType, msg.data)
		case msg := <-w.secondary:
			_ = w.conn.WriteMessage(msg.msgType, msg.data)
		case <-w.done:
			return
		}
	}
}

// WriteMessage enfileira áudio/controle/ping com prioridade — nunca espera uma escrita
// de vídeo em andamento na fila (só a escrita física atual, que é sempre curta).
func (w *wsWriter) WriteMessage(msgType int, data []byte) error {
	select {
	case w.priority <- wsWriteMsg{msgType, data}:
		return nil
	case <-w.done:
		return websocket.ErrCloseSent
	}
}

// WriteVideoMessage enfileira vídeo com prioridade menor que áudio/controle.
func (w *wsWriter) WriteVideoMessage(msgType int, data []byte) error {
	select {
	case w.secondary <- wsWriteMsg{msgType, data}:
		return nil
	case <-w.done:
		return websocket.ErrCloseSent
	default:
		return nil // fila cheia: descarta o frame de vídeo em vez de acumular atraso
	}
}

func (w *wsWriter) Close() {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
}

// CallWebSocket faz upgrade HTTP→WebSocket e estabelece a ponte de áudio entre
// o browser do agente e a chamada WhatsApp.
// Auth via query param ?token= (WebSocket não suporta headers customizados facilmente).
// Protocolo de frames binários: [byte prefixo][payload]. Prefixo 0x00 = áudio
// ([uint32LE: nFloats][float32LE × nFloats], PCM 16kHz mono); prefixo 0x01 = vídeo
// ([uint32LE: nBytes][bytes × nBytes], um access unit H.264 Annex-B).
// Frames de controle: JSON text frames {"type":"ended"|"error"|"video_state"|"keyframe_request",...}.
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
		defer writer.Close()

		// Source: browser → caller (WS binary frames → WhatsApp audio frames)
		src := newWSSource(ctx)
		call.Play(src)

		videoSrc := &wsVideoSource{call: call, callID: callID}
		call.ReceiveVideo(newWSVideoSink(writer, callID))

		call.OnVideoState(func(state meowcaller.VideoState) {
			b, _ := json.Marshal(map[string]interface{}{
				"type":        "video_state",
				"active":      state.Active,
				"upgrade":     state.Upgrade,
				"orientation": state.Orientation,
			})
			_ = writer.WriteMessage(websocket.TextMessage, b)
		})

		call.OnVideoKeyframeRequest(func() {
			b, _ := json.Marshal(map[string]string{"type": "keyframe_request"})
			_ = writer.WriteMessage(websocket.TextMessage, b)
		})

		if isIncoming {
			// Chamada entrante: registra sink agora e atende
			sink := newWSSink(writer, callID)
			call.Receive(sink)
			if err := call.Answer(); err != nil {
				log.Warn().Err(err).Str("callId", callID).Msg("[VOIP-WS] Answer failed (may already be answered)")
			}
		} else if entry.PreSink != nil {
			// Chamada sainte: preSink já registrado no DialCall — bombeia frames para o browser
			go entry.PreSink.pump(ctx, writer)
		}

		// Quando a chamada terminar pelo lado do WhatsApp:
		// envia frame de controle ANTES de cancelar o contexto para garantir entrega.
		// Usa AddEndListener (não call.OnEnd direto) porque a lib meowcaller só
		// guarda um callback por chamada — chamar OnEnd aqui sobrescreveria o
		// registro já feito em DialCall/OnIncomingCall e o backend nunca saberia
		// que a chamada terminou.
		callManager.AddEndListener(callID, func(reason string) {
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
			if msgType == websocket.BinaryMessage && len(data) > 0 {
				switch data[0] {
				case 0x00:
					src.push(data)
				case 0x01:
					videoSrc.push(data)
				}
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

// rmsFloat32Slice computa o RMS de um frame PCM para diagnóstico de distorção/silêncio.
func rmsFloat32Slice(frame []float32) float64 {
	if len(frame) == 0 {
		return 0
	}
	var sumSq float64
	for _, f := range frame {
		sumSq += float64(f * f)
	}
	return sumSq / float64(len(frame))
}

func (s *wsSink) WriteFrame(frame []float32) error {
	s.frames++
	if s.frames == 1 || s.frames%500 == 0 {
		log.Info().Str("callId", s.callID).Int("frames", s.frames).Int("samples", len(frame)).
			Float64("rms", rmsFloat32Slice(frame)).Msg("[VOIP-WS] sink→browser audio frame")
	}
	buf := make([]byte, 5+len(frame)*4)
	buf[0] = 0x00
	binary.LittleEndian.PutUint32(buf[1:5], uint32(len(frame)))
	for i, f := range frame {
		binary.LittleEndian.PutUint32(buf[5+i*4:], math.Float32bits(f))
	}
	return s.w.WriteMessage(websocket.BinaryMessage, buf)
}

func (s *wsSink) Close() error { return nil }

// ─────────────────────────────────────────────
// preSink: buffer para chamadas saintes
// Registrado imediatamente no DialCall para não perder frames
// enquanto o agente ainda não abriu o WebSocket.
// ─────────────────────────────────────────────

type preSink struct {
	ch     chan []float32
	callID string
	frames int
	done   chan struct{}
	once   sync.Once
}

func newPreSink(callID string) *preSink {
	return &preSink{
		ch:     make(chan []float32, 1024), // ~60s de áudio a 60ms/frame
		callID: callID,
		done:   make(chan struct{}),
	}
}

func (s *preSink) WriteFrame(frame []float32) error {
	s.frames++
	if s.frames == 1 || s.frames%500 == 0 {
		log.Info().Str("callId", s.callID).Int("frames", s.frames).Msg("[VOIP] preSink buffering audio frame from caller")
	}
	cp := make([]float32, len(frame))
	copy(cp, frame)
	select {
	case s.ch <- cp:
	default: // buffer cheio — descarta frame mais antigo e insere novo
		select {
		case <-s.ch:
		default:
		}
		select {
		case s.ch <- cp:
		default:
		}
	}
	return nil
}

func (s *preSink) Close() error {
	s.once.Do(func() { close(s.done) })
	return nil
}

// pump lê frames do preSink e os envia via WebSocket ao browser.
// Deve ser executado em goroutine separada após o agente conectar.
func (s *preSink) pump(ctx context.Context, w *wsWriter) {
	total := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			// drena o que ainda estiver no canal antes de sair
			for {
				select {
				case frame := <-s.ch:
					encodeAndSend(w, frame)
					total++
				default:
					log.Info().Str("callId", s.callID).Int("total", total).Msg("[VOIP-WS] preSink pump exiting")
					return
				}
			}
		case frame := <-s.ch:
			if err := encodeAndSend(w, frame); err != nil {
				return
			}
			total++
			if total == 1 || total%500 == 0 {
				log.Info().Str("callId", s.callID).Int("frames", total).
					Float64("rms", rmsFloat32Slice(frame)).Msg("[VOIP-WS] preSink→browser audio frame")
			}
		}
	}
}

func encodeAndSend(w *wsWriter, frame []float32) error {
	buf := make([]byte, 5+len(frame)*4)
	buf[0] = 0x00
	binary.LittleEndian.PutUint32(buf[1:5], uint32(len(frame)))
	for i, f := range frame {
		binary.LittleEndian.PutUint32(buf[5+i*4:], math.Float32bits(f))
	}
	return w.WriteMessage(websocket.BinaryMessage, buf)
}

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
	if len(data) < 5 || data[0] != 0x00 {
		return
	}
	body := data[1:]
	n := int(binary.LittleEndian.Uint32(body[:4]))
	if len(body) < 4+n*4 {
		return
	}
	frames := make([]float32, n)
	for i := 0; i < n; i++ {
		frames[i] = math.Float32frombits(binary.LittleEndian.Uint32(body[4+i*4:]))
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

// ─────────────────────────────────────────────
// wsVideoSink: meowcaller.VideoSink — caller → browser
// Frame: [0x01][uint32LE nBytes][bytes × nBytes] — um access unit H.264 Annex-B
// ─────────────────────────────────────────────

type wsVideoSink struct {
	w      *wsWriter
	callID string
}

func newWSVideoSink(w *wsWriter, callID string) *wsVideoSink {
	return &wsVideoSink{w: w, callID: callID}
}

func (s *wsVideoSink) WriteVideo(accessUnit []byte) error {
	buf := make([]byte, 5+len(accessUnit))
	buf[0] = 0x01
	binary.LittleEndian.PutUint32(buf[1:5], uint32(len(accessUnit)))
	copy(buf[5:], accessUnit)
	return s.w.WriteVideoMessage(websocket.BinaryMessage, buf)
}

func (s *wsVideoSink) Close() error { return nil }

// ─────────────────────────────────────────────
// wsVideoSource: meowcaller video source — browser → caller
// Mesmo protocolo de frame do wsVideoSink, mas em sentido contrário.
// ─────────────────────────────────────────────

type wsVideoSource struct {
	call     *meowcaller.Call
	callID   string
	sentOnce sync.Once
}

func (s *wsVideoSource) push(data []byte) {
	if len(data) < 5 || data[0] != 0x01 {
		return
	}
	body := data[1:]
	n := int(binary.LittleEndian.Uint32(body[:4]))
	if len(body) < 4+n {
		return
	}
	accessUnit := make([]byte, n)
	copy(accessUnit, body[4:4+n])
	if err := s.call.SendVideoWithDuration(accessUnit, 0); err != nil {
		log.Warn().Err(err).Str("callId", s.callID).Msg("[VOIP] Failed to send outbound video frame")
		return
	}
	s.sentOnce.Do(func() {
		log.Info().Str("callId", s.callID).Int("bytes", n).Msg("[VOIP] First outbound video frame sent")
	})
}
