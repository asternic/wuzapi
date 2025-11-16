package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestStdioHealthRequest(t *testing.T) {
	s := makeTestServer(t)

	request := newRequest("test-001", "health", nil).toJSON(t)
	response := executeRequest(t, s, request)

	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-001",
	}

	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's a success response (has result, no error)
	if response["result"] == nil {
		t.Errorf("Expected result field, got nil")
	}
	if response["error"] != nil {
		t.Errorf("Expected no error, got: %v", response["error"])
	}
}

func TestAdminUsersAddAndList(t *testing.T) {
	s := makeTestServer(t)

	// First, add a user
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "Alice",
		"token":      "alice-token-123",
	}).toJSON(t)
	addResponse := executeRequest(t, s, addRequest)

	expectedAdd := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
	}
	if diff := compareJSON(expectedAdd, addResponse); diff != "" {
		t.Errorf("Add response mismatch:\n%s", diff)
	}
	if addResponse["error"] != nil {
		t.Fatalf("Failed to add user: %v", addResponse["error"])
	}

	// Now list users to verify the user was added
	listRequest := newRequest("2", "admin.users.list", map[string]interface{}{
		"adminToken": "test-admin-token",
	}).toJSON(t)
	listResponse := executeRequest(t, s, listRequest)

	expectedList := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expectedList, listResponse); diff != "" {
		t.Errorf("List response mismatch:\n%s", diff)
	}
	if listResponse["error"] != nil {
		t.Fatalf("List request failed: %v", listResponse["error"])
	}

	// Verify the user appears in the list with correct data
	users := listResponse["result"].([]interface{})
	if len(users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(users))
	}

	user := users[0].(map[string]interface{})
	expectedUser := map[string]interface{}{
		"name":  "Alice",
		"token": "alice-token-123",
	}
	if diff := compareJSON(expectedUser, user); diff != "" {
		t.Errorf("User data mismatch:\n%s", diff)
	}
}

func TestAdminUsersGet(t *testing.T) {
	s := makeTestServer(t)

	// First add a user
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "Bob",
		"token":      "bob-token-456",
	}).toJSON(t)
	addResponse := executeRequest(t, s, addRequest)

	expectedAdd := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
	}
	if diff := compareJSON(expectedAdd, addResponse); diff != "" {
		t.Errorf("Add response mismatch:\n%s", diff)
	}

	// Extract the userId from the add response
	addData := addResponse["result"].(map[string]interface{})
	userId := addData["id"].(string)

	// Now get the specific user
	getRequest := newRequest("2", "admin.users.get", map[string]interface{}{
		"adminToken": "test-admin-token",
		"userId":     userId,
	}).toJSON(t)
	getResponse := executeRequest(t, s, getRequest)

	expectedGet := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expectedGet, getResponse); diff != "" {
		t.Errorf("Get response mismatch:\n%s", diff)
	}

	// Verify the user data
	users := getResponse["result"].([]interface{})
	if len(users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(users))
	}

	user := users[0].(map[string]interface{})
	expectedUser := map[string]interface{}{
		"name":  "Bob",
		"token": "bob-token-456",
	}
	if diff := compareJSON(expectedUser, user); diff != "" {
		t.Errorf("User data mismatch:\n%s", diff)
	}
}

func TestAdminUsersDelete(t *testing.T) {
	s := makeTestServer(t)

	// First add a user
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "Charlie",
		"token":      "charlie-token-789",
	}).toJSON(t)
	addResponse := executeRequest(t, s, addRequest)

	addData := addResponse["result"].(map[string]interface{})
	userId := addData["id"].(string)

	// Delete the user
	deleteRequest := newRequest("2", "admin.users.delete", map[string]interface{}{
		"adminToken": "test-admin-token",
		"userId":     userId,
	}).toJSON(t)
	deleteResponse := executeRequest(t, s, deleteRequest)

	expectedDelete := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expectedDelete, deleteResponse); diff != "" {
		t.Errorf("Delete response mismatch:\n%s", diff)
	}

	// Verify user is gone by listing
	listRequest := newRequest("3", "admin.users.list", map[string]interface{}{
		"adminToken": "test-admin-token",
	}).toJSON(t)
	listResponse := executeRequest(t, s, listRequest)

	expectedList := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "3",
	}
	if diff := compareJSON(expectedList, listResponse); diff != "" {
		t.Errorf("List response mismatch:\n%s", diff)
	}

	users := listResponse["result"].([]interface{})
	if len(users) != 0 {
		t.Errorf("Expected 0 users after deletion, got %d", len(users))
	}
}

func TestSessionStatus(t *testing.T) {
	s := makeTestServer(t)

	// First create a user to get a valid token
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "TestUser",
		"token":      "test-user-token",
	}).toJSON(t)
	addResponse := executeRequest(t, s, addRequest)

	expectedAdd := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
	}
	if diff := compareJSON(expectedAdd, addResponse); diff != "" {
		t.Errorf("Add response mismatch:\n%s", diff)
	}

	// Now check session status
	statusRequest := newRequest("2", "session.status", map[string]interface{}{
		"token": "test-user-token",
	}).toJSON(t)
	statusResponse := executeRequest(t, s, statusRequest)

	expectedStatus := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expectedStatus, statusResponse); diff != "" {
		t.Errorf("Status response mismatch:\n%s", diff)
	}

	// Verify response has expected fields
	data := statusResponse["result"].(map[string]interface{})
	if _, hasConnected := data["connected"]; !hasConnected {
		t.Errorf("Status response missing 'connected' field")
	}
	if _, hasLoggedIn := data["loggedIn"]; !hasLoggedIn {
		t.Errorf("Status response missing 'loggedIn' field")
	}
}

// Note: session.connect, session.disconnect, session.logout tests are skipped
// because they require full WhatsApp/whatsmeow initialization which is complex
// to set up in unit tests. The routing is tested via session.status.
// Manual/integration testing should be used for these methods.

func TestChatSendText(t *testing.T) {
	s := makeTestServer(t)

	// Create a user first
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "MessageUser",
		"token":      "message-token",
	}).toJSON(t)
	executeRequest(t, s, addRequest)

	// Try to send a message (will fail because no WhatsApp session, but tests routing)
	sendRequest := newRequest("2", "chat.send.text", map[string]interface{}{
		"token": "message-token",
		"Phone": "1234567890",
		"Body":  "Hello, World!",
	}).toJSON(t)
	sendResponse := executeRequest(t, s, sendRequest)

	// Should fail with "no session" error since we don't have WhatsApp initialized
	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expected, sendResponse); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's an error response
	if sendResponse["error"] == nil {
		t.Errorf("Expected error (no session), got success")
	}
	if sendResponse["result"] != nil {
		t.Errorf("Expected no result on error, got: %v", sendResponse["result"])
	}

	errorObj := sendResponse["error"].(map[string]interface{})
	if errorObj["code"].(float64) != 500 {
		t.Errorf("Expected error code 500 (no session), got %v", errorObj["code"])
	}
}

func TestChatHistory(t *testing.T) {
	s := makeTestServer(t)

	// Create a user with history enabled
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "HistoryUser",
		"token":      "history-token",
		"history":    100,
	}).toJSON(t)
	executeRequest(t, s, addRequest)

	// Try to get history (will fail because no WhatsApp session, but tests routing)
	historyRequest := newRequest("2", "chat.history", map[string]interface{}{
		"token":    "history-token",
		"chat_jid": "1234567890@s.whatsapp.net",
	}).toJSON(t)
	historyResponse := executeRequest(t, s, historyRequest)

	// The routing should work even if the actual operation fails
	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expected, historyResponse); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Either result or error should be present (check for key existence, not just value)
	_, hasResult := historyResponse["result"]
	_, hasError := historyResponse["error"]
	if !hasResult && !hasError {
		t.Errorf("Expected either result or error field. Got response: %+v", historyResponse)
	}
}

// testRequest builds a JSON-RPC request with type safety
type testRequest struct {
	ID     interface{} // Can be string, int, or nil
	Method string
	Params interface{}
}

// newRequest creates a new request builder with string or numeric ID
func newRequest(id interface{}, method string, params interface{}) *testRequest {
	return &testRequest{
		ID:     id,
		Method: method,
		Params: params,
	}
}

// toJSON converts the request to a JSON string
func (r *testRequest) toJSON(t *testing.T) string {
	t.Helper()

	reqData := map[string]interface{}{
		"id":     r.ID,
		"method": r.Method,
	}
	if r.Params != nil {
		reqData["params"] = r.Params
	}

	jsonBytes, err := json.Marshal(reqData)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	return string(jsonBytes)
}

// executeRequest is a helper that sends a JSON-RPC request and returns the parsed response
func executeRequest(t *testing.T, s *server, request string) map[string]interface{} {
	t.Helper()

	stdin := bytes.NewBufferString(request + "\n")
	stdout := &bytes.Buffer{}

	stdioServer := newStdioServerWithIO(s, stdin, stdout)
	if err := stdioServer.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response:\n%s\nError: %v", stdout.String(), err)
	}

	return response
}

func makeTestServer(t *testing.T) *server {
	t.Helper()

	// Set admin token for tests
	testToken := "test-admin-token"
	*adminToken = testToken

	// Use in-memory database
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Initialize schema using the same function as production
	if err := initializeSchema(db); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	s := &server{
		db:     db,
		router: mux.NewRouter(),
	}
	s.routes()

	return s
}

// assertJSONRPC20Success checks that a response is a successful JSON-RPC 2.0 response
// and returns the result data for further assertions
func assertJSONRPC20Success(t *testing.T, response map[string]interface{}, expectedID interface{}) interface{} {
	t.Helper()
	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      expectedID,
	}
	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response structure mismatch:\n%s", diff)
	}
	if response["error"] != nil {
		t.Fatalf("Expected no error, got: %v", response["error"])
	}
	if response["result"] == nil {
		t.Fatalf("Expected result field, got nil")
	}
	return response["result"]
}

// assertJSONRPC20Error checks that a response is an error JSON-RPC 2.0 response
func assertJSONRPC20Error(t *testing.T, response map[string]interface{}, expectedID interface{}, expectedCode float64) map[string]interface{} {
	t.Helper()
	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      expectedID,
	}
	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response structure mismatch:\n%s", diff)
	}
	if response["result"] != nil {
		t.Fatalf("Expected no result on error, got: %v", response["result"])
	}
	if response["error"] == nil {
		t.Fatalf("Expected error field, got nil")
	}
	errorObj := response["error"].(map[string]interface{})
	if errorObj["code"].(float64) != expectedCode {
		t.Errorf("Expected error code %v, got: %v", expectedCode, errorObj["code"])
	}
	return errorObj
}

// compareJSON compares two JSON objects and returns a human-readable diff
func compareJSON(expected, actual map[string]interface{}) string {
	var diffs []string
	for key, expectedVal := range expected {
		actualVal, exists := actual[key]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("  Missing field: %q", key))
			continue
		}
		if fmt.Sprintf("%v", expectedVal) != fmt.Sprintf("%v", actualVal) {
			diffs = append(diffs, fmt.Sprintf("  Field %q: expected %v, got %v", key, expectedVal, actualVal))
		}
	}

	if len(diffs) > 0 {
		expectedJSON, _ := json.MarshalIndent(expected, "    ", "  ")
		actualJSON, _ := json.MarshalIndent(actual, "    ", "  ")
		return fmt.Sprintf("Expected:\n    %s\n\n  Actual:\n    %s\n\n  Differences:\n%s",
			expectedJSON, actualJSON, strings.Join(diffs, "\n"))
	}

	return ""
}

func TestWebhookUpdate(t *testing.T) {
	s := makeTestServer(t)

	// Create a user first
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "WebhookUser",
		"token":      "webhook-token",
	}).toJSON(t)
	executeRequest(t, s, addRequest)

	// Update webhook to subscribe to events
	updateRequest := newRequest("2", "webhook.update", map[string]interface{}{
		"token":  "webhook-token",
		"events": []string{"Message", "Connected"},
		"active": true,
	}).toJSON(t)
	updateResponse := executeRequest(t, s, updateRequest)

	expectedUpdate := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expectedUpdate, updateResponse); diff != "" {
		t.Errorf("Update response mismatch:\n%s", diff)
	}

	// Verify the update worked by getting webhook config
	getRequest := newRequest("3", "webhook.get", map[string]interface{}{
		"token": "webhook-token",
	}).toJSON(t)
	getResponse := executeRequest(t, s, getRequest)

	expectedGet := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "3",
	}
	if diff := compareJSON(expectedGet, getResponse); diff != "" {
		t.Errorf("Get response mismatch:\n%s", diff)
	}

	// Check events are set
	data := getResponse["result"].(map[string]interface{})
	subscribe := data["subscribe"].([]interface{})
	if len(subscribe) != 2 {
		t.Errorf("Expected 2 subscribed events, got %d: %v", len(subscribe), subscribe)
	}
}

func TestWebhookSet(t *testing.T) {
	s := makeTestServer(t)

	// Create a user first
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "SetUser",
		"token":      "set-token",
	}).toJSON(t)
	executeRequest(t, s, addRequest)

	// Set webhook with events
	setRequest := newRequest("2", "webhook.set", map[string]interface{}{
		"token":      "set-token",
		"webhookurl": "http://example.com/webhook",
		"events":     []string{"Message", "Receipt"},
	}).toJSON(t)
	setResponse := executeRequest(t, s, setRequest)

	expectedSet := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expectedSet, setResponse); diff != "" {
		t.Errorf("Set response mismatch:\n%s", diff)
	}
}

func TestWebhookDelete(t *testing.T) {
	s := makeTestServer(t)

	// Create a user with webhook configured
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "DeleteUser",
		"token":      "delete-token",
	}).toJSON(t)
	executeRequest(t, s, addRequest)

	// Set a webhook first
	setRequest := newRequest("2", "webhook.set", map[string]interface{}{
		"token":      "delete-token",
		"webhookurl": "http://example.com/webhook",
		"events":     []string{"Message"},
	}).toJSON(t)
	executeRequest(t, s, setRequest)

	// Delete webhook
	deleteRequest := newRequest("3", "webhook.delete", map[string]interface{}{
		"token": "delete-token",
	}).toJSON(t)
	deleteResponse := executeRequest(t, s, deleteRequest)

	expectedDelete := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "3",
	}
	if diff := compareJSON(expectedDelete, deleteResponse); diff != "" {
		t.Errorf("Delete response mismatch:\n%s", diff)
	}

	// Verify webhook is cleared
	getRequest := newRequest("4", "webhook.get", map[string]interface{}{
		"token": "delete-token",
	}).toJSON(t)
	getResponse := executeRequest(t, s, getRequest)

	expectedGet := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "4",
	}
	if diff := compareJSON(expectedGet, getResponse); diff != "" {
		t.Errorf("Get response mismatch:\n%s", diff)
	}

	data := getResponse["result"].(map[string]interface{})
	webhook := data["webhook"].(string)
	if webhook != "" {
		t.Errorf("Expected empty webhook after delete, got: %s", webhook)
	}
}

func TestNumericRequestID(t *testing.T) {
	s := makeTestServer(t)

	// Send request with numeric ID (JSON-RPC 2.0 compliant)
	request := newRequest(42, "health", nil).toJSON(t)
	response := executeRequest(t, s, request)

	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      float64(42), // JSON unmarshals numbers as float64
	}
	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's a success response (has result, no error)
	if response["result"] == nil {
		t.Errorf("Expected result field, got nil")
	}
	if response["error"] != nil {
		t.Errorf("Expected no error, got: %v", response["error"])
	}
}

func TestNumericZeroRequestID(t *testing.T) {
	s := makeTestServer(t)

	// Send request with numeric ID 0 (valid per JSON-RPC 2.0 spec)
	request := newRequest(0, "health", nil).toJSON(t)
	response := executeRequest(t, s, request)

	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      float64(0), // JSON unmarshals numbers as float64
	}
	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's a success response (has result, no error)
	if response["result"] == nil {
		t.Errorf("Expected result field, got nil")
	}
	if response["error"] != nil {
		t.Errorf("Expected no error, got: %v", response["error"])
	}
}

func TestParseErrorReturnsNullID(t *testing.T) {
	s := makeTestServer(t)

	// Send invalid JSON to trigger parse error
	request := `{invalid json`
	response := executeRequest(t, s, request)

	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      nil, // null ID for parse errors
	}

	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's an error response (has error, no result)
	if response["error"] == nil {
		t.Errorf("Expected error field for parse error, got nil")
	}
	if response["result"] != nil {
		t.Errorf("Expected no result for parse error, got: %v", response["result"])
	}
}

func TestMissingUserIdParam(t *testing.T) {
	s := makeTestServer(t)

	// Test admin.users.get without userId parameter
	request := newRequest("1", "admin.users.get", map[string]interface{}{
		"adminToken": "test-admin-token",
		// userId missing
	}).toJSON(t)
	response := executeRequest(t, s, request)

	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"error": map[string]interface{}{
			"code":    float64(400),
			"message": "missing or invalid userId parameter",
		},
	}

	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's an error response (has error, no result)
	if response["error"] == nil {
		t.Errorf("Expected error field, got nil")
	}
	if response["result"] != nil {
		t.Errorf("Expected no result for error, got: %v", response["result"])
	}
}

func TestInvalidUserIdParamType(t *testing.T) {
	s := makeTestServer(t)

	// Test with invalid userId type (number instead of string)
	request := newRequest("1", "admin.users.get", map[string]interface{}{
		"adminToken": "test-admin-token",
		"userId":     12345, // number instead of string
	}).toJSON(t)
	response := executeRequest(t, s, request)

	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"error": map[string]interface{}{
			"code":    float64(400),
			"message": "missing or invalid userId parameter",
		},
	}

	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's an error response (has error, no result)
	if response["error"] == nil {
		t.Errorf("Expected error field, got nil")
	}
	if response["result"] != nil {
		t.Errorf("Expected no result for error, got: %v", response["result"])
	}
}

func TestStringRequestID(t *testing.T) {
	s := makeTestServer(t)

	// Send request with string ID (also JSON-RPC 2.0 compliant)
	request := newRequest("test-123", "health", nil).toJSON(t)
	response := executeRequest(t, s, request)

	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-123",
	}
	if diff := compareJSON(expected, response); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Verify it's a success response (has result, no error)
	if response["result"] == nil {
		t.Errorf("Expected result field, got nil")
	}
	if response["error"] != nil {
		t.Errorf("Expected no error, got: %v", response["error"])
	}
}

func TestUserContacts(t *testing.T) {
	s := makeTestServer(t)

	// Create a user first
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "ContactsUser",
		"token":      "contacts-token",
	}).toJSON(t)
	executeRequest(t, s, addRequest)

	// Try to get contacts (will fail because no WhatsApp session, but tests routing)
	contactsRequest := newRequest("2", "user.contacts", map[string]interface{}{
		"token": "contacts-token",
	}).toJSON(t)
	contactsResponse := executeRequest(t, s, contactsRequest)

	// The routing should work even if the actual operation fails
	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expected, contactsResponse); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Either result or error should be present
	if contactsResponse["result"] == nil && contactsResponse["error"] == nil {
		t.Errorf("Expected either result or error field")
	}
}

func TestGroupList(t *testing.T) {
	s := makeTestServer(t)

	// Create a user first
	addRequest := newRequest("1", "admin.users.add", map[string]interface{}{
		"adminToken": "test-admin-token",
		"name":       "GroupUser",
		"token":      "group-token",
	}).toJSON(t)
	executeRequest(t, s, addRequest)

	// Try to list groups (will fail because no WhatsApp session, but tests routing)
	groupListRequest := newRequest("2", "group.list", map[string]interface{}{
		"token": "group-token",
	}).toJSON(t)
	groupListResponse := executeRequest(t, s, groupListRequest)

	// The routing should work even if the actual operation fails
	expected := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
	}
	if diff := compareJSON(expected, groupListResponse); diff != "" {
		t.Errorf("Response mismatch:\n%s", diff)
	}

	// Either result or error should be present
	if groupListResponse["result"] == nil && groupListResponse["error"] == nil {
		t.Errorf("Expected either result or error field")
	}
}
