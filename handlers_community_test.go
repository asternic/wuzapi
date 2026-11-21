package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestCommunityRoutesRequireSession(t *testing.T) {
	s := makeTestServer(t)
	seedClientHistory(t, s, "community-user", "community-token", 100)
	routes := []struct{ method, path string }{
		{"GET", "/community/list"}, {"POST", "/community/link"},
		{"POST", "/community/unlink"}, {"GET", "/community/subgroups?communityJID=1"},
		{"GET", "/community/participants?communityJID=1"}, {"GET", "/group/members/export?groupJID=1"},
		{"GET", "/group/stats?groupJID=1"}, {"POST", "/newsletter/create"}, {"GET", "/newsletter/info?jid=1"},
		{"GET", "/newsletter/messages?jid=1"}, {"GET", "/newsletter/updates?jid=1"},
		{"POST", "/newsletter/subscribe"}, {"POST", "/newsletter/react"}, {"POST", "/newsletter/markviewed"},
	}
	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
		resp := httptest.NewRecorder()
		s.router.ServeHTTP(resp, req)
		if resp.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: %d", rt.method, rt.path, resp.Code)
		}
		req = httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
		req.Header.Set("token", "community-token")
		resp = httptest.NewRecorder()
		s.router.ServeHTTP(resp, req)
		if resp.Code != http.StatusInternalServerError || !strings.Contains(resp.Body.String(), "no session") {
			t.Errorf("%s %s: route not wired (%d %s)", rt.method, rt.path, resp.Code, resp.Body.String())
		}
	}
}

func TestStdioQueryForNewGetMethods(t *testing.T) {
	got := stdioQuery(map[string]interface{}{"jid": "123@newsletter", "count": float64(10), "other": "x"}, "jid", "count", "before")
	if got != "?count=10&jid=123%40newsletter" {
		t.Fatalf("query = %q", got)
	}
	if stdioQuery(map[string]interface{}{}, "jid") != "" {
		t.Fatal("empty params must give empty query")
	}
}

func TestParseServerJID(t *testing.T) {
	if j, err := parseServerJID("120363000000000000", types.GroupServer, "x"); err != nil || j.String() != "120363000000000000@g.us" {
		t.Fatalf("bare group id: %v %v", j, err)
	}
	if _, err := parseServerJID("123@s.whatsapp.net", types.GroupServer, "x"); err == nil {
		t.Fatal("user JID accepted as group")
	}
	if _, err := parseServerJID("", types.NewsletterServer, "jid"); err == nil {
		t.Fatal("empty accepted")
	}
	if newsletterInviteKey("https://whatsapp.com/channel/0029VaABC123?ref=x") != "0029VaABC123" || newsletterInviteKey("0029VaABC123") != "0029VaABC123" {
		t.Fatal("invite key parsing")
	}
	if b, err := decodePicture("data:image/png;base64,aGk="); err != nil || string(b) != "hi" {
		t.Fatalf("picture: %q %v", b, err)
	}
}

func TestFilterGroups(t *testing.T) {
	community := &types.GroupInfo{JID: types.NewJID("1", types.GroupServer)}
	community.IsParent = true
	announce := &types.GroupInfo{JID: types.NewJID("2", types.GroupServer)}
	announce.IsDefaultSubGroup = true
	plain := &types.GroupInfo{JID: types.NewJID("3", types.GroupServer)}
	all := []*types.GroupInfo{community, announce, plain}
	if g := filterGroups(all, "community"); len(g) != 1 || g[0] != community {
		t.Fatal("community filter")
	}
	if g := filterGroups(all, "announcement"); len(g) != 1 || g[0] != announce {
		t.Fatal("announcement filter")
	}
	if g := filterGroups(all, "group"); len(g) != 1 || g[0] != plain {
		t.Fatal("group filter")
	}
	if len(filterGroups(all, "all")) != 3 {
		t.Fatal("all filter")
	}
}

func TestNewsletterViewsTotals(t *testing.T) {
	msgs := []*types.NewsletterMessage{
		{MessageServerID: 10, ViewsCount: 100, ReactionCounts: map[string]int{"👍": 3, "❤️": 2}, Timestamp: time.Unix(1700000000, 0)},
		{MessageServerID: 11, ViewsCount: 50},
		nil,
	}
	views, totals := newsletterViews(msgs)
	if len(views) != 2 || views[0].ReactionsTotal != 5 || views[1].Reactions == nil || views[0].Timestamp != 1700000000 {
		t.Fatalf("views: %+v", views)
	}
	if totals["Posts"] != 2 || totals["Views"] != 150 || totals["Reactions"] != 5 {
		t.Fatalf("totals: %v", totals)
	}
}

func TestGroupMetrics(t *testing.T) {
	info := &types.GroupInfo{Participants: []types.GroupParticipant{
		{JID: types.NewJID("111", types.HiddenUserServer), PhoneNumber: types.NewJID("5511900000001", types.DefaultUserServer), IsAdmin: true},
		{JID: types.NewJID("222", types.HiddenUserServer)},
		{JID: types.NewJID("333", types.HiddenUserServer), IsSuperAdmin: true},
	}}
	members := groupMembers(info)
	if len(members) != 3 || !members[2].IsAdmin || !members[2].IsSuperAdmin || members[0].PhoneNumber == "" {
		t.Fatalf("members: %+v", members)
	}
	csvData, err := membersCSV(members)
	if err != nil || !strings.HasPrefix(string(csvData), "jid,phone_number,lid,display_name,is_admin,is_super_admin\n111@lid,5511900000001@s.whatsapp.net,") {
		t.Fatalf("csv: %s %v", csvData, err)
	}
	day1 := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	rows := []historyRow{
		{SenderJID: "111:3@lid", Timestamp: day1, MessageType: "text"},
		{SenderJID: "111@lid", Timestamp: day1.Add(time.Hour), MessageType: "image"},
		{SenderJID: "5511900000002@s.whatsapp.net", Timestamp: day1.Add(24 * time.Hour), MessageType: "text"},
		{SenderJID: "111@lid", Timestamp: day1, MessageType: "reaction"},
	}
	st := computeGroupStats(rows, members, time.UTC)
	if st.Messages != 3 || st.ActiveSenders != 2 || st.BySender[0].Sender != "111@lid" || st.BySender[0].Messages != 2 {
		t.Fatalf("stats: %+v", st)
	}
	if len(st.ByDay) != 2 || st.ByDay[0].Day != "2026-11-20" || st.ByType["image"] != 1 || st.SilentMembers != 2 {
		t.Fatalf("stats detail: %+v", st)
	}
}
