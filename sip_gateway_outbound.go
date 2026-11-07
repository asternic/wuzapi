package main

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/purpshell/meowcaller"
	"github.com/rs/zerolog/log"
)

type sipOutboundSession struct {
	peers   []netip.Addr
	calls   *meowcaller.Client
	ctx     context.Context
	reserve func() (release func(), status int)
}

func (g *sipGateway) registerSIPHandlers(server *sipgo.Server, client *sipgo.Client, addr *net.UDPAddr) {
	outbound := sipgo.NewDialogServerCache(client, sip.ContactHeader{Address: sip.Uri{User: "wuzapi", Host: addr.IP.String(), Port: addr.Port}})
	server.OnInvite(func(req *sip.Request, tx sip.ServerTransaction) {
		g.dialWhatsApp(req, tx, outbound)
	})
	trustedDialog := func(req *sip.Request) bool {
		dialog, err := outbound.MatchDialogRequest(req)
		return err == nil && sipSourceIP(req) == sipSourceIP(dialog.InviteRequest)
	}
	server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {
		if trustedDialog(req) {
			_ = outbound.ReadAck(req, tx)
		}
	})
	server.OnBye(func(req *sip.Request, tx sip.ServerTransaction) {
		if g.dialogs.ReadBye(req, tx) == nil {
			return
		}
		if trustedDialog(req) && outbound.ReadBye(req, tx) == nil {
			return
		}
		_ = tx.Respond(sip.NewResponseFromRequest(req, 481, "Call Does Not Exist", nil))
	})
}

func respondSIP(req *sip.Request, tx sip.ServerTransaction, status int) {
	_ = tx.Respond(sip.NewResponseFromRequest(req, status, sipResponseReason(status), nil))
}

func sipResponseReason(status int) string {
	switch status {
	case 480:
		return "Temporarily Unavailable"
	case 484:
		return "Address Incomplete"
	case 486:
		return "Busy Here"
	case 488:
		return "Not Acceptable Here"
	default:
		return http.StatusText(status)
	}
}

func sipSourceIP(req *sip.Request) netip.Addr {
	host, _, _ := net.SplitHostPort(req.Source())
	addr, _ := netip.ParseAddr(host)
	return addr.Unmap()
}

func outboundWhatsAppNumber(user string) (id, number string, ok bool) {
	number, ok = strings.CutPrefix(user, "wa-")
	if !ok {
		return "", "", false
	}
	if split := strings.LastIndexByte(number, '-'); split >= 0 {
		id, number = number[:split], number[split+1:]
		if id == "" {
			return "", "", false
		}
	}
	return id, "+" + number, len(number) >= 7 && len(number) <= 15 && number[0] != '0' && strings.Trim(number, "0123456789") == ""
}

func (g *sipGateway) outboundFor(source netip.Addr, id string) (*sipOutboundSession, int) {
	if id != "" {
		value, ok := g.outbound.Load(id)
		if ok && slices.Contains(value.(*sipOutboundSession).peers, source) {
			return value.(*sipOutboundSession), 0
		}
		return nil, 403
	}
	var session *sipOutboundSession
	ambiguous := false
	g.outbound.Range(func(_, value any) bool {
		candidate := value.(*sipOutboundSession)
		if !slices.Contains(candidate.peers, source) {
			return true
		}
		ambiguous = session != nil
		session = candidate
		return !ambiguous
	})
	if ambiguous {
		return nil, 484
	}
	if session == nil {
		return nil, 403
	}
	return session, 0
}

func (g *sipGateway) dialWhatsApp(req *sip.Request, tx sip.ServerTransaction, dialogs *sipgo.DialogServerCache) {
	if to := req.To(); to != nil && to.Params.Has("tag") {
		respondSIP(req, tx, 488)
		return
	}
	id, number, ok := outboundWhatsAppNumber(req.Recipient.User)
	if !ok {
		respondSIP(req, tx, 404)
		return
	}
	session, status := g.outboundFor(sipSourceIP(req), id)
	if session == nil {
		log.Warn().Int("sip_status", status).Msg("SIP outbound route unavailable")
		respondSIP(req, tx, status)
		return
	}
	peer, err := mediaAddress(req.Body())
	if err != nil {
		respondSIP(req, tx, 488)
		return
	}
	release, status := session.reserve()
	if release == nil {
		log.Warn().Int("sip_status", status).Msg("SIP outbound account unavailable or at capacity")
		respondSIP(req, tx, status)
		return
	}
	defer release()

	dialog, err := dialogs.ReadInvite(req, tx)
	if err != nil {
		respondSIP(req, tx, 400)
		return
	}
	defer dialog.Close()
	if err := dialog.Respond(100, "Trying", nil); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(session.ctx)
	defer cancel()
	defer context.AfterFunc(dialog.Context(), cancel)()
	dialCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: g.ip})
	if err != nil {
		log.Warn().Err(err).Msg("SIP outbound RTP socket failed")
		respondSIP(req, tx, 503)
		return
	}
	defer conn.Close()
	call, err := session.calls.Call(dialCtx, number)
	if err != nil {
		log.Warn().Err(err).Msg("SIP outbound WhatsApp setup failed")
		if ctx.Err() == nil {
			respondSIP(req, tx, 503)
		}
		return
	}
	defer call.Hangup()
	ended := make(chan struct{})
	call.OnEnd(func(string) { close(ended) })
	if call.State() == meowcaller.CallPhaseEnded {
		respondSIP(req, tx, 480)
		return
	}
	accepted := make(chan struct{})
	call.OnPeerAccept(func() { close(accepted) })
	startSIPAudio(ctx, call, conn, peer)
	if err := dialog.Respond(180, "Ringing", nil); err != nil {
		return
	}

	select {
	case <-accepted:
	case <-ended:
		respondSIP(req, tx, 480)
		return
	case <-dialCtx.Done():
		if ctx.Err() == nil {
			respondSIP(req, tx, 408)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	if err := dialog.RespondSDP([]byte(sipAudioSDP(g.ip, conn.LocalAddr().(*net.UDPAddr).Port))); err != nil {
		log.Warn().Err(err).Msg("SIP outbound answer failed")
		return
	}
	select {
	case <-ctx.Done():
	case <-ended:
	}
	if dialog.Context().Err() == nil {
		bye, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = dialog.Bye(bye)
	}
}
