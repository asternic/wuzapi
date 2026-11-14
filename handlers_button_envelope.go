package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// Explicit modes preserve their behavior; the handler resolves the conditional default.
func normalizeButtonEnvelope(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = "original"
	}
	switch value {
	case "original", "fork", "viewonce":
		return value, nil
	default:
		return "", fmt.Errorf("Envelope must be original, fork or viewonce")
	}
}

// Random fields are injected so offline fixtures can compare both encoders.
// Production uses crypto/rand; these bytes are never logged or persisted here.
func buildButtonEnvelope(im *waE2E.InteractiveMessage, mode, title string) (*waE2E.Message, []waBinary.Node, error) {
	secret, decision := make([]byte, 32), make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return nil, nil, err
	}
	if _, err := rand.Read(decision); err != nil {
		return nil, nil, err
	}
	return buildButtonEnvelopeAt(im, mode, title, secret, decision, time.Now().Unix())
}

func buildButtonEnvelopeAt(im *waE2E.InteractiveMessage, mode, title string, secret, decision []byte, unix int64) (*waE2E.Message, []waBinary.Node, error) {
	if mode != "fork" && mode != "viewonce" {
		return nil, nil, fmt.Errorf("unsupported experimental envelope %q", mode)
	}
	if im == nil || im.GetNativeFlowMessage() == nil || len(im.GetNativeFlowMessage().Buttons) == 0 {
		return nil, nil, fmt.Errorf("experimental envelope requires native flow buttons")
	}
	im = proto.Clone(im).(*waE2E.InteractiveMessage)
	nf := im.GetNativeFlowMessage()
	// Text native flows omit the header and message version.
	nf.MessageVersion = nil
	nf.MessageParamsJSON = proto.String("{}")
	if im.Header.GetHasMediaAttachment() {
		im.Header.Title = proto.String(title)
		im.Header.Subtitle = proto.String("")
	} else {
		im.Header = nil
	}
	if im.ContextInfo != nil && proto.Size(im.ContextInfo) == 0 {
		im.ContextInfo = nil
	}

	attrs := waBinary.Attrs{"actual_actors": "2", "host_storage": "2", "privacy_mode_ts": fmt.Sprint(unix)}
	quality := waBinary.Node{
		Tag:     "quality_control",
		Attrs:   waBinary.Attrs{"decision_id": hex.EncodeToString(decision), "source_type": "third_party"},
		Content: []waBinary.Node{{Tag: "decision_source", Attrs: waBinary.Attrs{"value": "df"}}},
	}
	flowName := nf.Buttons[0].GetName()
	var children []waBinary.Node
	if flowName == "payment_info" || flowName == "review_and_pay" {
		// The fork's payment branch has NO <interactive> child.
		if flowName == "review_and_pay" {
			flowName = "order_details"
		}
		attrs["native_flow_name"] = flowName
	} else {
		version, name := "9", "mixed"
		switch flowName {
		case "mpm", "cta_catalog", "send_location", "call_permission_request", "wa_payment_transaction_details", "automated_greeting_message_view_catalog":
			version, name = "2", flowName
		}
		children = append(children, waBinary.Node{
			Tag: "interactive", Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
			Content: []waBinary.Node{{Tag: "native_flow", Attrs: waBinary.Attrs{"v": version, "name": name}}},
		})
	}
	children = append(children, quality)
	msg := &waE2E.Message{InteractiveMessage: im}
	if mode == "viewonce" {
		msg = &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: msg}}
	}
	msg.MessageContextInfo = &waE2E.MessageContextInfo{MessageSecret: secret}
	return msg, []waBinary.Node{{Tag: "biz", Attrs: attrs, Content: children}}, nil
}
