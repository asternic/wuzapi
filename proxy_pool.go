package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// ErrProxyPoolExhausted is returned when the proxy pool has enabled entries
// but every one of them is already carrying its maximum number of devices.
var ErrProxyPoolExhausted = errors.New("proxy pool exhausted: every enabled pool proxy is at max_devices capacity")

// ProxyPoolFallback decides what Connect does when every enabled pool proxy
// is full. "block" (default) refuses to connect so a number never silently
// egresses from the host IP; "direct" connects without a proxy instead.
type ProxyPoolFallback string

const (
	ProxyPoolFallbackBlock  ProxyPoolFallback = "block"
	ProxyPoolFallbackDirect ProxyPoolFallback = "direct"
)

// proxyPoolFallback is set at startup from -proxypoolfallback /
// WUZAPI_PROXY_POOL_FALLBACK.
var proxyPoolFallback = ProxyPoolFallbackBlock

func parseProxyPoolFallback(value string) (ProxyPoolFallback, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ProxyPoolFallbackBlock):
		return ProxyPoolFallbackBlock, nil
	case string(ProxyPoolFallbackDirect):
		return ProxyPoolFallbackDirect, nil
	default:
		return "", fmt.Errorf("invalid proxy pool fallback %q: expected block or direct", value)
	}
}

// proxyPoolMu serializes pool assignment inside this process so two sessions
// connecting at the same time cannot both claim the last free slot of a proxy.
// On PostgreSQL the assignment transaction additionally locks the pool rows.
var proxyPoolMu sync.Mutex

// ProxyPoolEntry is one proxy in the admin-managed pool.
type ProxyPoolEntry struct {
	ID            string `db:"id" json:"id"`
	Label         string `db:"label" json:"label"`
	ProxyURL      string `db:"proxy_url" json:"proxy_url"`
	MaxDevices    int    `db:"max_devices" json:"max_devices"`
	Enabled       bool   `db:"enabled" json:"enabled"`
	AssignedCount int    `db:"assigned_count" json:"assigned_count"`
}

const proxyPoolSelectSQL = `
	SELECT p.id, p.label, p.proxy_url, p.max_devices, p.enabled, COUNT(u.id) AS assigned_count
	FROM proxy_pool p
	LEFT JOIN users u ON u.proxy_pool_id = p.id
	%s
	GROUP BY p.id, p.label, p.proxy_url, p.max_devices, p.enabled, p.created_at
	%s
	ORDER BY %s`

// validateProxyURL checks that a proxy URL uses a scheme supported by startClient.
func validateProxyURL(raw string) error {
	if raw == "" {
		return errors.New("missing proxy_url in payload")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid proxy URL format")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "socks5" {
		return errors.New("only HTTP and SOCKS5 proxies are supported")
	}
	if parsed.Host == "" {
		return errors.New("invalid proxy URL format")
	}
	return nil
}

// maskProxyURL hides the password of a proxy URL for API responses.
func maskProxyURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Redacted()
}

func (e ProxyPoolEntry) masked() ProxyPoolEntry {
	e.ProxyURL = maskProxyURL(e.ProxyURL)
	return e
}

// assignProxyFromPool gives a user without a proxy the least-loaded enabled pool
// proxy that still has capacity, and persists the choice on the user row.
//
// It returns the proxy URL the user will connect through ("" when the pool is
// not in use). Users that already have a proxy_url — set explicitly or by an
// earlier assignment — are left untouched, which keeps assignments sticky.
// When the pool has enabled entries but all are full, ErrProxyPoolExhausted is
// returned so the caller can refuse to connect instead of using the host IP.
func (s *server) assignProxyFromPool(userID string) (string, error) {
	proxyPoolMu.Lock()
	defer proxyPoolMu.Unlock()
	return s.assignProxyFromPoolLocked(userID)
}

// Caller holds proxyPoolMu. PostgreSQL locks always take pool rows before
// user rows, matching pool URL edits, which update assigned users as well.
func (s *server) assignProxyFromPoolLocked(userID string) (string, error) {
	tx, err := s.db.Beginx()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if s.db.DriverName() == "postgres" {
		if _, err = tx.Exec("SELECT id FROM proxy_pool ORDER BY id FOR UPDATE"); err != nil {
			return "", err
		}
	}

	var proxyURL string
	var poolID sql.NullString
	queryUser := "SELECT COALESCE(proxy_url, ''), proxy_pool_id FROM users WHERE id = $1"
	if s.db.DriverName() == "postgres" {
		queryUser += " FOR UPDATE"
	}
	err = tx.QueryRow(queryUser, userID).Scan(&proxyURL, &poolID)
	if err != nil {
		return "", err
	}
	if proxyURL != "" {
		return proxyURL, nil
	}

	if poolID.Valid && poolID.String != "" {
		// Assigned earlier but the copied URL was lost: restore it from the same
		// pool entry so the number keeps its IP, even if the entry is disabled.
		if err = tx.QueryRow("SELECT proxy_url FROM proxy_pool WHERE id = $1", poolID.String).Scan(&proxyURL); err != nil {
			return "", err
		}
	} else {
		var enabledCount int
		if err = tx.Get(&enabledCount, "SELECT COUNT(*) FROM proxy_pool WHERE enabled = TRUE"); err != nil {
			return "", err
		}
		if enabledCount == 0 {
			return "", nil
		}

		var entry ProxyPoolEntry
		query := fmt.Sprintf(proxyPoolSelectSQL,
			"WHERE p.enabled = TRUE",
			"HAVING COUNT(u.id) < p.max_devices",
			"CAST(COUNT(u.id) AS REAL) / p.max_devices, p.created_at, p.id LIMIT 1")
		err = tx.Get(&entry, query)
		if err == sql.ErrNoRows {
			return "", ErrProxyPoolExhausted
		}
		if err != nil {
			return "", err
		}
		poolID = sql.NullString{String: entry.ID, Valid: true}
		proxyURL = entry.ProxyURL
	}

	if _, err = tx.Exec("UPDATE users SET proxy_url = $1, proxy_pool_id = $2 WHERE id = $3", proxyURL, poolID.String, userID); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}

	log.Info().Str("user_id", userID).Str("proxy_pool_id", poolID.String).Str("proxy", maskProxyURL(proxyURL)).Msg("Assigned proxy from pool")
	return proxyURL, nil
}

// proxyForConnect resolves the proxy a session connects through, assigning one
// from the pool when needed. When the pool is exhausted it returns
// ErrProxyPoolExhausted, unless the fallback is "direct": then the session
// connects without a proxy, and nothing is saved, so the next connect tries
// the pool again.
func (s *server) proxyForConnect(userID string) (string, error) {
	proxyPoolMu.Lock()
	defer proxyPoolMu.Unlock()
	return s.proxyForConnectLocked(userID)
}

func (s *server) proxyForConnectLocked(userID string) (string, error) {
	proxyURL, err := s.assignProxyFromPoolLocked(userID)
	if errors.Is(err, ErrProxyPoolExhausted) && proxyPoolFallback == ProxyPoolFallbackDirect {
		log.Warn().Str("user_id", userID).Msg("Proxy pool exhausted; connecting without a proxy because WUZAPI_PROXY_POOL_FALLBACK=direct")
		return "", nil
	}
	return proxyURL, err
}

// refreshUserProxyCache updates the cached userinfo Proxy value for a user.
func (s *server) refreshUserProxyCache(userID, proxyURL string) {
	var token string
	if err := s.db.Get(&token, "SELECT token FROM users WHERE id = $1", userID); err != nil {
		return
	}
	if cachedUserInfo, found := userinfocache.Get(token); found {
		updatedUserInfo := updateUserInfo(cachedUserInfo.(Values), "Proxy", proxyURL).(Values)
		userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
	}
}

func (s *server) getProxyPoolEntry(id string) (ProxyPoolEntry, error) {
	return getProxyPoolEntry(s.db, id)
}

func getProxyPoolEntry(db sqlx.Queryer, id string) (ProxyPoolEntry, error) {
	var entry ProxyPoolEntry
	query := fmt.Sprintf(proxyPoolSelectSQL, "WHERE p.id = $1", "", "p.id")
	err := sqlx.Get(db, &entry, query, id)
	return entry, err
}

// Lock before reading aggregates: a concurrent process may be assigning users
// or editing this entry. GROUP BY queries themselves cannot use FOR UPDATE.
func (s *server) lockProxyPoolEntry(tx *sqlx.Tx, id string) error {
	if s.db.DriverName() != "postgres" {
		return nil
	}
	var lockedID string
	return tx.Get(&lockedID, "SELECT id FROM proxy_pool WHERE id = $1 FOR UPDATE", id)
}

func (s *server) respondAdminError(w http.ResponseWriter, status int, message string) {
	s.respondWithJSON(w, status, map[string]interface{}{
		"code":    status,
		"error":   message,
		"success": false,
	})
}

// ListProxyPool returns every pool entry with its current assignment count.
func (s *server) ListProxyPool() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries := []ProxyPoolEntry{}
		query := fmt.Sprintf(proxyPoolSelectSQL, "", "", "p.created_at, p.id")
		if err := s.db.Select(&entries, query); err != nil {
			log.Error().Err(err).Msg("Failed to list proxy pool")
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		for i := range entries {
			entries[i] = entries[i].masked()
		}
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"code":    http.StatusOK,
			"data":    entries,
			"success": true,
		})
	}
}

// AddProxyPoolEntry adds a proxy to the pool.
func (s *server) AddProxyPoolEntry() http.HandlerFunc {
	type addStruct struct {
		Label      string `json:"label"`
		ProxyURL   string `json:"proxy_url"`
		MaxDevices *int   `json:"max_devices,omitempty"`
		Enabled    *bool  `json:"enabled,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var t addStruct
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			s.respondAdminError(w, http.StatusBadRequest, "could not decode payload")
			return
		}

		t.ProxyURL = strings.TrimSpace(t.ProxyURL)
		if err := validateProxyURL(t.ProxyURL); err != nil {
			s.respondAdminError(w, http.StatusBadRequest, err.Error())
			return
		}
		maxDevices := 1
		if t.MaxDevices != nil {
			maxDevices = *t.MaxDevices
		}
		if maxDevices < 1 {
			s.respondAdminError(w, http.StatusBadRequest, "max_devices must be at least 1")
			return
		}
		enabled := true
		if t.Enabled != nil {
			enabled = *t.Enabled
		}

		var exists int
		if err := s.db.Get(&exists, "SELECT COUNT(*) FROM proxy_pool WHERE proxy_url = $1", t.ProxyURL); err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		if exists > 0 {
			s.respondAdminError(w, http.StatusConflict, "proxy_url already exists in the pool")
			return
		}

		id, err := GenerateRandomID()
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "failed to generate ID")
			return
		}
		if _, err = s.db.Exec(
			"INSERT INTO proxy_pool (id, label, proxy_url, max_devices, enabled) VALUES ($1, $2, $3, $4, $5)",
			id, strings.TrimSpace(t.Label), t.ProxyURL, maxDevices, enabled,
		); err != nil {
			log.Error().Err(err).Msg("Failed to insert proxy pool entry")
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}

		entry, err := s.getProxyPoolEntry(id)
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		s.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
			"code":    http.StatusCreated,
			"data":    entry.masked(),
			"success": true,
		})
	}
}

// EditProxyPoolEntry updates label, proxy_url, max_devices or enabled of a pool entry.
// Changing proxy_url also updates every user assigned to the entry; the new
// address is used the next time those sessions connect.
func (s *server) EditProxyPoolEntry() http.HandlerFunc {
	type editStruct struct {
		Label      *string `json:"label,omitempty"`
		ProxyURL   *string `json:"proxy_url,omitempty"`
		MaxDevices *int    `json:"max_devices,omitempty"`
		Enabled    *bool   `json:"enabled,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]

		var t editStruct
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			s.respondAdminError(w, http.StatusBadRequest, "could not decode payload")
			return
		}

		proxyPoolMu.Lock()
		defer proxyPoolMu.Unlock()

		tx, err := s.db.Beginx()
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tx.Rollback()
		err = s.lockProxyPoolEntry(tx, id)
		var current ProxyPoolEntry
		if err == nil {
			current, err = getProxyPoolEntry(tx, id)
		}
		if err == sql.ErrNoRows {
			s.respondAdminError(w, http.StatusNotFound, "proxy pool entry not found")
			return
		}
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}

		updated := current
		if t.Label != nil {
			updated.Label = strings.TrimSpace(*t.Label)
		}
		if t.ProxyURL != nil {
			updated.ProxyURL = strings.TrimSpace(*t.ProxyURL)
			if err := validateProxyURL(updated.ProxyURL); err != nil {
				s.respondAdminError(w, http.StatusBadRequest, err.Error())
				return
			}
			var exists int
			if err := tx.Get(&exists, "SELECT COUNT(*) FROM proxy_pool WHERE proxy_url = $1 AND id <> $2", updated.ProxyURL, id); err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "database error")
				return
			}
			if exists > 0 {
				s.respondAdminError(w, http.StatusConflict, "proxy_url already exists in the pool")
				return
			}
		}
		if t.MaxDevices != nil {
			if *t.MaxDevices < 1 {
				s.respondAdminError(w, http.StatusBadRequest, "max_devices must be at least 1")
				return
			}
			if *t.MaxDevices < current.AssignedCount {
				s.respondAdminError(w, http.StatusConflict, fmt.Sprintf("max_devices cannot be lower than the %d devices currently assigned; release them first", current.AssignedCount))
				return
			}
			updated.MaxDevices = *t.MaxDevices
		}
		if t.Enabled != nil {
			updated.Enabled = *t.Enabled
		}

		if _, err = tx.Exec(
			"UPDATE proxy_pool SET label = $1, proxy_url = $2, max_devices = $3, enabled = $4 WHERE id = $5",
			updated.Label, updated.ProxyURL, updated.MaxDevices, updated.Enabled, id,
		); err != nil {
			log.Error().Err(err).Msg("Failed to update proxy pool entry")
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		urlChanged := updated.ProxyURL != current.ProxyURL
		if urlChanged {
			if _, err = tx.Exec("UPDATE users SET proxy_url = $1 WHERE proxy_pool_id = $2", updated.ProxyURL, id); err != nil {
				s.respondAdminError(w, http.StatusInternalServerError, "database error")
				return
			}
		}
		if err = tx.Commit(); err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}

		if urlChanged {
			var userIDs []string
			if err := s.db.Select(&userIDs, "SELECT id FROM users WHERE proxy_pool_id = $1", id); err == nil {
				for _, userID := range userIDs {
					s.refreshUserProxyCache(userID, updated.ProxyURL)
				}
			}
		}

		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"code":    http.StatusOK,
			"data":    updated.masked(),
			"success": true,
		})
	}
}

// DeleteProxyPoolEntry removes a pool entry that no user is assigned to.
func (s *server) DeleteProxyPoolEntry() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]

		proxyPoolMu.Lock()
		defer proxyPoolMu.Unlock()

		tx, err := s.db.Beginx()
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tx.Rollback()
		err = s.lockProxyPoolEntry(tx, id)
		var entry ProxyPoolEntry
		if err == nil {
			entry, err = getProxyPoolEntry(tx, id)
		}
		if err == sql.ErrNoRows {
			s.respondAdminError(w, http.StatusNotFound, "proxy pool entry not found")
			return
		}
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		if entry.AssignedCount > 0 {
			s.respondAdminError(w, http.StatusConflict, fmt.Sprintf("proxy is assigned to %d device(s); disable it and release those users first", entry.AssignedCount))
			return
		}

		if _, err = tx.Exec("DELETE FROM proxy_pool WHERE id = $1", id); err != nil {
			log.Error().Err(err).Msg("Failed to delete proxy pool entry")
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		if err = tx.Commit(); err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"code":    http.StatusOK,
			"data":    map[string]string{"id": id},
			"success": true,
			"details": "proxy pool entry deleted successfully",
		})
	}
}

// ReleaseUserProxyPool frees the pool slot held by a user. The user's proxy is
// cleared, so the next connect assigns a pool proxy again if one is available.
func (s *server) ReleaseUserProxyPool() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := mux.Vars(r)["id"]

		proxyPoolMu.Lock()
		defer proxyPoolMu.Unlock()
		if sessionActive(userID) {
			s.respondAdminError(w, http.StatusConflict, "cannot release proxy while session is active. Please disconnect and wait for shutdown first")
			return
		}

		var poolID sql.NullString
		err := s.db.QueryRow("SELECT proxy_pool_id FROM users WHERE id = $1", userID).Scan(&poolID)
		if err == sql.ErrNoRows {
			s.respondAdminError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !poolID.Valid || poolID.String == "" {
			s.respondAdminError(w, http.StatusBadRequest, "user has no proxy assigned from the pool")
			return
		}

		// Compare the assignment too: a concurrent explicit proxy update on
		// another process must never be cleared by this release.
		result, err := s.db.Exec("UPDATE users SET proxy_url = '', proxy_pool_id = NULL WHERE id = $1 AND proxy_pool_id = $2", userID, poolID.String)
		if err != nil {
			s.respondAdminError(w, http.StatusInternalServerError, "database error")
			return
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			s.respondAdminError(w, http.StatusConflict, "proxy assignment changed; retry the request")
			return
		}
		s.refreshUserProxyCache(userID, "")

		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"code":    http.StatusOK,
			"data":    map[string]string{"id": userID, "proxy_pool_id": poolID.String},
			"success": true,
			"details": "proxy pool assignment released",
		})
	}
}
