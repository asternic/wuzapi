package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func deviceTestServer(t *testing.T, db *sqlx.DB) *server {
	t.Helper()
	if db.DriverName() == "sqlite" {
		db.SetMaxOpenConns(1)
		if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
			t.Fatal(err)
		}
	}
	// Replace the placeholder used by historyTestDB with the real store schema.
	if _, err := db.Exec(`DROP TABLE whatsmeow_message_secrets`); err != nil {
		t.Fatal(err)
	}
	stores := sqlstore.NewWithDB(db.DB, db.DriverName(), nil)
	if err := stores.Upgrade(context.Background()); err != nil {
		t.Fatal(err)
	}
	previous := container
	container = stores
	t.Cleanup(func() { container = previous })
	if err := initializeSchema(db); err != nil {
		t.Fatal(err)
	}
	return &server{db: db, exPath: t.TempDir()}
}

func savedTestDevice(t *testing.T, jidText string) *store.Device {
	t.Helper()
	jid, err := types.ParseJID(jidText)
	if err != nil {
		t.Fatal(err)
	}
	device := container.NewDevice()
	device.ID = &jid
	device.Account = &waAdv.ADVSignedDeviceIdentity{Details: []byte{}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64)}
	if err := device.Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	return device
}

func insertDeviceTestUser(t *testing.T, s *server, id, account, exact string) {
	t.Helper()
	_, err := s.db.Exec(s.db.Rebind(`INSERT INTO users(id,name,token,jid,device_jid,connected) VALUES (?,?,?,?,?,1)`), id, id, id, account, exact)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExactDeviceResolutionAndDeletion(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		s := deviceTestServer(t, db)
		ctx := context.Background()
		savedTestDevice(t, "15551234567:92@s.whatsapp.net")
		savedTestDevice(t, "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "current", "15551234567@s.whatsapp.net", "15551234567:95@s.whatsapp.net")
		device, err := s.resolveDeviceStore(ctx, "current")
		if err != nil {
			t.Fatal(err)
		}
		if device.ID.String() != "15551234567:95@s.whatsapp.net" {
			t.Fatalf("wrong device: %v", device.ID)
		}
		if err := s.deleteSessionDevice(ctx, "current"); err != nil {
			t.Fatal(err)
		}
		if err := s.deleteSessionDevice(ctx, "current"); err != nil {
			t.Fatalf("cleanup not idempotent: %v", err)
		}
		all, err := container.GetAllDevices(ctx)
		if err != nil || len(all) != 1 || all[0].ID.Device != 92 {
			t.Fatalf("deleted unrelated device: %v %v", all, err)
		}
		// Missing exact credentials must allow fresh pairing, not reuse stale :92.
		device, err = s.resolveDeviceStore(ctx, "current")
		if err != nil || device.ID != nil {
			t.Fatalf("reused another device: %v %v", device, err)
		}
	})
}

func TestLegacyDeviceResolution(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		s := deviceTestServer(t, db)
		ctx := context.Background()
		savedTestDevice(t, "15551234567:92@s.whatsapp.net")
		insertDeviceTestUser(t, s, "legacy", "15551234567@s.whatsapp.net", "")
		device, err := s.resolveDeviceStore(ctx, "legacy")
		if err != nil || device.ID.Device != 92 {
			t.Fatalf("singleton upgrade failed: %v %v", device, err)
		}
		var exact string
		if err := db.Get(&exact, `SELECT device_jid FROM users WHERE id='legacy'`); err != nil || exact != device.ID.String() {
			t.Fatalf("mapping not saved: %q %v", exact, err)
		}
		savedTestDevice(t, "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "ambiguous", "15551234567@s.whatsapp.net", "")
		if _, err := s.resolveDeviceStore(ctx, "ambiguous"); !errors.Is(err, errAmbiguousSessionDevice) {
			t.Fatalf("ambiguous match accepted: %v", err)
		}
		if err := s.deleteSessionDevice(ctx, "ambiguous"); !errors.Is(err, errAmbiguousSessionDevice) {
			t.Fatalf("ambiguous deletion accepted: %v", err)
		}
		insertDeviceTestUser(t, s, "full-jid", "15551234567:95@s.whatsapp.net", "")
		device, err = s.resolveDeviceStore(ctx, "full-jid")
		if err != nil || device.ID.Device != 95 {
			t.Fatalf("legacy exact JID failed: %v %v", device, err)
		}
	})
}

func TestFullDeletionWithoutLiveClient(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		s := deviceTestServer(t, db)
		savedTestDevice(t, "15551234567:92@s.whatsapp.net")
		savedTestDevice(t, "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "delete", "15551234567@s.whatsapp.net", "15551234567:92@s.whatsapp.net")
		insertDeviceTestUser(t, s, "keep", "15551234567@s.whatsapp.net", "15551234567:95@s.whatsapp.net")
		req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"id": "delete"})
		response := httptest.NewRecorder()
		s.DeleteUserComplete()(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("delete failed: %d %s", response.Code, response.Body.String())
		}
		var count int
		if err := db.Get(&count, `SELECT COUNT(*) FROM users WHERE id='delete'`); err != nil || count != 0 {
			t.Fatalf("user retained: %d %v", count, err)
		}
		all, err := container.GetAllDevices(context.Background())
		if err != nil || len(all) != 1 || all[0].ID.Device != 95 {
			t.Fatalf("wrong remaining devices: %v %v", all, err)
		}
	})
}

func TestPairingPersistsExactDeviceAndRemoteLogoutDeletesIt(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		s := deviceTestServer(t, db)
		device := savedTestDevice(t, "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "paired", "", "")
		client := &MyClient{WAClient: whatsmeow.NewClient(device, nil), userID: "paired", token: "paired", db: db, s: s}
		client.myEventHandler(&events.PairSuccess{ID: *device.ID})
		var account, exact string
		if err := db.QueryRow(`SELECT jid,device_jid FROM users WHERE id='paired'`).Scan(&account, &exact); err != nil {
			t.Fatal(err)
		}
		if account != "15551234567@s.whatsapp.net" || exact != "15551234567:95@s.whatsapp.net" {
			t.Fatalf("lost identity: %q %q", account, exact)
		}
		client.myEventHandler(&events.LoggedOut{Reason: events.ConnectFailureLoggedOut})
		var connected int
		if err := db.Get(&connected, `SELECT connected FROM users WHERE id='paired'`); err != nil || connected != 0 {
			t.Fatalf("logout state: %d %v", connected, err)
		}
		all, err := container.GetAllDevices(context.Background())
		if err != nil || len(all) != 0 {
			t.Fatalf("credentials survived remote logout: %v %v", all, err)
		}
		if device.ID == nil {
			t.Fatal("cleanup mutated the live Store")
		}
	})
}

func TestDeviceOwnershipMigration(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := createMigrationsTable(db); err != nil {
			t.Fatal(err)
		}
		for _, migration := range migrations {
			if migration.ID != 15 {
				if err := applyMigration(db, migration); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err := db.Exec(`INSERT INTO users(id,name,token,jid) VALUES ('legacy','legacy','legacy','15551234567@s.whatsapp.net')`); err != nil {
			t.Fatal(err)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM migrations WHERE id=15`); err != nil {
			t.Fatal(err)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		var exact, account string
		if err := db.QueryRow(`SELECT device_jid,jid FROM users WHERE id='legacy'`).Scan(&exact, &account); err != nil {
			t.Fatal(err)
		}
		if exact != "" || account != "15551234567@s.whatsapp.net" {
			t.Fatalf("migration changed legacy identity: %q %q", exact, account)
		}
		if _, err := db.Exec(`UPDATE users SET device_jid='15551234567:95@s.whatsapp.net' WHERE id='legacy'`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO users(id,name,token,device_jid) VALUES ('duplicate','duplicate','duplicate','15551234567:95@s.whatsapp.net')`); err == nil {
			t.Fatal("duplicate device ownership accepted")
		}
	})
}

func TestAmbiguousFullDeletionRetainsUserAndDevices(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		s := deviceTestServer(t, db)
		savedTestDevice(t, "15551234567:92@s.whatsapp.net")
		savedTestDevice(t, "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "ambiguous-delete", "15551234567@s.whatsapp.net", "")
		req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"id": "ambiguous-delete"})
		response := httptest.NewRecorder()
		s.DeleteUserComplete()(response, req)
		if response.Code != http.StatusConflict {
			t.Fatalf("unsafe deletion: %d %s", response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), `"error_code":"ambiguous_device_ownership"`) {
			t.Fatalf("missing recovery code: %s", response.Body.String())
		}
		var count int
		if err := db.Get(&count, `SELECT COUNT(*) FROM users WHERE id='ambiguous-delete'`); err != nil || count != 1 {
			t.Fatalf("user removed: %d %v", count, err)
		}
		all, err := container.GetAllDevices(context.Background())
		if err != nil || len(all) != 2 {
			t.Fatalf("devices removed: %v %v", all, err)
		}
	})
}

func TestDeletionReservationBlocksConnect(t *testing.T) {
	s := makeTestServer(t)
	insertDeviceTestUser(t, s, "deleting", "", "")
	if err := reserveSessionDeletion("deleting"); err != nil {
		t.Fatal(err)
	}
	defer releaseSessionDeletion("deleting")
	if _, _, err := s.prepareClientStart("deleting", false); err == nil {
		t.Fatal("connect allowed during deletion")
	}
	if err := reserveSessionDeletion("deleting"); err == nil {
		t.Fatal("concurrent deletion allowed")
	}
	releaseSessionDeletion("deleting")
	if _, err := s.db.Exec(`DELETE FROM users WHERE id='deleting'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.prepareClientStart("deleting", false); err == nil {
		t.Fatal("stale connect request restarted deleted user")
	}
}

func TestLegacyUserCannotClaimOrDeleteAnotherUsersDevice(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		s := deviceTestServer(t, db)
		savedTestDevice(t, "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "owner", "15551234567@s.whatsapp.net", "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "unmapped", "15551234567@s.whatsapp.net", "")
		if _, err := s.resolveDeviceStore(context.Background(), "unmapped"); err == nil {
			t.Fatal("claimed another user's device")
		}
		if err := s.deleteSessionDevice(context.Background(), "unmapped"); err == nil {
			t.Fatal("deleted another user's device")
		}
		all, err := container.GetAllDevices(context.Background())
		if err != nil || len(all) != 1 {
			t.Fatalf("owner's device removed: %v %v", all, err)
		}
	})
}

type unavailableDeviceStore struct{ err error }

func (s unavailableDeviceStore) GetDevice(context.Context, types.JID) (*store.Device, error) {
	return nil, s.err
}
func (s unavailableDeviceStore) GetAllDevices(context.Context) ([]*store.Device, error) {
	return nil, s.err
}

func TestDeviceLookupErrorsDoNotFallBack(t *testing.T) {
	failure := errors.New("device database unavailable")
	for _, exact := range []string{"", "15551234567:95@s.whatsapp.net"} {
		if device, err := findSessionDevice(context.Background(), unavailableDeviceStore{failure}, exact, "15551234567@s.whatsapp.net"); !errors.Is(err, failure) || device != nil {
			t.Fatalf("database failure masked: %v %v", device, err)
		}
	}
}
