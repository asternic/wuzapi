// Package main provides a JSON-RPC 2.0 server over stdin/stdout.
//
// This file implements a stdio-based JSON-RPC 2.0 interface that bridges
// to the existing HTTP API handlers. It enables programmatic access to
// wuzapi functionality through standard input/output, making it suitable
// for use as a subprocess or in headless environments.
//
// The implementation:
//   - Reads newline-delimited JSON-RPC 2.0 requests from stdin
//   - Routes requests to existing HTTP handlers via httptest
//   - Writes JSON-RPC 2.0 responses to stdout
//   - Supports both notification and request/response patterns
//
// JSON-RPC methods map directly to HTTP endpoints (e.g., "user.login"
// maps to POST /user/login). See JSON-RPC-API.md for available methods.
//
// Author: Alvaro Ramirez https://xenodium.com
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"

	"github.com/rs/zerolog/log"
)

// ID represents a JSON-RPC 2.0 request/response identifier
// It can be either a string or number per the spec
type ID struct {
	Num      uint64
	Str      string
	IsString bool // true if ID is a string, false if numeric
	IsSet    bool // true if ID was present in JSON (not omitted)
}

// MarshalJSON implements json.Marshaler
func (id ID) MarshalJSON() ([]byte, error) {
	if !id.IsSet {
		return []byte("null"), nil
	}
	if id.IsString {
		return json.Marshal(id.Str)
	}
	return json.Marshal(id.Num)
}

// UnmarshalJSON implements json.Unmarshaler
func (id *ID) UnmarshalJSON(data []byte) error {
	// Try numeric first
	var num uint64
	if err := json.Unmarshal(data, &num); err == nil {
		*id = ID{Num: num, IsString: false, IsSet: true}
		return nil
	}
	// Fall back to string
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*id = ID{Str: str, IsString: true, IsSet: true}
	return nil
}

// String returns the ID as a string for logging/display
func (id ID) String() string {
	if id.IsString {
		return id.Str
	}
	return fmt.Sprintf("%d", id.Num)
}

// jsonRpcRequest represents an incoming JSON request from stdin
type jsonRpcRequest struct {
	ID     ID                     `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// jsonRpcResponse represents an outgoing JSON response to stdout
// Follows JSON-RPC 2.0 specification
type jsonRpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      ID          `json:"id"`
	Result  *jsonResult `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

// jsonResult wraps the result value so we can distinguish between
// "no result" (nil pointer, omitted) and "null result" (pointer to nil)
type jsonResult struct {
	Value interface{}
}

// MarshalJSON makes jsonResult marshal as its wrapped value
func (r *jsonResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Value)
}

// rpcError represents a JSON-RPC 2.0 error object
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// stdioServer handles stdin/stdout JSON-based API by wrapping HTTP handlers
type stdioServer struct {
	server *server
	stdin  io.Reader
	stdout io.Writer
}

// NewStdioServer creates a new stdio server instance
func NewStdioServer(s *server) *stdioServer {
	return &stdioServer{
		server: s,
		stdin:  os.Stdin,
		stdout: os.Stdout,
	}
}

// newStdioServerWithIO creates a stdio server with custom IO streams (for testing)
func newStdioServerWithIO(s *server, stdin io.Reader, stdout io.Writer) *stdioServer {
	return &stdioServer{
		server: s,
		stdin:  stdin,
		stdout: stdout,
	}
}

func (ss *stdioServer) Start() error {
	log.Info().Msg("Starting stdio mode - reading JSON requests from stdin")

	scanner := bufio.NewScanner(ss.stdin)

	const maxCapacity = 512 * 1024 // 512KB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // Skip empty lines
		}
		ss.handleRequest(line)
	}

	// Scanner stopped, check why
	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Msg("Error reading from stdin")
		return err
	}

	log.Info().Msg("EOF reached on stdin, shutting down")
	return nil
}

func (ss *stdioServer) handleRequest(requestBytes []byte) {
	var req jsonRpcRequest
	if err := json.Unmarshal(requestBytes, &req); err != nil {
		ss.sendError(ID{}, 400, fmt.Sprintf("invalid JSON request: %v", err))
		return
	}
	// ID is required - check if it was present in the JSON
	if !req.ID.IsSet {
		ss.sendError(ID{}, 400, "missing request id")
		return
	}
	if req.Method == "" {
		ss.sendError(req.ID, 400, "missing method")
		return
	}
	log.Info().
		Str("id", req.ID.String()).
		Str("method", req.Method).
		Msg("Processing stdio request")
	ss.routeRequest(&req)
}

// routeRequest dispatches the request to the appropriate HTTP handler
// getUserIdParam extracts and validates userId from request params
func (ss *stdioServer) getUserIdParam(req *jsonRpcRequest) (string, bool) {
	userId, ok := req.Params["userId"].(string)
	if !ok || userId == "" {
		ss.sendError(req.ID, 400, "missing or invalid userId parameter")
		return "", false
	}
	return userId, true
}

func (ss *stdioServer) routeRequest(req *jsonRpcRequest) {
	// Map stdio method to HTTP route and method
	var httpMethod, httpPath string

	switch req.Method {
	case "health":
		httpMethod = "GET"
		httpPath = "/health"

	// Admin user management
	case "admin.users.add":
		httpMethod = "POST"
		httpPath = "/admin/users"
	case "admin.users.list":
		httpMethod = "GET"
		httpPath = "/admin/users"
	case "admin.users.get":
		httpMethod = "GET"
		userId, ok := ss.getUserIdParam(req)
		if !ok {
			// Error sent by getUserIdParam.
			return
		}
		httpPath = "/admin/users/" + userId
	case "admin.users.delete":
		httpMethod = "DELETE"
		userId, ok := ss.getUserIdParam(req)
		if !ok {
			// Error sent by getUserIdParam.
			return
		}
		httpPath = "/admin/users/" + userId
	case "admin.users.edit":
		httpMethod = "PUT"
		userId, ok := ss.getUserIdParam(req)
		if !ok {
			// Error sent by getUserIdParam.
			return
		}
		httpPath = "/admin/users/" + userId
	case "admin.users.delete.full":
		httpMethod = "DELETE"
		userId, ok := ss.getUserIdParam(req)
		if !ok {
			// Error sent by getUserIdParam.
			return
		}
		httpPath = "/admin/users/" + userId + "/full"

	// Session management
	case "session.connect":
		httpMethod = "POST"
		httpPath = "/session/connect"
	case "session.qr":
		httpMethod = "GET"
		httpPath = "/session/qr"
	case "session.status":
		httpMethod = "GET"
		httpPath = "/session/status"
	case "session.disconnect":
		httpMethod = "POST"
		httpPath = "/session/disconnect"
	case "session.logout":
		httpMethod = "POST"
		httpPath = "/session/logout"
	case "session.pairphone":
		httpMethod = "POST"
		httpPath = "/session/pairphone"
	case "session.history":
		httpMethod = "GET"
		httpPath = "/session/history"
	case "session.history.set":
		httpMethod = "POST"
		httpPath = "/session/history"
	case "session.proxy":
		httpMethod = "POST"
		httpPath = "/session/proxy"
	case "session.hmac.config":
		httpMethod = "POST"
		httpPath = "/session/hmac/config"
	case "session.hmac.config.get":
		httpMethod = "GET"
		httpPath = "/session/hmac/config"
	case "session.hmac.config.delete":
		httpMethod = "DELETE"
		httpPath = "/session/hmac/config"

	// Messaging
	case "chat.send.text":
		httpMethod = "POST"
		httpPath = "/chat/send/text"
	case "chat.send.image":
		httpMethod = "POST"
		httpPath = "/chat/send/image"
	case "chat.send.video":
		httpMethod = "POST"
		httpPath = "/chat/send/video"
	case "chat.send.document":
		httpMethod = "POST"
		httpPath = "/chat/send/document"
	case "chat.send.audio":
		httpMethod = "POST"
		httpPath = "/chat/send/audio"
	case "chat.send.sticker":
		httpMethod = "POST"
		httpPath = "/chat/send/sticker"
	case "chat.send.location":
		httpMethod = "POST"
		httpPath = "/chat/send/location"
	case "chat.send.contact":
		httpMethod = "POST"
		httpPath = "/chat/send/contact"
	case "chat.send.poll":
		httpMethod = "POST"
		httpPath = "/chat/send/poll"
	case "chat.send.buttons":
		httpMethod = "POST"
		httpPath = "/chat/send/buttons"
	case "chat.send.list":
		httpMethod = "POST"
		httpPath = "/chat/send/list"
	case "chat.send.edit":
		httpMethod = "POST"
		httpPath = "/chat/send/edit"
	case "chat.delete":
		httpMethod = "POST"
		httpPath = "/chat/delete"
	case "chat.react":
		httpMethod = "POST"
		httpPath = "/chat/react"
	case "chat.archive":
		httpMethod = "POST"
		httpPath = "/chat/archive"
	case "chat.presence":
		httpMethod = "POST"
		httpPath = "/chat/presence"
	case "chat.markread":
		httpMethod = "POST"
		httpPath = "/chat/markread"
	case "chat.request-unavailable-message":
		httpMethod = "POST"
		httpPath = "/chat/request-unavailable-message"
	case "chat.download.image":
		httpMethod = "POST"
		httpPath = "/chat/downloadimage"
	case "chat.download.video":
		httpMethod = "POST"
		httpPath = "/chat/downloadvideo"
	case "chat.download.audio":
		httpMethod = "POST"
		httpPath = "/chat/downloadaudio"
	case "chat.download.document":
		httpMethod = "POST"
		httpPath = "/chat/downloaddocument"
	case "chat.history":
		httpMethod = "GET"
		chatJID, ok := req.Params["chat_jid"].(string)
		if !ok || chatJID == "" {
			ss.sendError(req.ID, 400, "missing or invalid chat_jid parameter")
			return
		}
		httpPath = "/chat/history?chat_jid=" + chatJID
		// Add optional limit parameter
		if limit, ok := req.Params["limit"].(float64); ok {
			httpPath += fmt.Sprintf("&limit=%d", int(limit))
		}

	// User info
	case "user.contacts":
		httpMethod = "GET"
		httpPath = "/user/contacts"
	case "user.presence":
		httpMethod = "POST"
		httpPath = "/user/presence"
	case "user.info":
		httpMethod = "POST"
		httpPath = "/user/info"
	case "user.check":
		httpMethod = "POST"
		httpPath = "/user/check"
	case "user.avatar":
		httpMethod = "POST"
		httpPath = "/user/avatar"
	case "user.lid":
		httpMethod = "GET"
		jid, ok := req.Params["jid"].(string)
		if !ok || jid == "" {
			ss.sendError(req.ID, 400, "missing or invalid jid parameter")
			return
		}
		httpPath = "/user/lid/" + jid

	// Status
	case "status.set.text":
		httpMethod = "POST"
		httpPath = "/status/set/text"

	// Calls
	case "call.reject":
		httpMethod = "POST"
		httpPath = "/call/reject"

	// Group management
	case "group.list":
		httpMethod = "GET"
		httpPath = "/group/list"
	case "group.create":
		httpMethod = "POST"
		httpPath = "/group/create"
	case "group.info":
		httpMethod = "GET"
		httpPath = "/group/info"
	case "group.invitelink":
		httpMethod = "GET"
		httpPath = "/group/invitelink"
	case "group.photo":
		httpMethod = "POST"
		httpPath = "/group/photo"
	case "group.photo.remove":
		httpMethod = "POST"
		httpPath = "/group/photo/remove"
	case "group.leave":
		httpMethod = "POST"
		httpPath = "/group/leave"
	case "group.name":
		httpMethod = "POST"
		httpPath = "/group/name"
	case "group.topic":
		httpMethod = "POST"
		httpPath = "/group/topic"
	case "group.announce":
		httpMethod = "POST"
		httpPath = "/group/announce"
	case "group.locked":
		httpMethod = "POST"
		httpPath = "/group/locked"
	case "group.ephemeral":
		httpMethod = "POST"
		httpPath = "/group/ephemeral"
	case "group.join":
		httpMethod = "POST"
		httpPath = "/group/join"
	case "group.inviteinfo":
		httpMethod = "POST"
		httpPath = "/group/inviteinfo"
	case "group.updateparticipants":
		httpMethod = "POST"
		httpPath = "/group/updateparticipants"

	// Newsletter
	case "newsletter.list":
		httpMethod = "GET"
		httpPath = "/newsletter/list"

	// Webhook management
	case "webhook.get":
		httpMethod = "GET"
		httpPath = "/webhook"
	case "webhook.set":
		httpMethod = "POST"
		httpPath = "/webhook"
	case "webhook.update":
		httpMethod = "PUT"
		httpPath = "/webhook"
	case "webhook.delete":
		httpMethod = "DELETE"
		httpPath = "/webhook"

	default:
		ss.sendError(req.ID, 404, fmt.Sprintf("unknown method: %s", req.Method))
		return
	}
	ss.executeHTTPHandler(req, httpMethod, httpPath)
}

// executeHTTPHandler wraps the existing HTTP handler and adapts it for stdio
func (ss *stdioServer) executeHTTPHandler(req *jsonRpcRequest, httpMethod, httpPath string) {
	// Create a mock HTTP request
	var body io.Reader
	if req.Params != nil && len(req.Params) > 0 {
		jsonParams, err := json.Marshal(req.Params)
		if err != nil {
			ss.sendError(req.ID, 400, fmt.Sprintf("invalid params: %v", err))
			return
		}
		body = bytes.NewReader(jsonParams)
	}

	httpReq := httptest.NewRequest(httpMethod, httpPath, body)
	httpReq.Header.Set("Content-Type", "application/json")

	// Set user token header (for user authentication)
	if token, ok := req.Params["token"].(string); ok {
		httpReq.Header.Set("token", token)
	}
	// Set admin token header (for admin authentication)
	if adminToken, ok := req.Params["adminToken"].(string); ok {
		httpReq.Header.Set("Authorization", adminToken)
	}

	recorder := httptest.NewRecorder()
	ss.server.router.ServeHTTP(recorder, httpReq)
	ss.convertHTTPResponse(req.ID, recorder)
}

// convertHTTPResponse converts an HTTP response to a stdio response
func (ss *stdioServer) convertHTTPResponse(requestID ID, recorder *httptest.ResponseRecorder) {
	statusCode := recorder.Code
	responseBody := recorder.Body.Bytes()

	var responseData interface{}
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &responseData); err != nil {
			// If it's not JSON, just use the raw string
			responseData = string(responseBody)
		}
	}

	success := statusCode >= 200 && statusCode < 300

	if respMap, ok := responseData.(map[string]interface{}); ok {
		// If it's already in wuzapi format, extract the data/error
		if data, hasData := respMap["data"]; hasData {
			ss.sendSuccess(requestID, statusCode, data)
			return
		}
		if errMsg, hasError := respMap["error"]; hasError {
			if errStr, ok := errMsg.(string); ok {
				ss.sendError(requestID, statusCode, errStr)
				return
			}
		}
		ss.sendSuccess(requestID, statusCode, respMap)
		return
	}

	// For non-JSON or simple responses
	if success {
		ss.sendSuccess(requestID, statusCode, responseData)
	} else {
		errorMsg := "request failed"
		if str, ok := responseData.(string); ok && str != "" {
			errorMsg = str
		}
		ss.sendError(requestID, statusCode, errorMsg)
	}
}

func (ss *stdioServer) sendSuccess(id ID, code int, data interface{}) {
	response := jsonRpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  &jsonResult{Value: data},
	}
	ss.writeResponse(response)
}

func (ss *stdioServer) sendError(id ID, code int, errorMsg string) {
	response := jsonRpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: errorMsg,
		},
	}
	ss.writeResponse(response)
}

func (ss *stdioServer) writeResponse(response jsonRpcResponse) {
	// Marshalled response as single line
	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal response")
		fallback := jsonRpcResponse{
			JSONRPC: "2.0",
			ID:      response.ID,
			Error: &rpcError{
				Code:    -32603,
				Message: "Internal error: failed to marshal response",
			},
		}
		responseBytes, err = json.Marshal(fallback)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal fallback response")
			return
		}
	}

	// Write to stdout with newline
	fmt.Fprintf(ss.stdout, "%s\n", string(responseBytes))

	// Log with appropriate fields based on response type
	logEvent := log.Debug().Str("id", response.ID.String())
	if response.Error != nil {
		logEvent.Bool("success", false).Int("code", response.Error.Code).Str("error", response.Error.Message)
	} else {
		logEvent.Bool("success", true)
	}
	logEvent.Msg("Sent stdio response")
}

// jsonRpcNotification represents a one-way notification (no id, no response expected)
// Follows JSON-RPC 2.0 specification
type jsonRpcNotification struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// SendNotification sends a JSON-RPC notification to stdout (webhooks in stdio mode)
// This is thread-safe - os.Stdout writes are atomic at the OS level
func (s *server) SendNotification(method string, params map[string]interface{}) {
	if s.mode != Stdio {
		return
	}

	notification := jsonRpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	notificationBytes, err := json.Marshal(notification)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal notification")
		return
	}

	fmt.Fprintf(os.Stdout, "%s\n", string(notificationBytes))

	log.Debug().
		Str("method", method).
		Msg("Sent stdio notification")
}
