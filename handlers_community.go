package main

// Communities, channel (newsletter) management/metrics and group metrics, for
// features whatsmeow already supports (parity with WAHA, Evolution Go and
// WPPConnect).

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const newsletterTOSNotice, newsletterTOSStage = "20601218", "5"

func (s *server) respondValue(w http.ResponseWriter, r *http.Request, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		s.Respond(w, r, http.StatusInternalServerError, err)
		return
	}
	s.Respond(w, r, http.StatusOK, string(b))
}

// sessionClient returns the user's connected client or writes the error.
func (s *server) sessionClient(w http.ResponseWriter, r *http.Request) *whatsmeow.Client {
	client := clientManager.GetWhatsmeowClient(r.Context().Value("userinfo").(Values).Get("Id"))
	if client == nil {
		s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
	}
	return client
}

func decodeBody(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errors.New("could not decode Payload")
	}
	return nil
}

func parseServerJID(value, server, field string) (types.JID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return types.EmptyJID, fmt.Errorf("missing %s", field)
	}
	if !strings.Contains(value, "@") {
		value += "@" + server
	}
	jid, err := types.ParseJID(value)
	if err != nil || jid.User == "" || jid.Server != server {
		return types.EmptyJID, fmt.Errorf("invalid %s: expected a %s JID", field, server)
	}
	return jid, nil
}

// ---------------------------------------------------------------- communities

func (s *server) communityLink(link bool) http.HandlerFunc {
	type request struct{ CommunityJID, GroupJID string }
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		var t request
		if err := decodeBody(r, &t); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		parent, err := parseServerJID(t.CommunityJID, types.GroupServer, "CommunityJID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		child, err := parseServerJID(t.GroupJID, types.GroupServer, "GroupJID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		action := "linked"
		if link {
			err = client.LinkGroup(r.Context(), parent, child)
		} else {
			action = "unlinked"
			err = client.UnlinkGroup(r.Context(), parent, child)
		}
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to update community: %v", err))
			return
		}
		s.respondValue(w, r, map[string]string{"Details": "Group " + action, "CommunityJID": parent.String(), "GroupJID": child.String()})
	}
}

func (s *server) LinkCommunityGroup() http.HandlerFunc   { return s.communityLink(true) }
func (s *server) UnlinkCommunityGroup() http.HandlerFunc { return s.communityLink(false) }

func (s *server) GetCommunitySubGroups() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		parent, err := parseServerJID(r.URL.Query().Get("communityJID"), types.GroupServer, "communityJID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		groups, err := client.GetSubGroups(r.Context(), parent)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get subgroups: %v", err))
			return
		}
		if groups == nil {
			groups = []*types.GroupLinkTarget{}
		}
		s.respondValue(w, r, map[string]interface{}{"CommunityJID": parent.String(), "Groups": groups})
	}
}

func (s *server) GetCommunityParticipants() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		parent, err := parseServerJID(r.URL.Query().Get("communityJID"), types.GroupServer, "communityJID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		participants, err := client.GetLinkedGroupsParticipants(r.Context(), parent)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get community participants: %v", err))
			return
		}
		list := make([]string, len(participants))
		for i, p := range participants {
			list[i] = p.String()
		}
		s.respondValue(w, r, map[string]interface{}{"CommunityJID": parent.String(), "Count": len(list), "Participants": list})
	}
}

// filterGroups splits joined groups by kind: community | announcement | group | all.
func filterGroups(groups []*types.GroupInfo, kind string) []*types.GroupInfo {
	out := []*types.GroupInfo{}
	for _, g := range groups {
		isCommunity := g.IsParent
		isAnnouncement := g.IsDefaultSubGroup
		switch kind {
		case "community":
			if !isCommunity {
				continue
			}
		case "announcement":
			if !isAnnouncement {
				continue
			}
		case "group":
			if isCommunity || isAnnouncement {
				continue
			}
		}
		out = append(out, g)
	}
	return out
}

func (s *server) ListCommunities() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		kind := r.URL.Query().Get("type")
		if kind == "" {
			kind = "community"
		}
		if kind != "community" && kind != "announcement" && kind != "group" && kind != "all" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("type must be community, announcement, group or all"))
			return
		}
		groups, err := client.GetJoinedGroups(r.Context())
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to list groups: %v", err))
			return
		}
		s.respondValue(w, r, map[string]interface{}{"Type": kind, "Groups": filterGroups(groups, kind)})
	}
}

// ----------------------------------------------------------------- newsletter

func decodePicture(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if i := strings.Index(value, ";base64,"); strings.HasPrefix(value, "data:") && i > 0 {
		value = value[i+len(";base64,"):]
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("Picture must be base64 or a data URL")
	}
	return data, nil
}

func (s *server) CreateNewsletter() http.HandlerFunc {
	type request struct{ Name, Description, Picture string }
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		var t request
		if err := decodeBody(r, &t); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(t.Name) == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Name"))
			return
		}
		picture, err := decodePicture(t.Picture)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		params := whatsmeow.CreateNewsletterParams{Name: strings.TrimSpace(t.Name), Description: t.Description, Picture: picture}
		meta, err := client.CreateNewsletter(r.Context(), params)
		if err != nil {
			// First channel on an account: accept the channel terms once and retry.
			if tosErr := client.AcceptTOSNotice(r.Context(), newsletterTOSNotice, newsletterTOSStage); tosErr == nil {
				meta, err = client.CreateNewsletter(r.Context(), params)
			}
		}
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to create channel: %v", err))
			return
		}
		s.respondValue(w, r, meta)
	}
}

// newsletterInviteKey accepts a code or a full https://whatsapp.com/channel/<code> link.
func newsletterInviteKey(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.Index(value, "/channel/"); i >= 0 {
		value = value[i+len("/channel/"):]
	}
	if i := strings.IndexAny(value, "/?#"); i >= 0 {
		value = value[:i]
	}
	return value
}

func (s *server) GetNewsletterInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		q := r.URL.Query()
		var meta *types.NewsletterMetadata
		var err error
		switch {
		case q.Get("jid") != "":
			jid, perr := parseServerJID(q.Get("jid"), types.NewsletterServer, "jid")
			if perr != nil {
				s.Respond(w, r, http.StatusBadRequest, perr)
				return
			}
			meta, err = client.GetNewsletterInfo(r.Context(), jid)
		case q.Get("code") != "":
			meta, err = client.GetNewsletterInfoWithInvite(r.Context(), newsletterInviteKey(q.Get("code")))
		default:
			s.Respond(w, r, http.StatusBadRequest, errors.New("provide jid or code (invite code or link)"))
			return
		}
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get channel info: %v", err))
			return
		}
		s.respondValue(w, r, meta)
	}
}

type newsletterMessageView struct {
	ServerID       types.MessageServerID `json:"ServerID"`
	MessageID      string                `json:"MessageID"`
	Type           string                `json:"Type"`
	Timestamp      int64                 `json:"Timestamp"`
	Views          int                   `json:"Views"`
	Reactions      map[string]int        `json:"Reactions"`
	ReactionsTotal int                   `json:"ReactionsTotal"`
	Message        interface{}           `json:"Message,omitempty"`
}

func newsletterViews(msgs []*types.NewsletterMessage) ([]newsletterMessageView, map[string]int) {
	out := make([]newsletterMessageView, 0, len(msgs))
	totals := map[string]int{"Posts": 0, "Views": 0, "Reactions": 0}
	for _, m := range msgs {
		if m == nil {
			continue
		}
		v := newsletterMessageView{ServerID: m.MessageServerID, MessageID: string(m.MessageID), Type: m.Type, Views: m.ViewsCount, Reactions: m.ReactionCounts}
		if !m.Timestamp.IsZero() {
			v.Timestamp = m.Timestamp.Unix()
		}
		if v.Reactions == nil {
			v.Reactions = map[string]int{}
		}
		for _, n := range v.Reactions {
			v.ReactionsTotal += n
		}
		if m.Message != nil {
			v.Message = m.Message
		}
		totals["Posts"]++
		totals["Views"] += v.Views
		totals["Reactions"] += v.ReactionsTotal
		out = append(out, v)
	}
	return out, totals
}

func queryInt(r *http.Request, name string, def, max int) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return def, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > max {
		return 0, fmt.Errorf("%s must be between 0 and %d", name, max)
	}
	return n, nil
}

// GetNewsletterMessages returns posts with view and reaction counts.
func (s *server) GetNewsletterMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		jid, err := parseServerJID(r.URL.Query().Get("jid"), types.NewsletterServer, "jid")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		count, err := queryInt(r, "count", 20, 100)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		before, err := queryInt(r, "before", 0, 1<<30)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		msgs, err := client.GetNewsletterMessages(r.Context(), jid, &whatsmeow.GetNewsletterMessagesParams{Count: count, Before: types.MessageServerID(before)})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get channel messages: %v", err))
			return
		}
		views, totals := newsletterViews(msgs)
		s.respondValue(w, r, map[string]interface{}{"JID": jid.String(), "Messages": views, "Totals": totals})
	}
}

// GetNewsletterUpdates returns only the counters (views/reactions) that changed.
func (s *server) GetNewsletterUpdates() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		jid, err := parseServerJID(r.URL.Query().Get("jid"), types.NewsletterServer, "jid")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		count, err := queryInt(r, "count", 50, 200)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		after, err := queryInt(r, "after", 0, 1<<30)
		if err != nil || after == 0 {
			// WhatsApp does not answer update queries without "after" (or with "since").
			s.Respond(w, r, http.StatusBadRequest, errors.New("after (a post ServerID from /newsletter/messages) is required"))
			return
		}
		params := &whatsmeow.GetNewsletterUpdatesParams{Count: count, After: types.MessageServerID(after)}
		msgs, err := client.GetNewsletterMessageUpdates(r.Context(), jid, params)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get channel updates: %v", err))
			return
		}
		views, totals := newsletterViews(msgs)
		s.respondValue(w, r, map[string]interface{}{"JID": jid.String(), "Updates": views, "Totals": totals})
	}
}

// SubscribeNewsletterLiveUpdates makes WhatsApp push NewsletterLiveUpdate
// events (views/reactions) for a while; the response says for how long.
func (s *server) SubscribeNewsletterLiveUpdates() http.HandlerFunc {
	type request struct{ JID string }
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		var t request
		if err := decodeBody(r, &t); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		jid, err := parseServerJID(t.JID, types.NewsletterServer, "JID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		d, err := client.NewsletterSubscribeLiveUpdates(r.Context(), jid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to subscribe: %v", err))
			return
		}
		s.respondValue(w, r, map[string]interface{}{"JID": jid.String(), "DurationSeconds": int(d.Seconds())})
	}
}

func (s *server) NewsletterReact() http.HandlerFunc {
	type request struct {
		JID       string
		ServerID  int
		Reaction  string
		MessageID string
	}
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		var t request
		if err := decodeBody(r, &t); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		jid, err := parseServerJID(t.JID, types.NewsletterServer, "JID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if t.ServerID <= 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing ServerID (from /newsletter/messages)"))
			return
		}
		msgID := t.MessageID
		if msgID == "" {
			msgID = client.GenerateMessageID()
		}
		// An empty Reaction removes a previous reaction.
		if err := client.NewsletterSendReaction(r.Context(), jid, types.MessageServerID(t.ServerID), t.Reaction, msgID); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to react: %v", err))
			return
		}
		s.respondValue(w, r, map[string]interface{}{"Details": "Reaction sent", "JID": jid.String(), "ServerID": t.ServerID, "Id": msgID})
	}
}

func (s *server) NewsletterMarkViewed() http.HandlerFunc {
	type request struct {
		JID       string
		ServerIDs []int
	}
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		var t request
		if err := decodeBody(r, &t); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		jid, err := parseServerJID(t.JID, types.NewsletterServer, "JID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if len(t.ServerIDs) == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing ServerIDs"))
			return
		}
		ids := make([]types.MessageServerID, len(t.ServerIDs))
		for i, id := range t.ServerIDs {
			ids[i] = types.MessageServerID(id)
		}
		if err := client.NewsletterMarkViewed(r.Context(), jid, ids); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to mark viewed: %v", err))
			return
		}
		s.respondValue(w, r, map[string]interface{}{"Details": "Marked as viewed", "JID": jid.String(), "Count": len(ids)})
	}
}

// --------------------------------------------------------------- group metrics

type groupMember struct {
	JID          string `json:"JID"`
	PhoneNumber  string `json:"PhoneNumber"`
	LID          string `json:"LID"`
	DisplayName  string `json:"DisplayName"`
	IsAdmin      bool   `json:"IsAdmin"`
	IsSuperAdmin bool   `json:"IsSuperAdmin"`
}

func groupMembers(info *types.GroupInfo) []groupMember {
	out := make([]groupMember, 0, len(info.Participants))
	str := func(j types.JID) string {
		if j.IsEmpty() {
			return ""
		}
		return j.String()
	}
	for _, p := range info.Participants {
		out = append(out, groupMember{JID: str(p.JID), PhoneNumber: str(p.PhoneNumber), LID: str(p.LID), DisplayName: p.DisplayName, IsAdmin: p.IsAdmin || p.IsSuperAdmin, IsSuperAdmin: p.IsSuperAdmin})
	}
	return out
}

func membersCSV(members []groupMember) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"jid", "phone_number", "lid", "display_name", "is_admin", "is_super_admin"})
	for _, m := range members {
		_ = w.Write([]string{m.JID, m.PhoneNumber, m.LID, m.DisplayName, strconv.FormatBool(m.IsAdmin), strconv.FormatBool(m.IsSuperAdmin)})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func (s *server) ExportGroupMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		group, err := parseServerJID(r.URL.Query().Get("groupJID"), types.GroupServer, "groupJID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		info, err := client.GetGroupInfo(r.Context(), group)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get group info: %v", err))
			return
		}
		members := groupMembers(info)
		if r.URL.Query().Get("format") == "csv" {
			data, err := membersCSV(members)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
				return
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", group.User+"-members.csv"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		s.respondValue(w, r, map[string]interface{}{"GroupJID": group.String(), "Name": info.Name, "Count": len(members), "Members": members})
	}
}

type historyRow struct {
	SenderJID   string    `db:"sender_jid"`
	Timestamp   time.Time `db:"timestamp"`
	MessageType string    `db:"message_type"`
}

type groupStats struct {
	GroupJID      string         `json:"GroupJID"`
	Name          string         `json:"Name,omitempty"`
	MemberCount   int            `json:"MemberCount"`
	Since         int64          `json:"Since"`
	Messages      int            `json:"Messages"`
	ActiveSenders int            `json:"ActiveSenders"`
	SilentMembers int            `json:"SilentMembers"`
	BySender      []senderStat   `json:"BySender"`
	ByDay         []dayStat      `json:"ByDay"`
	ByType        map[string]int `json:"ByType"`
	Note          string         `json:"Note"`
}

type senderStat struct {
	Sender   string `json:"Sender"`
	Messages int    `json:"Messages"`
	LastAt   int64  `json:"LastAt"`
}

type dayStat struct {
	Day      string `json:"Day"`
	Messages int    `json:"Messages"`
}

func computeGroupStats(rows []historyRow, members []groupMember, loc *time.Location) groupStats {
	st := groupStats{BySender: []senderStat{}, ByDay: []dayStat{}, ByType: map[string]int{}}
	senders := map[string]*senderStat{}
	days := map[string]int{}
	for _, row := range rows {
		if row.MessageType == "reaction" || row.MessageType == "delete" || row.MessageType == "edit" {
			continue
		}
		st.Messages++
		st.ByType[row.MessageType]++
		sender := row.SenderJID
		if i := strings.Index(sender, ":"); i >= 0 && strings.Contains(sender, "@") { // drop device suffix
			sender = sender[:i] + sender[strings.Index(sender, "@"):]
		}
		ss := senders[sender]
		if ss == nil {
			ss = &senderStat{Sender: sender}
			senders[sender] = ss
		}
		ss.Messages++
		if t := row.Timestamp.Unix(); t > ss.LastAt {
			ss.LastAt = t
		}
		days[row.Timestamp.In(loc).Format("2006-01-02")]++
	}
	for _, ss := range senders {
		st.BySender = append(st.BySender, *ss)
	}
	sort.Slice(st.BySender, func(i, j int) bool {
		if st.BySender[i].Messages != st.BySender[j].Messages {
			return st.BySender[i].Messages > st.BySender[j].Messages
		}
		return st.BySender[i].Sender < st.BySender[j].Sender
	})
	for d, n := range days {
		st.ByDay = append(st.ByDay, dayStat{Day: d, Messages: n})
	}
	sort.Slice(st.ByDay, func(i, j int) bool { return st.ByDay[i].Day < st.ByDay[j].Day })
	st.ActiveSenders = len(senders)
	st.MemberCount = len(members)
	for _, m := range members {
		if senders[m.JID] == nil && senders[m.PhoneNumber] == nil && senders[m.LID] == nil {
			st.SilentMembers++
		}
	}
	return st
}

// GroupStats computes activity metrics from this instance's message history.
func (s *server) GroupStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := s.sessionClient(w, r)
		if client == nil {
			return
		}
		group, err := parseServerJID(r.URL.Query().Get("groupJID"), types.GroupServer, "groupJID")
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		days, err := queryInt(r, "days", 30, 365)
		if err != nil || days == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("days must be between 1 and 365"))
			return
		}
		since := time.Now().AddDate(0, 0, -days)
		var members []groupMember
		name := ""
		if info, err := client.GetGroupInfo(r.Context(), group); err == nil {
			members, name = groupMembers(info), info.Name
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		var rows []historyRow
		userID := r.Context().Value("userinfo").(Values).Get("Id")
		query := s.db.Rebind("SELECT sender_jid, timestamp, message_type FROM message_history WHERE user_id = ? AND chat_jid = ? AND timestamp >= ? ORDER BY timestamp LIMIT 200000")
		if err := s.db.SelectContext(ctx, &rows, query, userID, group.String(), since); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("could not read history: %v", err))
			return
		}
		loc, _ := time.LoadLocation("America/Sao_Paulo")
		if loc == nil {
			loc = time.UTC
		}
		st := computeGroupStats(rows, members, loc)
		st.GroupJID, st.Name, st.Since = group.String(), name, since.Unix()
		st.Note = "Computed from this instance's message history (requires history enabled); messages sent while the session was offline or before it was connected are not counted."
		s.respondValue(w, r, st)
	}
}
