package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func insertPoolTestUser(t *testing.T, s *server, id, proxyURL string) {
	t.Helper()
	if _, err := s.db.Exec(
		"INSERT INTO users (id, name, token, proxy_url) VALUES ($1, $2, $3, $4)",
		id, id, "token-"+id, proxyURL,
	); err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func insertPoolTestEntry(t *testing.T, s *server, id, proxyURL string, maxDevices int, enabled bool) {
	t.Helper()
	if _, err := s.db.Exec(
		"INSERT INTO proxy_pool (id, label, proxy_url, max_devices, enabled) VALUES ($1, $2, $3, $4, $5)",
		id, id, proxyURL, maxDevices, enabled,
	); err != nil {
		t.Fatalf("insert pool entry %s: %v", id, err)
	}
}

func userPoolState(t *testing.T, s *server, id string) (string, string) {
	t.Helper()
	var proxyURL string
	var poolID sql.NullString
	if err := s.db.QueryRow("SELECT COALESCE(proxy_url, ''), proxy_pool_id FROM users WHERE id = $1", id).Scan(&proxyURL, &poolID); err != nil {
		t.Fatalf("read user %s: %v", id, err)
	}
	return proxyURL, poolID.String
}

func adminRequest(t *testing.T, s *server, method, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return rec.Code, resp
}

func TestAssignProxyFromPoolEmptyPoolKeepsCurrentBehavior(t *testing.T) {
	s := makeTestServer(t)
	insertPoolTestUser(t, s, "u1", "")

	proxyURL, err := s.assignProxyFromPool("u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proxyURL != "" {
		t.Fatalf("expected no proxy with an empty pool, got %q", proxyURL)
	}

	// A pool with only disabled entries behaves like an empty pool.
	insertPoolTestEntry(t, s, "p1", "socks5://10.0.0.1:1080", 1, false)
	if proxyURL, err = s.assignProxyFromPool("u1"); err != nil || proxyURL != "" {
		t.Fatalf("expected no assignment from a disabled-only pool, got %q, %v", proxyURL, err)
	}
	if got, poolID := userPoolState(t, s, "u1"); got != "" || poolID != "" {
		t.Fatalf("user should be untouched, got proxy=%q pool=%q", got, poolID)
	}
}

func TestAssignProxyFromPoolCapacityAndLeastLoaded(t *testing.T) {
	s := makeTestServer(t)
	insertPoolTestEntry(t, s, "p1", "socks5://10.0.0.1:1080", 2, true)
	insertPoolTestEntry(t, s, "p2", "http://10.0.0.2:3128", 1, true)
	for _, id := range []string{"u1", "u2", "u3", "u4"} {
		insertPoolTestUser(t, s, id, "")
	}

	// Least loaded first: u1 -> p1 (0/2 vs 0/1, tie broken by creation order),
	// u2 -> p2 (p1 is 1/2 = 50%, p2 is 0/1 = 0%), u3 -> p1, u4 -> exhausted.
	expected := map[string]string{"u1": "p1", "u2": "p2", "u3": "p1"}
	for _, id := range []string{"u1", "u2", "u3"} {
		if _, err := s.assignProxyFromPool(id); err != nil {
			t.Fatalf("assign %s: %v", id, err)
		}
		if _, poolID := userPoolState(t, s, id); poolID != expected[id] {
			t.Fatalf("expected %s on %s, got %q", id, expected[id], poolID)
		}
	}

	if _, err := s.assignProxyFromPool("u4"); !errors.Is(err, ErrProxyPoolExhausted) {
		t.Fatalf("expected ErrProxyPoolExhausted, got %v", err)
	}
	if got, poolID := userPoolState(t, s, "u4"); got != "" || poolID != "" {
		t.Fatalf("exhausted user must not get a proxy, got proxy=%q pool=%q", got, poolID)
	}
}

func TestAssignProxyFromPoolIsSticky(t *testing.T) {
	s := makeTestServer(t)
	insertPoolTestEntry(t, s, "p1", "socks5://10.0.0.1:1080", 1, true)
	insertPoolTestUser(t, s, "u1", "")

	first, err := s.assignProxyFromPool("u1")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Disabling the entry stops new assignments but keeps existing ones.
	if _, err := s.db.Exec("UPDATE proxy_pool SET enabled = FALSE WHERE id = 'p1'"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	insertPoolTestEntry(t, s, "p2", "socks5://10.0.0.2:1080", 5, true)

	second, err := s.assignProxyFromPool("u1")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if second != first {
		t.Fatalf("assignment must be sticky: first %q, second %q", first, second)
	}

	// If the copied URL is lost, it is restored from the same pool entry.
	if _, err := s.db.Exec("UPDATE users SET proxy_url = '' WHERE id = 'u1'"); err != nil {
		t.Fatalf("clear url: %v", err)
	}
	third, err := s.assignProxyFromPool("u1")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if third != first {
		t.Fatalf("expected restored proxy %q, got %q", first, third)
	}
	if _, poolID := userPoolState(t, s, "u1"); poolID != "p1" {
		t.Fatalf("expected user to stay on p1, got %q", poolID)
	}
}

func TestAssignProxyFromPoolExplicitProxyWins(t *testing.T) {
	s := makeTestServer(t)
	insertPoolTestEntry(t, s, "p1", "socks5://10.0.0.1:1080", 1, true)
	insertPoolTestUser(t, s, "u1", "http://203.0.113.5:8080")

	proxyURL, err := s.assignProxyFromPool("u1")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if proxyURL != "http://203.0.113.5:8080" {
		t.Fatalf("explicit proxy must be kept, got %q", proxyURL)
	}
	if _, poolID := userPoolState(t, s, "u1"); poolID != "" {
		t.Fatalf("explicit proxy user must not take a pool slot, got %q", poolID)
	}
}

func TestAssignProxyFromPoolConcurrentConnects(t *testing.T) {
	s := makeTestServer(t)
	s.db.SetMaxOpenConns(1) // in-memory SQLite is per connection
	insertPoolTestEntry(t, s, "p1", "socks5://10.0.0.1:1080", 2, true)
	insertPoolTestEntry(t, s, "p2", "socks5://10.0.0.2:1080", 1, true)

	const users = 10
	for i := 0; i < users; i++ {
		insertPoolTestUser(t, s, "u"+string(rune('a'+i)), "")
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	assigned, exhausted := 0, 0
	for i := 0; i < users; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			proxyURL, err := s.assignProxyFromPool(id)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, ErrProxyPoolExhausted):
				exhausted++
			case err != nil:
				t.Errorf("assign %s: %v", id, err)
			case proxyURL != "":
				assigned++
			}
		}("u" + string(rune('a'+i)))
	}
	wg.Wait()

	if assigned != 3 || exhausted != users-3 {
		t.Fatalf("expected 3 assigned and %d exhausted, got %d and %d", users-3, assigned, exhausted)
	}
	var overCapacity int
	if err := s.db.Get(&overCapacity, `
		SELECT COUNT(*) FROM proxy_pool p
		WHERE (SELECT COUNT(*) FROM users u WHERE u.proxy_pool_id = p.id) > p.max_devices`); err != nil {
		t.Fatalf("capacity check: %v", err)
	}
	if overCapacity != 0 {
		t.Fatalf("%d pool entries are over capacity", overCapacity)
	}
}

func TestProxyPoolAdminCRUD(t *testing.T) {
	s := makeTestServer(t)

	// Validation
	code, _ := adminRequest(t, s, http.MethodPost, "/admin/proxy-pool", map[string]interface{}{"proxy_url": "ftp://10.0.0.1:21"})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported scheme, got %d", code)
	}
	code, _ = adminRequest(t, s, http.MethodPost, "/admin/proxy-pool", map[string]interface{}{"proxy_url": "socks5://10.0.0.1:1080", "max_devices": 0})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for max_devices 0, got %d", code)
	}

	// Create
	code, resp := adminRequest(t, s, http.MethodPost, "/admin/proxy-pool", map[string]interface{}{
		"label": "residential-1", "proxy_url": "socks5://alice:secret@10.0.0.1:1080", "max_devices": 2,
	})
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %v", code, resp)
	}
	data := resp["data"].(map[string]interface{})
	id := data["id"].(string)
	if strings.Contains(data["proxy_url"].(string), "secret") {
		t.Fatalf("proxy password must be masked, got %v", data["proxy_url"])
	}
	if data["enabled"] != true || data["max_devices"].(float64) != 2 {
		t.Fatalf("unexpected entry: %v", data)
	}

	// Duplicate
	code, _ = adminRequest(t, s, http.MethodPost, "/admin/proxy-pool", map[string]interface{}{"proxy_url": "socks5://alice:secret@10.0.0.1:1080"})
	if code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate proxy_url, got %d", code)
	}

	// Assign two users, then check the list counts.
	insertPoolTestUser(t, s, "u1", "")
	insertPoolTestUser(t, s, "u2", "")
	for _, u := range []string{"u1", "u2"} {
		if _, err := s.assignProxyFromPool(u); err != nil {
			t.Fatalf("assign %s: %v", u, err)
		}
	}
	code, resp = adminRequest(t, s, http.MethodGet, "/admin/proxy-pool", nil)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	list := resp["data"].([]interface{})
	if len(list) != 1 || list[0].(map[string]interface{})["assigned_count"].(float64) != 2 {
		t.Fatalf("unexpected list: %v", list)
	}

	// Cannot shrink below the assigned count.
	code, _ = adminRequest(t, s, http.MethodPut, "/admin/proxy-pool/"+id, map[string]interface{}{"max_devices": 1})
	if code != http.StatusConflict {
		t.Fatalf("expected 409 when shrinking below assigned, got %d", code)
	}

	// Changing the URL updates the assigned users.
	code, _ = adminRequest(t, s, http.MethodPut, "/admin/proxy-pool/"+id, map[string]interface{}{"proxy_url": "socks5://alice:rotated@10.0.0.9:1080"})
	if code != http.StatusOK {
		t.Fatalf("expected 200 on edit, got %d", code)
	}
	if got, _ := userPoolState(t, s, "u1"); got != "socks5://alice:rotated@10.0.0.9:1080" {
		t.Fatalf("assigned user proxy_url not updated, got %q", got)
	}

	// Cannot delete while assigned.
	code, _ = adminRequest(t, s, http.MethodDelete, "/admin/proxy-pool/"+id, nil)
	if code != http.StatusConflict {
		t.Fatalf("expected 409 deleting an assigned entry, got %d", code)
	}

	// Release both users, then delete.
	for _, u := range []string{"u1", "u2"} {
		code, resp = adminRequest(t, s, http.MethodPost, "/admin/users/"+u+"/proxy-pool/release", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200 releasing %s, got %d: %v", u, code, resp)
		}
		if got, poolID := userPoolState(t, s, u); got != "" || poolID != "" {
			t.Fatalf("release must clear proxy and pool id, got proxy=%q pool=%q", got, poolID)
		}
	}
	code, _ = adminRequest(t, s, http.MethodPost, "/admin/users/u1/proxy-pool/release", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 releasing a user without a pool proxy, got %d", code)
	}
	code, _ = adminRequest(t, s, http.MethodDelete, "/admin/proxy-pool/"+id, nil)
	if code != http.StatusOK {
		t.Fatalf("expected 200 deleting an unassigned entry, got %d", code)
	}
	code, _ = adminRequest(t, s, http.MethodDelete, "/admin/proxy-pool/"+id, nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting a missing entry, got %d", code)
	}
}

func TestEditUserExplicitProxyLeavesPool(t *testing.T) {
	s := makeTestServer(t)
	insertPoolTestEntry(t, s, "p1", "socks5://10.0.0.1:1080", 1, true)
	insertPoolTestUser(t, s, "u1", "")
	if _, err := s.assignProxyFromPool("u1"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	code, resp := adminRequest(t, s, http.MethodPut, "/admin/users/u1", map[string]interface{}{
		"proxyConfig": map[string]interface{}{"enabled": true, "proxyURL": "http://203.0.113.5:8080"},
	})
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", code, resp)
	}
	got, poolID := userPoolState(t, s, "u1")
	if got != "http://203.0.113.5:8080" || poolID != "" {
		t.Fatalf("explicit proxy must replace the pool assignment, got proxy=%q pool=%q", got, poolID)
	}
}

func TestValidateAndMaskProxyURL(t *testing.T) {
	for _, valid := range []string{"socks5://10.0.0.1:1080", "http://user:pass@10.0.0.1:3128"} {
		if err := validateProxyURL(valid); err != nil {
			t.Fatalf("expected %q to be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "https://10.0.0.1:443", "socks5://", "10.0.0.1:1080"} {
		if err := validateProxyURL(invalid); err == nil {
			t.Fatalf("expected %q to be rejected", invalid)
		}
	}
	if got := maskProxyURL("socks5://alice:secret@10.0.0.1:1080"); strings.Contains(got, "secret") || !strings.Contains(got, "alice") {
		t.Fatalf("unexpected mask result %q", got)
	}
}

func TestParseProxyPoolFallback(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    ProxyPoolFallback
		wantErr bool
	}{
		{name: "block", value: "block", want: ProxyPoolFallbackBlock},
		{name: "direct", value: "direct", want: ProxyPoolFallbackDirect},
		{name: "normalizes case and whitespace", value: "  DIRECT  ", want: ProxyPoolFallbackDirect},
		{name: "rejects unknown value", value: "none", wantErr: true},
		{name: "rejects empty value", value: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProxyPoolFallback(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseProxyPoolFallback(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseProxyPoolFallback(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestProxyForConnectFallback(t *testing.T) {
	original := proxyPoolFallback
	defer func() { proxyPoolFallback = original }()

	s := makeTestServer(t)
	insertPoolTestEntry(t, s, "p1", "socks5://10.0.0.1:1080", 1, true)
	insertPoolTestUser(t, s, "u1", "")
	insertPoolTestUser(t, s, "u2", "")
	if _, err := s.proxyForConnect("u1"); err != nil {
		t.Fatalf("assign u1: %v", err)
	}

	// block (default): a full pool refuses the connect.
	proxyPoolFallback = ProxyPoolFallbackBlock
	if _, err := s.proxyForConnect("u2"); !errors.Is(err, ErrProxyPoolExhausted) {
		t.Fatalf("expected ErrProxyPoolExhausted with block fallback, got %v", err)
	}

	// direct: a full pool connects without a proxy and saves nothing.
	proxyPoolFallback = ProxyPoolFallbackDirect
	proxyURL, err := s.proxyForConnect("u2")
	if err != nil || proxyURL != "" {
		t.Fatalf("expected direct connect with no proxy, got %q, %v", proxyURL, err)
	}
	if got, poolID := userPoolState(t, s, "u2"); got != "" || poolID != "" {
		t.Fatalf("direct fallback must not persist anything, got proxy=%q pool=%q", got, poolID)
	}

	// Once the pool has room again, the next connect is assigned a pool proxy.
	insertPoolTestEntry(t, s, "p2", "socks5://10.0.0.2:1080", 1, true)
	if proxyURL, err = s.proxyForConnect("u2"); err != nil || proxyURL != "socks5://10.0.0.2:1080" {
		t.Fatalf("expected pool proxy once capacity is available, got %q, %v", proxyURL, err)
	}
}
