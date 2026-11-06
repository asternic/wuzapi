package main

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/pion/rtp"
	"github.com/purpshell/meowcaller"
)

func TestPCMUOverUDP(t *testing.T) {
	listen := func() *net.UDPConn {
		t.Helper()
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	gateway, pbx := listen(), listen()
	sdp := fmt.Sprintf("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=test\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio %d RTP/AVP 0\r\n", pbx.LocalAddr().(*net.UDPAddr).Port)
	peer, err := mediaAddress([]byte(sdp))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	media := &audioBridge{ctx: ctx, input: make(chan float32, 1600), output: make(chan [160]byte, 9)}
	go media.receive(gateway, peer)
	go media.send(gateway, peer)

	frame := make([]float32, meowcaller.FrameSamples)
	for i := range frame {
		frame[i] = 0.25
	}
	for offset := 0; offset < len(frame); offset += 320 {
		media.enqueue(frame[offset : offset+320])
	}
	if err := pbx.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	var previous rtp.Packet
	for i := 0; i < 3; i++ {
		n, from, err := pbx.ReadFromUDP(buf)
		if err != nil {
			t.Fatal(err)
		}
		var packet rtp.Packet
		if err := packet.Unmarshal(buf[:n]); err != nil {
			t.Fatal(err)
		}
		if packet.PayloadType != 0 || len(packet.Payload) != 160 {
			t.Fatalf("unexpected payload type/length: %d/%d", packet.PayloadType, len(packet.Payload))
		}
		if i > 0 && (packet.SequenceNumber != previous.SequenceNumber+1 || packet.Timestamp != previous.Timestamp+160) {
			t.Fatal("RTP sequence or timestamp discontinuity")
		}
		previous = packet
		if _, err := pbx.WriteToUDP(buf[:n], from); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(time.Second); len(media.input) < 480; {
		if time.Now().After(deadline) {
			t.Fatal("gateway did not receive the RTP loopback")
		}
		time.Sleep(time.Millisecond)
	}
	received, err := media.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	for i, sample := range received {
		if sample < 0.23 || sample > 0.27 {
			t.Fatalf("sample %d: got %f, want approximately 0.25", i, sample)
		}
	}
}

type testCall struct {
	answered bool
	rejected bool
	hungUp   bool
	cancel   context.CancelFunc
}

func (c *testCall) Answer() error {
	c.answered = true
	c.cancel()
	return nil
}
func (c *testCall) Reject() error                                { c.rejected = true; return nil }
func (c *testCall) Hangup() error                                { c.hungUp = true; return nil }
func (*testCall) Play(meowcaller.AudioSource) *meowcaller.Player { return nil }
func (*testCall) Receive(meowcaller.AudioSink)                   {}

func TestBridgeSIPAnswerAndUnavailable(t *testing.T) {
	for _, status := range []int{200, 480} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			ip := net.IPv4(127, 0, 0, 1)
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			addr := conn.LocalAddr().(*net.UDPAddr)
			pbx, err := sipgo.NewUA()
			if err != nil {
				t.Fatal(err)
			}
			defer pbx.Close()
			server, err := sipgo.NewServer(pbx)
			if err != nil {
				t.Fatal(err)
			}
			var dest sip.Uri
			if err := sip.ParseUri("sip:wa-in@"+addr.String(), &dest); err != nil {
				t.Fatal(err)
			}
			server.OnInvite(func(req *sip.Request, tx sip.ServerTransaction) {
				from := req.From()
				if from == nil || from.Address.User != "+15551234567" || from.DisplayName != "María" || !from.Params.Has("tag") {
					t.Errorf("unexpected INVITE From: %v", from)
				}
				var body []byte
				if status == 200 {
					body = []byte("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=test\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 9000 RTP/AVP 0\r\n")
				}
				response := sip.NewResponseFromRequest(req, status, "Test", body)
				response.AppendHeader(&sip.ContactHeader{Address: dest})
				if err := tx.Respond(response); err != nil {
					t.Error(err)
				}
			})
			server.OnAck(func(*sip.Request, sip.ServerTransaction) {})
			server.OnBye(func(req *sip.Request, tx sip.ServerTransaction) {
				if err := tx.Respond(sip.NewResponseFromRequest(req, 200, "OK", nil)); err != nil {
					t.Error(err)
				}
			})
			go func() { _ = server.ServeUDP(conn) }()

			ua, err := sipgo.NewUA()
			if err != nil {
				t.Fatal(err)
			}
			defer ua.Close()
			client, err := sipgo.NewClient(ua, sipgo.WithClientHostname(ip.String()))
			if err != nil {
				t.Fatal(err)
			}
			dialogs := sipgo.NewDialogClientCache(client, sip.ContactHeader{Address: sip.Uri{User: "lab", Host: ip.String()}})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			call := &testCall{cancel: cancel}
			err = bridgeSIPCall(ctx, call, dialogs, dest, ip, "15551234567", "María")
			if status == 200 {
				if err != nil || !call.answered || !call.hungUp || call.rejected {
					t.Fatalf("successful SIP dialog: call=%+v error=%v", call, err)
				}
			} else if err == nil || !call.rejected || call.answered || call.hungUp {
				t.Fatalf("unavailable SIP destination: call=%+v error=%v", call, err)
			}
		})
	}
}
