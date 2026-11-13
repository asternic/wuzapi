package main

import (
	"encoding/json"
	"fmt"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// The base is also used for own-device DSM, history and retries.
// Stanza-level biz nodes cannot vary per device; use the tested viewonce/fork biz.
func buildAutoButtonEnvelope(current *waE2E.Message, title string, copyIndexes []int, sender types.JID) (*waE2E.Message, []waBinary.Node, func(types.JID) *waE2E.Message, error) {
	im := proto.Clone(current.GetInteractiveMessage()).(*waE2E.InteractiveMessage)
	isPix := false
	for _, btn := range im.GetNativeFlowMessage().GetButtons() {
		if btn.GetName() == "payment_info" {
			isPix = true
		}
	}
	mode := "viewonce"
	if isPix {
		mode = "fork"
		copyIndexes = nil
	}
	for _, idx := range copyIndexes {
		btn := im.GetNativeFlowMessage().Buttons[idx]
		var params struct {
			DisplayText string `json:"display_text"`
			CopyCode    string `json:"copy_code"`
		}
		if err := json.Unmarshal([]byte(btn.GetButtonParamsJSON()), &params); err != nil {
			return nil, nil, nil, err
		}
		data, err := json.Marshal(params)
		if err != nil {
			return nil, nil, nil, err
		}
		btn.ButtonParamsJSON = proto.String(string(data))
	}
	primary, nodes, err := buildButtonEnvelope(im, mode, title)
	if err != nil {
		return nil, nil, nil, err
	}
	base := proto.Clone(current).(*waE2E.Message)
	// Linked recipients and all own devices (DSM) use the same safe base.
	// Build primary first so its original voice_call remains intact.
	for _, btn := range base.GetInteractiveMessage().GetNativeFlowMessage().GetButtons() {
		if btn.GetName() != "voice_call" {
			continue
		}
		if sender.Server != types.DefaultUserServer || sender.User == "" {
			return nil, nil, nil, fmt.Errorf("voice_call auto requires sender phone JID")
		}
		for _, digit := range sender.User {
			if digit < '0' || digit > '9' {
				return nil, nil, nil, fmt.Errorf("voice_call auto requires numeric sender phone")
			}
		}
		var params struct {
			DisplayText string `json:"display_text"`
		}
		if err := json.Unmarshal([]byte(btn.GetButtonParamsJSON()), &params); err != nil {
			return nil, nil, nil, err
		}
		url := "https://wa.me/" + sender.User
		data, err := json.Marshal(map[string]string{"display_text": params.DisplayText, "url": url, "merchant_url": url})
		if err != nil {
			return nil, nil, nil, err
		}
		btn.Name = proto.String("cta_url")
		btn.ButtonParamsJSON = proto.String(string(data))
	}
	if isPix {
		// PIX wins even when a different button precedes payment_info.
		base, _, err = buildButtonEnvelope(base.GetInteractiveMessage(), "fork", title)
		if err != nil {
			return nil, nil, nil, err
		}
		for i := range nodes {
			if nodes[i].Tag != "biz" {
				continue
			}
			nodes[i].Attrs["native_flow_name"] = "payment_info"
			children, _ := nodes[i].Content.([]waBinary.Node)
			kept := make([]waBinary.Node, 0, len(children))
			for _, child := range children {
				if child.Tag != "interactive" {
					kept = append(kept, child)
				}
			}
			nodes[i].Content = kept
		}
	}
	// One secret identifies the logical message across all copies, reactions and edits.
	// Preserve any existing root context fields; replace only the secret.
	if base.MessageContextInfo == nil {
		base.MessageContextInfo = &waE2E.MessageContextInfo{}
	}
	base.MessageContextInfo.MessageSecret = primary.MessageContextInfo.MessageSecret
	hook := func(jid types.JID) *waE2E.Message {
		if jid.Device == 0 {
			return primary
		}
		return nil // base plaintext; the transport always protects own devices with base DSM
	}
	return base, nodes, hook, nil
}
