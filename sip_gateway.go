package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/purpshell/meowcaller"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type sipGateway struct {
	ua       *sipgo.UserAgent
	listener *net.UDPConn
	dialogs  *sipgo.DialogClientCache
	ip       net.IP
	targets  map[string]sip.Uri
	slots    chan struct{}
}

var inboundSIP *sipGateway

func configureSIPGateway() (*sipGateway, error) {
	local := os.Getenv("WUZAPI_SIP_LOCAL")
	routes := os.Getenv("WUZAPI_SIP_TARGETS")
	if local == "" && routes == "" {
		return nil, nil
	}
	addr, err := net.ResolveUDPAddr("udp4", local)
	if err != nil || addr.IP.To4() == nil || addr.IP.IsUnspecified() || addr.Port == 0 {
		return nil, fmt.Errorf("WUZAPI_SIP_LOCAL must be a specific IPv4 address and port")
	}
	var config map[string]string
	if routes != "" && json.Unmarshal([]byte(routes), &config) != nil {
		return nil, fmt.Errorf("WUZAPI_SIP_TARGETS must map user IDs to SIP URIs")
	}
	targets := make(map[string]sip.Uri, len(config))
	for id, target := range config {
		uri, err := parseSIPTarget(target)
		if err != nil {
			return nil, fmt.Errorf("invalid SIP/UDP target for user %q", id)
		}
		targets[id] = uri
	}
	ua, err := sipgo.NewUA()
	if err != nil {
		return nil, err
	}
	client, err := sipgo.NewClient(ua, sipgo.WithClientAddr(addr.String()), sipgo.WithClientConnectionAddr(addr.String()))
	if err != nil {
		ua.Close()
		return nil, err
	}
	dialogs := sipgo.NewDialogClientCache(client, sip.ContactHeader{Address: sip.Uri{User: "wuzapi", Host: addr.IP.String(), Port: addr.Port}})
	server, err := sipgo.NewServer(ua)
	if err != nil {
		ua.Close()
		return nil, err
	}
	server.OnBye(func(req *sip.Request, tx sip.ServerTransaction) {
		if err := dialogs.ReadBye(req, tx); err != nil {
			_ = tx.Respond(sip.NewResponseFromRequest(req, 481, "Call Does Not Exist", nil))
		}
	})
	listener, err := net.ListenUDP("udp4", addr)
	if err != nil {
		ua.Close()
		return nil, err
	}
	go func() {
		if err := server.ServeUDP(listener); err != nil {
			log.Debug().Err(err).Msg("SIP listener stopped")
		}
	}()
	return &sipGateway{ua: ua, listener: listener, dialogs: dialogs, ip: addr.IP, targets: targets, slots: make(chan struct{}, 32)}, nil
}

func (g *sipGateway) attach(ctx context.Context, s *server, userID string, wa *whatsmeow.Client) func() {
	if g == nil {
		return func() {}
	}
	target, err := s.sipTarget(ctx, userID)
	if err != nil {
		log.Warn().Err(err).Str("user_id", userID).Msg("Cannot load SIP route")
	}
	if err != nil || target.Host == "" {
		return func() {}
	}
	calls := meowcaller.NewClient(wa, meowcaller.WithLogger(log.With().Str("user_id", userID).Str("component", "meowcaller").Logger()))
	var mu sync.Mutex
	var running sync.WaitGroup
	stopping := false
	active := make(map[string]context.CancelFunc)
	calls.OnIncomingCall(func(call *meowcaller.Call) {
		mu.Lock()
		defer mu.Unlock()
		if stopping || ctx.Err() != nil {
			return
		}
		if _, exists := active[call.ID()]; exists {
			return
		}
		_, group := call.GroupState()
		if group || call.IsVideo() || len(active) >= 4 {
			go func() { _ = call.Reject() }()
			return
		}
		select {
		case g.slots <- struct{}{}:
		default:
			go func() { _ = call.Reject() }()
			return
		}
		callCtx, cancel := context.WithCancel(ctx)
		active[call.ID()] = cancel
		call.OnEnd(func(string) { cancel() })
		if call.State() == meowcaller.CallPhaseEnded {
			cancel()
		}
		running.Add(1)
		go func() {
			defer running.Done()
			defer func() {
				cancel()
				mu.Lock()
				delete(active, call.ID())
				mu.Unlock()
				<-g.slots
			}()
			if callCtx.Err() == nil {
				number, name := sipCallerIdentity(callCtx, wa, call.Peer())
				if err := bridgeSIPCall(callCtx, call, g.dialogs, target, g.ip, number, name); err != nil {
					log.Warn().Str("user_id", userID).Err(err).Msg("SIP call ended")
				}
			}
		}()
	})
	return func() {
		mu.Lock()
		stopping = true
		for _, cancel := range active {
			cancel()
		}
		mu.Unlock()
		done := make(chan struct{})
		go func() { running.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
}

func sipCallerIdentity(ctx context.Context, wa *whatsmeow.Client, peer types.JID) (number, name string) {
	lookup, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	peer = peer.ToNonAD()
	pn := peer
	if peer.Server == types.HiddenUserServer {
		pn, _ = wa.Store.LIDs.GetPNForLID(lookup, peer)
	}
	if pn.Server == types.DefaultUserServer && strings.Trim(pn.User, "0123456789") == "" {
		number = pn.User
	}

	name = sipContactPushName(lookup, wa, peer)
	if name == "" && number != "" && pn != peer {
		name = sipContactPushName(lookup, wa, pn)
	}
	return number, name
}

func sipContactPushName(ctx context.Context, wa *whatsmeow.Client, jid types.JID) string {
	contact, err := wa.Store.Contacts.GetContact(ctx, jid)
	if err != nil {
		return ""
	}
	// sipgo writes display names verbatim inside a quoted string.
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' || r == '\\' {
			return ' '
		}
		return r
	}, contact.PushName))
}

func parseSIPTarget(value string) (sip.Uri, error) {
	var uri sip.Uri
	if strings.ContainsAny(value, " \t\r\n") || sip.ParseUri(value, &uri) != nil ||
		uri.Scheme != "sip" || uri.Host == "" || uri.User == "" || uri.Password != "" ||
		uri.UriParams.Has("transport") || uri.Headers.Length() != 0 || uri.Port < 0 || uri.Port > 65535 {
		return sip.Uri{}, fmt.Errorf("target must be a SIP URI such as sip:wa-in@pbx.example.com:5060")
	}
	return uri, nil
}

func (s *server) sipTarget(ctx context.Context, id string) (sip.Uri, error) {
	var target sql.NullString
	if err := s.db.GetContext(ctx, &target, "SELECT sip_gateway_target FROM users WHERE id = $1", id); err != nil {
		return sip.Uri{}, err
	}
	if target.Valid {
		if target.String == "" {
			return sip.Uri{}, nil
		}
		return parseSIPTarget(target.String)
	}
	if inboundSIP != nil {
		return inboundSIP.targets[id], nil
	}
	return sip.Uri{}, nil
}
