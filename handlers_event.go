package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// An ordinary WhatsApp event (the card with title, description, date, location,
// call link and "going / not going"). The message carries a MessageSecret so the
// RSVPs (secret-encrypted EventResponseMessage) can be decrypted on arrival; the
// webhook already swaps decrypted secret messages in (wmiau.go).
type eventRequest struct {
	Phone              string `json:"Phone"`
	Name               string `json:"Name"`
	Description        string `json:"Description"`
	StartTime          int64  `json:"StartTime"`
	EndTime            int64  `json:"EndTime"`
	JoinLink           string `json:"JoinLink"`
	ExtraGuestsAllowed bool   `json:"ExtraGuestsAllowed"`
	IsScheduleCall     bool   `json:"IsScheduleCall"`
	HasReminder        bool   `json:"HasReminder"`
	ReminderOffsetSec  int64  `json:"ReminderOffsetSec"`
	Location           *struct {
		Name      string  `json:"Name"`
		Address   string  `json:"Address"`
		Latitude  float64 `json:"Latitude"`
		Longitude float64 `json:"Longitude"`
	} `json:"Location"`
	Id string `json:"Id"`
}

func buildEventMessage(req eventRequest) (*waE2E.Message, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("Name is required")
	}
	if req.StartTime <= 0 {
		return nil, errors.New("StartTime (unix seconds) is required")
	}
	if req.EndTime != 0 && req.EndTime < req.StartTime {
		return nil, errors.New("EndTime must be after StartTime")
	}
	if req.ReminderOffsetSec < 0 {
		return nil, errors.New("ReminderOffsetSec must be non-negative")
	}
	if req.JoinLink != "" {
		link, err := url.Parse(req.JoinLink)
		if err != nil || link.Scheme != "https" || link.Host != "call.whatsapp.com" || link.User != nil {
			return nil, errors.New("JoinLink must be a WhatsApp call link (https://call.whatsapp.com/...); put other links in Description")
		}
	}
	ev := &waE2E.EventMessage{
		Name:               proto.String(req.Name),
		StartTime:          proto.Int64(req.StartTime),
		ExtraGuestsAllowed: proto.Bool(req.ExtraGuestsAllowed),
		IsCanceled:         proto.Bool(false),
		IsScheduleCall:     proto.Bool(req.IsScheduleCall),
		HasReminder:        proto.Bool(req.HasReminder),
		ReminderOffsetSec:  proto.Int64(req.ReminderOffsetSec),
	}
	if req.Description != "" {
		ev.Description = proto.String(req.Description)
	}
	if req.EndTime > 0 {
		ev.EndTime = proto.Int64(req.EndTime)
	}
	if req.JoinLink != "" {
		ev.JoinLink = proto.String(req.JoinLink)
	}
	if req.Location != nil && (req.Location.Name != "" || req.Location.Address != "" || req.Location.Latitude != 0 || req.Location.Longitude != 0) {
		loc := &waE2E.LocationMessage{}
		if req.Location.Name != "" {
			loc.Name = proto.String(req.Location.Name)
		}
		if req.Location.Address != "" {
			loc.Address = proto.String(req.Location.Address)
		}
		if req.Location.Latitude != 0 || req.Location.Longitude != 0 {
			loc.DegreesLatitude = proto.Float64(req.Location.Latitude)
			loc.DegreesLongitude = proto.Float64(req.Location.Longitude)
		}
		ev.Location = loc
	}
	return &waE2E.Message{
		EventMessage:       ev,
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: random.Bytes(32)},
	}, nil
}

func (s *server) SendEvent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		cli := clientManager.GetWhatsmeowClient(txtid)
		if cli == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}
		var req eventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}
		msg, err := buildEventMessage(req)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		recipient, err := validateMessageFields(r.Context(), cli, req.Phone, nil, nil)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		msgid := req.Id
		if msgid == "" {
			msgid = cli.GenerateMessageID()
		}
		resp, err := sendMessageWithRecovery(txtid, cli, context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to send event: %v", err))
			return
		}
		data, _ := json.Marshal(map[string]interface{}{"Details": "Event sent", "Id": msgid, "Timestamp": resp.Timestamp})
		s.Respond(w, r, http.StatusOK, string(data))
	}
}
