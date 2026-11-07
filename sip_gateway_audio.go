package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/sdp/v3"
	"github.com/purpshell/meowcaller"
	"github.com/rs/zerolog/log"
	"github.com/zaf/g711"
)

type incomingCall interface {
	Answer() error
	Reject() error
	Hangup() error
	Play(meowcaller.AudioSource) *meowcaller.Player
	Receive(meowcaller.AudioSink)
}

func bridgeSIPCall(ctx context.Context, call incomingCall, dialogs *sipgo.DialogClientCache, dest sip.Uri, ip net.IP, number, name string) error {
	answered := false
	defer func() {
		if answered {
			_ = call.Hangup()
		} else {
			_ = call.Reject()
		}
	}()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
	if err != nil {
		return err
	}
	defer conn.Close()

	offer := sipAudioSDP(ip, conn.LocalAddr().(*net.UDPAddr).Port)
	dial, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if name == "" {
		name = "WhatsApp"
	}
	from := sip.FromHeader{
		DisplayName: name,
		Address:     sip.Uri{Scheme: "sip", User: "whatsapp", Host: ip.String()},
	}
	if number != "" {
		from.Address.User = "+" + number
	}
	from.Params.Add("tag", sip.GenerateTagN(16))
	dialog, err := dialogs.Invite(dial, dest, []byte(offer), &from, sip.NewHeader("Content-Type", "application/sdp"))
	if err != nil {
		return err
	}
	defer dialog.Close()
	if err := dialog.WaitAnswer(dial, sipgo.AnswerOptions{}); err != nil &&
		(dialog.InviteResponse == nil || !dialog.InviteResponse.IsSuccess()) {
		return err
	}
	ack, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if err := dialog.Ack(ack); err != nil {
		return err
	}
	defer func() {
		if dialog.Context().Err() == nil {
			bye, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			_ = dialog.Bye(bye)
		}
	}()
	peer, err := mediaAddress(dialog.InviteResponse.Body())
	if err != nil {
		return err
	}
	if ctx.Err() != nil || dialog.Context().Err() != nil {
		return nil
	}

	audioCtx, stopAudio := context.WithCancel(ctx)
	defer stopAudio()
	startSIPAudio(audioCtx, call, conn, peer)
	if err := call.Answer(); err != nil {
		return err
	}
	answered = true
	select {
	case <-ctx.Done():
	case <-dialog.Context().Done():
	}
	return nil
}

func mediaAddress(body []byte) (*net.UDPAddr, error) {
	var answer sdp.SessionDescription
	if err := answer.Unmarshal(body); err != nil {
		return nil, err
	}
	for _, media := range answer.MediaDescriptions {
		port := media.MediaName.Port.Value
		if media.MediaName.Media != "audio" || port < 1 || port > 65535 ||
			!slices.Equal(media.MediaName.Protos, []string{"RTP", "AVP"}) ||
			!slices.Contains(media.MediaName.Formats, "0") {
			continue
		}
		connection := media.ConnectionInformation
		if connection == nil {
			connection = answer.ConnectionInformation
		}
		if connection != nil && connection.Address != nil {
			ip := net.ParseIP(connection.Address.Address).To4()
			if ip != nil && !ip.IsUnspecified() {
				return &net.UDPAddr{IP: ip, Port: port}, nil
			}
		}
	}
	return nil, fmt.Errorf("Asterisk did not offer usable IPv4 PCMU audio")
}

type audioBridge struct {
	ctx        context.Context
	closed     atomic.Bool
	input      chan float32
	output     chan [160]byte
	outputMu   sync.Mutex
	partial    [160]byte
	partialLen int
	firstFrame sync.Once
}

func (m *audioBridge) Close() error {
	m.closed.Store(true)
	return nil
}

func (m *audioBridge) ReadFrame() ([]float32, error) {
	if m.closed.Load() || m.ctx.Err() != nil {
		return nil, io.EOF
	}
	frame := make([]float32, meowcaller.FrameSamples)
	for i := 0; i < len(frame); i += 2 {
		select {
		case sample := <-m.input:
			frame[i], frame[i+1] = sample, sample
		default:
			return frame, nil
		}
	}
	return frame, nil
}

func (m *audioBridge) enqueue(frame []float32) {
	if len(frame) < 2 || m.ctx.Err() != nil {
		return
	}
	m.firstFrame.Do(func() {
		log.Debug().Int("samples", len(frame)).Msg("Received first WhatsApp PCM frame for SIP")
	})
	m.outputMu.Lock()
	defer m.outputMu.Unlock()
	for i := 0; i+1 < len(frame); i += 2 {
		average := (frame[i] + frame[i+1]) / 2
		m.partial[m.partialLen] = g711.EncodeUlawFrame(int16(max(-1, min(1, average)) * 32767))
		m.partialLen++
		if m.partialLen == len(m.partial) {
			select {
			case m.output <- m.partial:
			default:
			}
			m.partialLen = 0
		}
	}
}

func (m *audioBridge) receive(conn *net.UDPConn, peer *net.UDPAddr) {
	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !addr.IP.Equal(peer.IP) || addr.Port != peer.Port {
			continue
		}
		var packet rtp.Packet
		if packet.Unmarshal(buf[:n]) != nil || packet.PayloadType != 0 {
			continue
		}
		for _, value := range packet.Payload {
			select {
			case m.input <- float32(g711.DecodeUlawFrame(value)) / 32768:
			default:
			}
		}
	}
}

func (m *audioBridge) send(conn *net.UDPConn, peer *net.UDPAddr) {
	packetizer := rtp.NewPacketizer(172, 0, 0x57414c42, &codecs.G711Payloader{}, rtp.NewRandomSequencer(), 8000)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var sent bool
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			var frame [160]byte
			select {
			case frame = <-m.output:
			default:
				packetizer.SkipSamples(160)
				continue
			}
			packet := packetizer.Packetize(frame[:], 160)[0]
			raw, err := packet.Marshal()
			if err != nil {
				return
			}
			if _, err := conn.WriteToUDP(raw, peer); err != nil {
				log.Warn().Err(err).Str("peer", peer.String()).Msg("Failed to send SIP RTP")
				return
			}
			if !sent {
				sent = true
				log.Debug().Str("local", conn.LocalAddr().String()).Str("peer", peer.String()).Msg("Sent first RTP packet to Asterisk")
			}
		}
	}
}

func sipAudioSDP(ip net.IP, port int) string {
	return fmt.Sprintf("v=0\r\no=- 1 1 IN IP4 %[1]s\r\ns=WuzAPI\r\nc=IN IP4 %[1]s\r\nt=0 0\r\nm=audio %[2]d RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=ptime:20\r\na=sendrecv\r\n", ip, port)
}

func startSIPAudio(ctx context.Context, call incomingCall, conn *net.UDPConn, peer *net.UDPAddr) {
	media := &audioBridge{ctx: ctx, input: make(chan float32, 1600), output: make(chan [160]byte, 9)}
	go media.receive(conn, peer)
	go media.send(conn, peer)
	call.Receive(meowcaller.SinkFunc(media.enqueue))
	call.Play(media)
}
