package main

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

var errAmbiguousSessionDevice = errors.New("multiple stored devices match this account; exact device ownership must be established before reconnecting or deleting")

// Use fresh store objects for cleanup: whatsmeow also deletes (and mutates) its
// live Store when dispatching LoggedOut, so deleting that object again races.
type sessionDeviceStore interface {
	GetDevice(context.Context, types.JID) (*store.Device, error)
	GetAllDevices(context.Context) ([]*store.Device, error)
}

func findSessionDevice(ctx context.Context, stores sessionDeviceStore, exact, account string) (*store.Device, error) {
	if exact != "" {
		jid, err := types.ParseJID(exact)
		if err != nil || jid.IsEmpty() {
			return nil, fmt.Errorf("invalid saved device JID %q", exact)
		}
		// Missing exact credentials must never fall back to another device.
		return stores.GetDevice(ctx, jid)
	}
	if account == "" {
		return nil, nil
	}
	jid, ok := parseJID(account)
	if !ok || jid.IsEmpty() {
		return nil, fmt.Errorf("invalid saved account JID %q", account)
	}
	// Some older installations already saved a full device JID in users.jid.
	if jid.Device != 0 || jid.RawAgent != 0 {
		return stores.GetDevice(ctx, jid)
	}
	devices, err := stores.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	var match *store.Device
	for _, device := range devices {
		if device.ID == nil || jidUserKey(device.ID.String()) != jidUserKey(account) {
			continue
		}
		if match != nil {
			return nil, errAmbiguousSessionDevice
		}
		match = device
	}
	return match, nil
}

func (s *server) sessionDeviceIdentity(ctx context.Context, userID string) (string, string, error) {
	var exact, account string
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT COALESCE(device_jid,''), COALESCE(jid,'') FROM users WHERE id=?`), userID).Scan(&exact, &account)
	return exact, account, err
}

func (s *server) checkDeviceOwner(ctx context.Context, userID, deviceJID string, legacy bool) error {
	var claimed bool
	jid, err := types.ParseJID(deviceJID)
	if err != nil {
		return err
	}
	err = s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT EXISTS(SELECT 1 FROM users WHERE id<>? AND (device_jid=? OR (? AND COALESCE(device_jid,'')='' AND jid=?)))`), userID, deviceJID, legacy, jid.ToNonAD().String()).Scan(&claimed)
	if err != nil {
		return err
	}
	if claimed {
		return errors.New("stored device belongs to another user")
	}
	return nil
}

func (s *server) resolveDeviceStore(ctx context.Context, userID string) (*store.Device, error) {
	exact, account, err := s.sessionDeviceIdentity(ctx, userID)
	if err != nil {
		return nil, err
	}
	device, err := findSessionDevice(ctx, container, exact, account)
	if err != nil {
		return nil, err
	}
	if device == nil {
		return container.NewDevice(), nil
	}
	if err := s.checkDeviceOwner(ctx, userID, device.ID.String(), exact == "" && account != device.ID.String()); err != nil {
		return nil, err
	}
	// The unique index also protects concurrent claims from different users.
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE users SET device_jid=? WHERE id=?`), device.ID.String(), userID)
	if err != nil {
		return nil, fmt.Errorf("persist device ownership: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return nil, errors.New("user no longer exists")
	}
	return device, nil
}

// deleteSessionDevice only deletes a proven device identity. In particular it
// never deletes all devices sharing a phone number.
func (s *server) deleteSessionDevice(ctx context.Context, userID string) error {
	exact, account, err := s.sessionDeviceIdentity(ctx, userID)
	if err != nil {
		return err
	}
	if exact == "" && account == "" {
		return nil
	}
	if container == nil {
		return errors.New("device store is unavailable")
	}
	device, err := findSessionDevice(ctx, container, exact, account)
	if err != nil {
		return err
	}
	if device == nil {
		return nil
	}
	if err := s.checkDeviceOwner(ctx, userID, device.ID.String(), exact == "" && account != device.ID.String()); err != nil {
		return err
	}
	// This is a freshly loaded object, independent of the live client's Store.
	return device.Delete(ctx)
}
