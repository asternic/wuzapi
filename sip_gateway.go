package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
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
	outbound sync.Map // WuzAPI user ID to *sipOutboundSession
}

var inboundSIP *sipGateway

func configureSIPGateway() (*sipGateway, error) {
	local := os.Getenv("WUZAPI_SIP_LOCAL")
	routes := os.Getenv("WUZAPI_SIP_TARGETS")
	if local == "" && routes == "" {
		return nil, nil
	}
	maxCalls := 32
	if value := os.Getenv("WUZAPI_SIP_MAX_CALLS"); value != "" {
		limit, err := strconv.ParseInt(value, 10, 32)
		if err != nil || limit < 1 {
			return nil, fmt.Errorf("WUZAPI_SIP_MAX_CALLS must be a positive integer")
		}
		maxCalls = int(limit)
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
	g := &sipGateway{ua: ua, dialogs: dialogs, ip: addr.IP, targets: targets, slots: make(chan struct{}, maxCalls)}
	g.registerSIPHandlers(server, client, addr)
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
	g.listener = listener
	return g, nil
}

func (g *sipGateway) attach(ctx context.Context, s *server, userID string, wa *whatsmeow.Client) func() {
	if g == nil {
		return func() {}
	}
	target, maxCalls, allowOutbound, err := s.sipTarget(ctx, userID)
	if err != nil {
		log.Warn().Err(err).Str("user_id", userID).Msg("Cannot load SIP route")
	}
	if err != nil || target.Host == "" {
		return func() {}
	}

	auth, err := s.sipAuth(ctx, userID)
	if err != nil {
		log.Error().Str("user_id", userID).Msg("Cannot decrypt SIP credentials; check the global encryption key")
		return func() {}
	}
	calls := meowcaller.NewClient(wa, meowcaller.WithLogger(log.With().Str("user_id", userID).Str("component", "meowcaller").Logger()))
	var mu sync.Mutex
	var running sync.WaitGroup
	stopping := false
	outgoing := 0
	active := make(map[string]context.CancelFunc)
	if allowOutbound {
		lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
		peers, err := net.DefaultResolver.LookupNetIP(lookup, "ip4", target.Host)
		cancel()
		if err != nil || len(peers) == 0 {
			log.Warn().Err(err).Str("user_id", userID).Msg("Cannot resolve outbound SIP PBX")
		} else {
			g.outbound.Store(userID, &sipOutboundSession{peers: peers, calls: calls, ctx: ctx, reserve: func() (func(), int) {
				mu.Lock()
				defer mu.Unlock()
				if stopping || ctx.Err() != nil || !wa.IsLoggedIn() || !wa.IsConnected() {
					return nil, 503
				}
				if len(active)+outgoing >= maxCalls {
					return nil, 486
				}
				select {
				case g.slots <- struct{}{}:
				default:
					return nil, 486
				}
				outgoing++
				running.Add(1)
				return func() {
					mu.Lock()
					outgoing--
					mu.Unlock()
					<-g.slots
					running.Done()
				}, 0
			}})
			log.Info().Str("user_id", userID).Any("pbx_ips", peers).
				Msg("SIP outbound route attached")
		}
	}
	calls.OnIncomingCall(func(call *meowcaller.Call) {
		mu.Lock()
		defer mu.Unlock()
		if stopping || ctx.Err() != nil || target.Host == "" {
			return
		}
		if _, exists := active[call.ID()]; exists {
			return
		}
		_, group := call.GroupState()
		if group || call.IsVideo() || len(active)+outgoing >= maxCalls {
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
				if err := bridgeSIPCall(callCtx, call, g.dialogs, target, g.ip, number, name, auth); err != nil {
					log.Warn().Str("user_id", userID).Err(err).Msg("SIP call ended")
				}
			}
		}()
	})
	return func() {
		mu.Lock()
		stopping = true
		g.outbound.Delete(userID)
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

const sipAccountCallLimit = 32

func (s *server) sipTarget(ctx context.Context, id string) (sip.Uri, int, bool, error) {
	var target sql.NullString
	var maxCalls int
	var outbound bool
	err := s.db.QueryRowxContext(ctx, "SELECT sip_gateway_target, sip_gateway_max_calls, sip_gateway_outbound FROM users WHERE id = $1", id).Scan(&target, &maxCalls, &outbound)
	if err != nil {
		return sip.Uri{}, 0, false, err
	}
	if target.Valid {
		if target.String == "" {
			return sip.Uri{}, maxCalls, false, nil
		}
		uri, err := parseSIPTarget(target.String)
		return uri, maxCalls, outbound, err
	}
	if inboundSIP != nil {
		return inboundSIP.targets[id], maxCalls, outbound, nil
	}
	return sip.Uri{}, maxCalls, false, nil
}

func (s *server) sipAuth(ctx context.Context, id string) (sipgo.AnswerOptions, error) {
	var auth sipgo.AnswerOptions
	var encoded string
	if err := s.db.QueryRowxContext(ctx, "SELECT sip_gateway_auth_username, sip_gateway_auth_password FROM users WHERE id = $1", id).Scan(&auth.Username, &encoded); err != nil {
		return auth, err
	}
	if encoded != "" {
		ciphertext, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return auth, err
		}
		auth.Password, err = decryptHMACKey(ciphertext)
		if err != nil {
			return auth, err
		}
	}
	return auth, nil
}
