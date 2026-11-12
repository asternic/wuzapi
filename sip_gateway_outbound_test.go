package main

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

func TestOutboundPBXIPv4Representations(t *testing.T) {
	resolved, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip4", "172.16.1.242")
	if err != nil {
		t.Fatal(err)
	}
	for _, peers := range [][]netip.Addr{
		resolved,
		{netip.MustParseAddr("::ffff:172.16.1.242")},
		{netip.MustParseAddr("172.16.1.242")},
	} {
		g := &sipGateway{}
		session := &sipOutboundSession{peers: peers}
		g.outbound.Store("account", session)
		for _, id := range []string{"", "account"} {
			for _, source := range []string{"172.16.1.242", "::ffff:172.16.1.242"} {
				got, status := g.outboundFor(netip.MustParseAddr(source), id)
				if got != session || status != 0 {
					t.Fatalf("peers=%v source=%s id=%q: status=%d", peers, source, id, status)
				}
			}
			got, status := g.outboundFor(netip.MustParseAddr("172.16.1.243"), id)
			if got != nil || status != 403 {
				t.Fatal("different PBX source was authorized")
			}
			got, status = g.outboundFor(netip.Addr{}, id)
			if got != nil || status != 403 {
				t.Fatal("invalid PBX source was authorized")
			}
		}
	}
}

func TestOutboundPBXAmbiguousAccounts(t *testing.T) {
	g := &sipGateway{}
	g.outbound.Store("one", &sipOutboundSession{peers: []netip.Addr{netip.MustParseAddr("172.16.1.242")}})
	g.outbound.Store("two", &sipOutboundSession{peers: []netip.Addr{netip.MustParseAddr("::ffff:172.16.1.242")}})
	got, status := g.outboundFor(netip.MustParseAddr("172.16.1.242"), "")
	if got != nil || status != 484 {
		t.Fatal("multiple matching accounts must require an explicit account")
	}
}
