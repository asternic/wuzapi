package main

import "testing"

func TestBuildEventMessage(t *testing.T) {
	m, err := buildEventMessage(eventRequest{Name: "Live", Description: "d", StartTime: 1794857400, EndTime: 1794864600, JoinLink: "https://call.whatsapp.com/voice/x", ExtraGuestsAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	ev := m.GetEventMessage()
	if ev.GetName() != "Live" || ev.GetStartTime() != 1794857400 || ev.GetEndTime() != 1794864600 || ev.GetJoinLink() == "" || !ev.GetExtraGuestsAllowed() {
		t.Fatalf("unexpected event: %v", ev)
	}
	if len(m.GetMessageContextInfo().GetMessageSecret()) != 32 {
		t.Fatal("event must carry a 32-byte message secret for RSVPs")
	}
	if ev.IsCanceled == nil || ev.GetIsCanceled() || ev.IsScheduleCall == nil || ev.GetIsScheduleCall() || ev.HasReminder == nil || ev.GetHasReminder() || ev.ReminderOffsetSec == nil || ev.GetReminderOffsetSec() != 0 || ev.ExtraGuestsAllowed == nil {
		t.Fatalf("event defaults must be explicit: %v", ev)
	}
	defaults, err := buildEventMessage(eventRequest{Name: "x", StartTime: 10})
	if err != nil || defaults.GetEventMessage().ExtraGuestsAllowed == nil || defaults.GetEventMessage().GetExtraGuestsAllowed() {
		t.Fatalf("extra guests default must be explicit and false: %v, %v", defaults, err)
	}
	reminder, err := buildEventMessage(eventRequest{Name: "x", StartTime: 10, HasReminder: true, ReminderOffsetSec: 60, IsScheduleCall: true})
	if err != nil || !reminder.GetEventMessage().GetHasReminder() || reminder.GetEventMessage().GetReminderOffsetSec() != 60 || !reminder.GetEventMessage().GetIsScheduleCall() {
		t.Fatalf("reminder and call settings missing: %v, %v", reminder, err)
	}
	for _, link := range []string{"https://example.com", "http://call.whatsapp.com/voice/x", "https://call.whatsapp.com.evil.test/voice/x"} {
		if _, err := buildEventMessage(eventRequest{Name: "x", StartTime: 10, JoinLink: link}); err == nil {
			t.Fatalf("invalid JoinLink accepted: %s", link)
		}
	}
	if _, err := buildEventMessage(eventRequest{Name: "x", StartTime: 10, ReminderOffsetSec: -1}); err == nil {
		t.Fatal("negative ReminderOffsetSec accepted")
	}
	if _, err := buildEventMessage(eventRequest{Name: "x"}); err == nil {
		t.Fatal("missing StartTime accepted")
	}
	if _, err := buildEventMessage(eventRequest{Name: "x", StartTime: 10, EndTime: 5}); err == nil {
		t.Fatal("EndTime before StartTime accepted")
	}
}
