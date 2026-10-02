package main

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"google.golang.org/protobuf/proto"
)

// This is an application validation limit, not a guarantee of WhatsApp availability.
const maxHistorySyncDays = 365

func validateHistorySyncDays(days int) error {
	if days < 0 || days > maxHistorySyncDays {
		return fmt.Errorf("days_to_sync_history must be between 0 and %d", maxHistorySyncDays)
	}
	return nil
}

// configureHistorySyncClient customizes each registration payload independently.
// Never mutate store.DeviceProps: it is shared by every user's WhatsApp client.
func (s *server) configureHistorySyncClient(client *whatsmeow.Client, userID string) {
	client.GetClientPayload = func() *waWa6.ClientPayload {
		payload := client.Store.GetClientPayload()
		if payload.DevicePairingData == nil {
			return payload // Reconnecting an already linked device is not a new pairing.
		}
		var days int
		err := s.db.Get(&days, s.db.Rebind("SELECT COALESCE(days_to_sync_history, 0) FROM users WHERE id = ?"), userID)
		if err == nil {
			err = validateHistorySyncDays(days)
		}
		if err != nil {
			log.Error().Err(err).Str("userID", userID).Msg("Failed to load history sync configuration; using WhatsApp defaults")
			days = 0
		}
		if err := configurePairingHistory(payload, days); err != nil {
			log.Error().Err(err).Str("userID", userID).Msg("Failed to configure pairing history")
		}
		return payload
	}
}

func configurePairingHistory(payload *waWa6.ClientPayload, days int) error {
	if err := validateHistorySyncDays(days); err != nil {
		return err
	}
	if payload.DevicePairingData == nil {
		return nil
	}
	var props waCompanionReg.DeviceProps
	if err := proto.Unmarshal(payload.DevicePairingData.DeviceProps, &props); err != nil {
		return fmt.Errorf("decode pairing device properties: %w", err)
	}
	// Zero preserves the upstream default sync behavior. Positive values request
	// a full sync window in real days, rather than estimating a message count.
	if days > 0 {
		props.RequireFullSync = proto.Bool(true)
		if props.HistorySyncConfig == nil {
			props.HistorySyncConfig = &waCompanionReg.DeviceProps_HistorySyncConfig{}
		}
		props.HistorySyncConfig.FullSyncDaysLimit = proto.Uint32(uint32(days))
	}
	encoded, err := proto.Marshal(&props)
	if err != nil {
		return fmt.Errorf("encode pairing device properties: %w", err)
	}
	payload.DevicePairingData.DeviceProps = encoded
	return nil
}
