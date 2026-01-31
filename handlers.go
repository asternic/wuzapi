package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type Values struct {
	m map[string]string
}

func (v Values) Get(key string) string {
	return v.m[key]
}

func (s *server) GetHealth() http.HandlerFunc {
	type HealthResponse struct {
		Status            string                 `json:"status"`
		Timestamp         string                 `json:"timestamp"`
		Uptime            string                 `json:"uptime"`
		ActiveConnections int                    `json:"active_connections"`
		TotalUsers        int                    `json:"total_users"`
		ConnectedUsers    int                    `json:"connected_users"`
		LoggedInUsers     int                    `json:"logged_in_users"`
		MemoryStats       map[string]interface{} `json:"memory_stats"`
		GoRoutines        int                    `json:"goroutines"`
		Version           string                 `json:"version,omitempty"`
	}

	startTime := time.Now()

	return func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime)

		var totalUsers int
		rows, err := s.db.Query("SELECT COUNT(*) FROM users")
		if err == nil {
			defer rows.Close()
			if rows.Next() {
				rows.Scan(&totalUsers)
			}
		}

		clientManager.RLock()
		activeConnections := len(clientManager.whatsmeowClients)
		connectedUsers := 0
		loggedInUsers := 0

		for _, client := range clientManager.whatsmeowClients {
			if client != nil {
				if client.IsConnected() {
					connectedUsers++
				}
				if client.IsLoggedIn() {
					loggedInUsers++
				}
			}
		}
		clientManager.RUnlock()

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		memoryStats := map[string]interface{}{
			"alloc_mb":       memStats.Alloc / 1024 / 1024,
			"total_alloc_mb": memStats.TotalAlloc / 1024 / 1024,
			"sys_mb":         memStats.Sys / 1024 / 1024,
			"num_gc":         memStats.NumGC,
		}

		response := HealthResponse{
			Status:            "ok",
			Timestamp:         time.Now().UTC().Format(time.RFC3339),
			Uptime:            uptime.String(),
			ActiveConnections: activeConnections,
			TotalUsers:        totalUsers,
			ConnectedUsers:    connectedUsers,
			LoggedInUsers:     loggedInUsers,
			MemoryStats:       memoryStats,
			GoRoutines:        runtime.NumGoroutine(),
			Version:           version,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error().Err(err).Msg("Failed to write health check response")
		}
	}
}

// messageTypes moved to constants.go as supportedEventTypes

func (s *server) authadmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != *adminToken {
			s.Respond(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) authalice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var ctx context.Context
		txtid := ""
		name := ""
		webhook := ""
		jid := ""
		events := ""
		proxy_url := ""
		qrcode := ""
		var hasHmac bool // ← Nova variável para status HMAC

		// Get token from headers or uri parameters
		token := r.Header.Get("token")
		if token == "" {
			token = strings.Join(r.URL.Query()["token"], "")
		}

		myuserinfo, found := userinfocache.Get(token)
		if !found {
			log.Info().Msg("Looking for user information in DB")
			// Checks DB from matching user and store user values in context
			rows, err := s.db.Query("SELECT id,name,webhook,jid,events,proxy_url,qrcode,history,hmac_key IS NOT NULL AND length(hmac_key) > 0 FROM users WHERE token=$1 LIMIT 1", token)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
				return
			}
			defer rows.Close()
			var history sql.NullInt64
			for rows.Next() {
				err = rows.Scan(&txtid, &name, &webhook, &jid, &events, &proxy_url, &qrcode, &history, &hasHmac)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, err)
					return
				}
				historyStr := "0"
				if history.Valid {
					historyStr = fmt.Sprintf("%d", history.Int64)
				}

				// Debug logging for history value
				log.Debug().Str("userId", txtid).Bool("historyValid", history.Valid).Int64("historyValue", history.Int64).Str("historyStr", historyStr).Msg("User authentication - history debug")

				v := Values{map[string]string{
					"Id":      txtid,
					"Name":    name,
					"Jid":     jid,
					"Webhook": webhook,
					"Token":   token,
					"Proxy":   proxy_url,
					"Events":  events,
					"Qrcode":  qrcode,
					"History": historyStr,
					"HasHmac": strconv.FormatBool(hasHmac),
				}}

				userinfocache.Set(token, v, cache.NoExpiration)
				log.Info().Str("name", name).Msg("User info name from DB")
				ctx = context.WithValue(r.Context(), "userinfo", v)
			}
		} else {
			ctx = context.WithValue(r.Context(), "userinfo", myuserinfo)
			log.Info().Str("name", myuserinfo.(Values).Get("name")).Msg("User info name from Cache")
			txtid = myuserinfo.(Values).Get("Id")
		}

		if txtid == "" {
			s.Respond(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Connects to Whatsapp Servers
func (s *server) Connect() http.HandlerFunc {

	type connectStruct struct {
		Subscribe []string
		Immediate bool
	}

	return func(w http.ResponseWriter, r *http.Request) {

		webhook := r.Context().Value("userinfo").(Values).Get("Webhook")
		jid := r.Context().Value("userinfo").(Values).Get("Jid")
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")
		eventstring := ""

		// Decodes request BODY looking for events to subscribe
		decoder := json.NewDecoder(r.Body)
		var t connectStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if clientManager.GetWhatsmeowClient(txtid) != nil {
			isConnected := clientManager.GetWhatsmeowClient(txtid).IsConnected()
			if isConnected == true {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("already connected"))
				return
			}
		}

		var subscribedEvents []string
		if len(t.Subscribe) < 1 {
			if !Find(subscribedEvents, "") {
				subscribedEvents = append(subscribedEvents, "")
			}
		} else {
			for _, arg := range t.Subscribe {
				if !Find(supportedEventTypes, arg) {
					log.Warn().Str("Type", arg).Msg("Event type discarded")
					continue
				}
				if !Find(subscribedEvents, arg) {
					subscribedEvents = append(subscribedEvents, arg)
				}
			}
		}
		eventstring = strings.Join(subscribedEvents, ",")
		_, err = s.db.Exec("UPDATE users SET events=$1 WHERE id=$2", eventstring, txtid)
		if err != nil {
			log.Warn().Msg("Could not set events in users table")
		}
		log.Info().Str("events", eventstring).Msg("Setting subscribed events")
		v := updateUserInfo(r.Context().Value("userinfo"), "Events", eventstring)
		userinfocache.Set(token, v, cache.NoExpiration)

		log.Info().Str("jid", jid).Msg("Attempt to connect")
		killchannel[txtid] = make(chan bool, 1)
		go s.startClient(txtid, jid, token, subscribedEvents)

		if t.Immediate == false {
			log.Warn().Msg("Waiting 10 seconds")
			time.Sleep(10000 * time.Millisecond)

			if clientManager.GetWhatsmeowClient(txtid) != nil {
				if !clientManager.GetWhatsmeowClient(txtid).IsConnected() {
					s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to Connect"))
					return
				}
			} else {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to connect"))
				return
			}
		}

		response := map[string]interface{}{"webhook": webhook, "jid": jid, "events": eventstring, "details": "Connected!"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
			return
		}
	}
}

// Disconnects from Whatsapp websocket, does not log out device
func (s *server) Disconnect() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		jid := r.Context().Value("userinfo").(Values).Get("Jid")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}
		if clientManager.GetWhatsmeowClient(txtid).IsConnected() == true {
			//if clientManager.GetWhatsmeowClient(txtid).IsLoggedIn() == true {
			log.Info().Str("jid", jid).Msg("Disconnection successfull")
			_, err := s.db.Exec("UPDATE users SET connected=0,events=$1 WHERE id=$2", "", txtid)
			if err != nil {
				log.Warn().Str("txtid", txtid).Msg("Could not set events in users table")
			}
			log.Info().Str("txtid", txtid).Msg("Update DB on disconnection")
			v := updateUserInfo(r.Context().Value("userinfo"), "Events", "")
			userinfocache.Set(token, v, cache.NoExpiration)

			response := map[string]interface{}{"Details": "Disconnected"}
			responseJson, err := json.Marshal(response)

			clientManager.DeleteWhatsmeowClient(txtid)
			select {
			case killchannel[txtid] <- true:
			default:
			}

			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
			} else {
				s.Respond(w, r, http.StatusOK, string(responseJson))
			}
			return
			//} else {
			//	log.Warn().Str("jid", jid).Msg("Ignoring disconnect as it was not connected")
			//	s.Respond(w, r, http.StatusInternalServerError, errors.New("Cannot disconnect because it is not logged in"))
			//	return
			//}
		} else {
			log.Warn().Str("jid", jid).Msg("Ignoring disconnect as it was not connected")
			s.Respond(w, r, http.StatusInternalServerError, errors.New("cannot disconnect because it is not logged in"))
			return
		}
	}
}

// Gets WebHook
func (s *server) GetWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		webhook := ""
		events := ""
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		rows, err := s.db.Query("SELECT webhook,events FROM users WHERE id=$1 LIMIT 1", txtid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not get webhook: %v", err)))
			return
		}
		defer rows.Close()
		for rows.Next() {
			err = rows.Scan(&webhook, &events)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not get webhook: %s", fmt.Sprintf("%s", err))))
				return
			}
		}
		err = rows.Err()
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not get webhook: %s", fmt.Sprintf("%s", err))))
			return
		}

		eventarray := strings.Split(events, ",")

		response := map[string]interface{}{"webhook": webhook, "subscribe": eventarray}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// DeleteWebhook removes the webhook and clears events for a user
func (s *server) DeleteWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		// Update the database to remove the webhook and clear events
		_, err := s.db.Exec("UPDATE users SET webhook='', events='' WHERE id=$1", txtid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not delete webhook: %v", err)))
			return
		}

		// Update the user info cache
		v := updateUserInfo(r.Context().Value("userinfo"), "Webhook", "")
		v = updateUserInfo(v, "Events", "")
		userinfocache.Set(token, v, cache.NoExpiration)

		response := map[string]interface{}{"Details": "Webhook and events deleted successfully"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// UpdateWebhook updates the webhook URL and events for a user
func (s *server) UpdateWebhook() http.HandlerFunc {
	type updateWebhookStruct struct {
		WebhookURL string   `json:"webhook"`
		Events     []string `json:"events,omitempty"`
		Active     bool     `json:"active"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		decoder := json.NewDecoder(r.Body)
		var t updateWebhookStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		webhook := t.WebhookURL

		var eventstring string
		var validEvents []string
		for _, event := range t.Events {
			if !Find(supportedEventTypes, event) {
				log.Warn().Str("Type", event).Msg("Event type discarded")
				continue
			}
			validEvents = append(validEvents, event)
		}
		eventstring = strings.Join(validEvents, ",")
		if eventstring == "," || eventstring == "" {
			eventstring = ""
		}

		if !t.Active {
			webhook = ""
			eventstring = ""
		}

		if len(t.Events) > 0 {
			_, err = s.db.Exec("UPDATE users SET webhook=$1, events=$2 WHERE id=$3", webhook, eventstring, txtid)

			// Update MyClient if connected - integrated UpdateEvents functionality
			if len(validEvents) > 0 {
				clientManager.UpdateMyClientSubscriptions(txtid, validEvents)
				log.Info().Strs("events", validEvents).Str("user", txtid).Msg("Updated event subscriptions")
			}
		} else {
			// Update only webhook
			_, err = s.db.Exec("UPDATE users SET webhook=$1 WHERE id=$2", webhook, txtid)
		}

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not update webhook: %v", err)))
			return
		}

		v := updateUserInfo(r.Context().Value("userinfo"), "Webhook", webhook)
		v = updateUserInfo(v, "Events", eventstring)
		userinfocache.Set(token, v, cache.NoExpiration)

		response := map[string]interface{}{"webhook": webhook, "events": validEvents, "active": t.Active}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// SetWebhook sets the webhook URL and events for a user
func (s *server) SetWebhook() http.HandlerFunc {
	type webhookStruct struct {
		WebhookURL string   `json:"webhookurl"`
		Events     []string `json:"events,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		decoder := json.NewDecoder(r.Body)
		var t webhookStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		webhook := t.WebhookURL

		// If events are provided, validate them
		var eventstring string
		if len(t.Events) > 0 {
			var validEvents []string
			for _, event := range t.Events {
				if !Find(supportedEventTypes, event) {
					log.Warn().Str("Type", event).Msg("Event type discarded")
					continue
				}
				validEvents = append(validEvents, event)
			}
			eventstring = strings.Join(validEvents, ",")
			if eventstring == "," || eventstring == "" {
				eventstring = ""
			}

			// Update both webhook and events
			_, err = s.db.Exec("UPDATE users SET webhook=$1, events=$2 WHERE id=$3", webhook, eventstring, txtid)

			// Update MyClient if connected - integrated UpdateEvents functionality
			if len(validEvents) > 0 {
				clientManager.UpdateMyClientSubscriptions(txtid, validEvents)
				log.Info().Strs("events", validEvents).Str("user", txtid).Msg("Updated event subscriptions")
			}
		} else {
			// Update only webhook
			_, err = s.db.Exec("UPDATE users SET webhook=$1 WHERE id=$2", webhook, txtid)
		}

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not set webhook: %v", err)))
			return
		}

		v := updateUserInfo(r.Context().Value("userinfo"), "Webhook", webhook)
		v = updateUserInfo(v, "Events", eventstring)
		userinfocache.Set(token, v, cache.NoExpiration)

		response := map[string]interface{}{"webhook": webhook}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Gets QR code encoded in Base64
func (s *server) GetQR() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		code := ""

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		} else {
			if clientManager.GetWhatsmeowClient(txtid).IsConnected() == false {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("not connected"))
				return
			}
			rows, err := s.db.Query("SELECT qrcode AS code FROM users WHERE id=$1 LIMIT 1", txtid)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				err = rows.Scan(&code)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, err)
					return
				}
			}
			err = rows.Err()
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
				return
			}
			if clientManager.GetWhatsmeowClient(txtid).IsLoggedIn() == true {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("already logged in"))
				return
			}
		}

		log.Info().Str("instance", txtid).Str("qrcode", code).Msg("Get QR successful")
		response := map[string]interface{}{"QRCode": fmt.Sprintf("%s", code)}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Logs out device from Whatsapp (requires to scan QR next time)
func (s *server) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		jid := r.Context().Value("userinfo").(Values).Get("Jid")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		} else {
			if clientManager.GetWhatsmeowClient(txtid).IsLoggedIn() == true &&
				clientManager.GetWhatsmeowClient(txtid).IsConnected() == true {
				err := clientManager.GetWhatsmeowClient(txtid).Logout(context.Background())
				if err != nil {
					log.Error().Str("jid", jid).Msg("Could not perform logout")
					s.Respond(w, r, http.StatusInternalServerError, errors.New("could not perform logout"))
					return
				} else {
					log.Info().Str("jid", jid).Msg("Logged out")
					clientManager.DeleteWhatsmeowClient(txtid)
					select {
					case killchannel[txtid] <- true:
					default:
					}
				}
			} else {
				if clientManager.GetWhatsmeowClient(txtid).IsConnected() == true {
					log.Warn().Str("jid", jid).Msg("Ignoring logout as it was not logged in")
					s.Respond(w, r, http.StatusInternalServerError, errors.New("could not logout as it was not logged in"))
					return
				} else {
					log.Warn().Str("jid", jid).Msg("Ignoring logout as it was not connected")
					s.Respond(w, r, http.StatusInternalServerError, errors.New("could not disconnect as it was not connected"))
					return
				}
			}
		}

		response := map[string]interface{}{"Details": "Logged out"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Pair by Phone. Retrieves the code to pair by phone number instead of QR
func (s *server) PairPhone() http.HandlerFunc {

	type pairStruct struct {
		Phone string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t pairStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		isLoggedIn := clientManager.GetWhatsmeowClient(txtid).IsLoggedIn()
		if isLoggedIn {
			log.Error().Msg(fmt.Sprintf("%s", "already paired"))
			s.Respond(w, r, http.StatusBadRequest, errors.New("already paired"))
			return
		}

		linkingCode, err := clientManager.GetWhatsmeowClient(txtid).PairPhone(context.Background(), t.Phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		response := map[string]interface{}{"LinkingCode": linkingCode}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Gets Connected and LoggedIn Status
func (s *server) GetStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userInfo := r.Context().Value("userinfo").(Values)

		log.Info().
			Str("Id", userInfo.Get("Id")).
			Str("Jid", userInfo.Get("Jid")).
			Str("Name", userInfo.Get("Name")).
			Str("Webhook", userInfo.Get("Webhook")).
			Str("Token", userInfo.Get("Token")).
			Str("Events", userInfo.Get("Events")).
			Str("Proxy", userInfo.Get("Proxy")).
			Str("History", userInfo.Get("History")).
			Str("HasHmac", userInfo.Get("HasHmac")).
			Msg("User info values")

		txtid := userInfo.Get("Id")

		isConnected := clientManager.GetWhatsmeowClient(txtid).IsConnected()
		isLoggedIn := clientManager.GetWhatsmeowClient(txtid).IsLoggedIn()

		var proxyURL string
		s.db.QueryRow("SELECT proxy_url FROM users WHERE id = $1", txtid).Scan(&proxyURL)
		proxyConfig := map[string]interface{}{
			"enabled":   proxyURL != "",
			"proxy_url": proxyURL,
		}

		var s3Enabled bool
		var s3Endpoint, s3Region, s3Bucket, s3PublicURL, s3MediaDelivery string
		var s3PathStyle bool
		var s3RetentionDays int

		// Start with safe defaults so the field is always present in the response
		s3Config := map[string]interface{}{
			"enabled":        false,
			"endpoint":       "",
			"region":         "",
			"bucket":         "",
			"access_key":     "***",
			"path_style":     false,
			"public_url":     "",
			"media_delivery": "",
			"retention_days": 0,
		}
		err := s.db.QueryRow(`SELECT COALESCE(s3_enabled, false), COALESCE(s3_endpoint, ''), COALESCE(s3_region, ''), COALESCE(s3_bucket, ''), COALESCE(s3_path_style, false), COALESCE(s3_public_url, ''), COALESCE(media_delivery, ''), COALESCE(s3_retention_days, 0) FROM users WHERE id = $1`, txtid).Scan(&s3Enabled, &s3Endpoint, &s3Region, &s3Bucket, &s3PathStyle, &s3PublicURL, &s3MediaDelivery, &s3RetentionDays)

		if err == nil {
			// Overwrite defaults with actual values if the query succeeded
			s3Config["enabled"] = s3Enabled
			s3Config["endpoint"] = s3Endpoint
			s3Config["region"] = s3Region
			s3Config["bucket"] = s3Bucket
			s3Config["path_style"] = s3PathStyle
			s3Config["public_url"] = s3PublicURL
			s3Config["media_delivery"] = s3MediaDelivery
			s3Config["retention_days"] = s3RetentionDays
		} else {
			if err != sql.ErrNoRows {
				log.Warn().Err(err).Str("user_id", txtid).Msg("Failed to query S3 config for user")
			}
		}

		var hmacKey []byte
		err = s.db.QueryRow("SELECT hmac_key FROM users WHERE id = $1", txtid).Scan(&hmacKey)
		if err != nil && err != sql.ErrNoRows {
			log.Error().Err(err).Str("userID", txtid).Msg("Failed to query HMAC key")
		}
		hmacConfigured := len(hmacKey) > 0

		response := map[string]interface{}{
			"id":              txtid,
			"name":            userInfo.Get("Name"),
			"connected":       isConnected,
			"loggedIn":        isLoggedIn,
			"token":           userInfo.Get("Token"),
			"jid":             userInfo.Get("Jid"),
			"webhook":         userInfo.Get("Webhook"),
			"events":          userInfo.Get("Events"),
			"proxy_url":       userInfo.Get("Proxy"),
			"qrcode":          userInfo.Get("Qrcode"),
			"history":         userInfo.Get("History"),
			"proxy_config":    proxyConfig,
			"s3_config":       s3Config,
			"hmac_configured": hmacConfigured,
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends a document/attachment message
func (s *server) SendDocument() http.HandlerFunc {

	type documentStruct struct {
		Caption     string
		Phone       string
		Document    string
		FileName    string
		Id          string
		MimeType    string
		ContextInfo waE2E.ContextInfo
		QuotedMessage *waE2E.Message `json:"QuotedMessage,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t documentStruct
		var err error
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Document == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Document in Payload"))
			return
		}

		if t.FileName == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing FileName in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte

		if t.Document[0:29] == "data:application/octet-stream" {
			var dataURL, err = dataurl.DecodeString(t.Document)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
				uploaded, err = clientManager.GetWhatsmeowClient(txtid).Upload(context.Background(), filedata, whatsmeow.MediaDocument)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to upload file: %v", err)))
					return
				}
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("document data should start with \"data:application/octet-stream;base64,\""))
			return
		}

		msg := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			URL:        proto.String(uploaded.URL),
			FileName:   &t.FileName,
			DirectPath: proto.String(uploaded.DirectPath),
			MediaKey:   uploaded.MediaKey,
			Mimetype: proto.String(func() string {
				if t.MimeType != "" {
					return t.MimeType
				}
				return http.DetectContentType(filedata)
			}()),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			Caption:       proto.String(t.Caption),
		}}

		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, it uses the old logic (empty message).
				qm = &waE2E.Message{Conversation: proto.String("")}
			}

			if msg.DocumentMessage.ContextInfo == nil {
				msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: qm,
				}
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.DocumentMessage.ContextInfo == nil {
				msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.DocumentMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.DocumentMessage.ContextInfo == nil {
				msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.DocumentMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "document", t.Caption, "", historyLimit)

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends an audio message
func (s *server) SendAudio() http.HandlerFunc {

	type audioStruct struct {
		Phone       string
		Audio       string
		Caption     string
		Id          string
		PTT         *bool  `json:"ptt,omitempty"`
		MimeType    string `json:"mimetype,omitempty"`
		Seconds     uint32
		Waveform    []byte
		ContextInfo waE2E.ContextInfo
		QuotedMessage *waE2E.Message `json:"QuotedMessage,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t audioStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Audio == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Audio in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte

		if strings.HasPrefix(t.Audio, "data:audio/") {
			var dataURL, err = dataurl.DecodeString(t.Audio)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
				uploaded, err = clientManager.GetWhatsmeowClient(txtid).Upload(context.Background(), filedata, whatsmeow.MediaAudio)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to upload file: %v", err)))
					return
				}
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("audio data should start with \"data:audio/\""))
			return
		}

		// Configure PTT (Push to Talk) - default is true, setting it to false is a breaking change
		ptt := true
		if t.PTT != nil {
			ptt = *t.PTT
		}

		// Configure MIME type
		var mime string
		if t.MimeType != "" {
			mime = t.MimeType
		} else {
			// Default MIME types based on PTT setting
			if ptt {
				mime = "audio/ogg; codecs=opus"
			} else {
				mime = "audio/mpeg"
			}
		}

		msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mime,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			PTT:           &ptt,
			Seconds:       proto.Uint32(t.Seconds),
			Waveform:      t.Waveform,
		}}

		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, it uses the old logic (empty message).
				qm = &waE2E.Message{Conversation: proto.String("")}
			}

			if msg.AudioMessage.ContextInfo == nil {
				msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: qm,
				}
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.AudioMessage.ContextInfo == nil {
				msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.AudioMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.AudioMessage.ContextInfo == nil {
				msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.AudioMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "audio", "", "", historyLimit)

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends an Image message
func (s *server) SendImage() http.HandlerFunc {

	type imageStruct struct {
		Phone         string
		Image         string
		Caption       string
		Id            string
		MimeType      string
		ContextInfo   waE2E.ContextInfo
		QuotedMessage *waE2E.Message `json:"QuotedMessage,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t imageStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Image == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Image in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte
		var thumbnailBytes []byte

		if len(t.Image) >= 10 && t.Image[0:10] == "data:image" {
			var dataURL, err = dataurl.DecodeString(t.Image)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
			}
		} else if isHTTPURL(t.Image) {
			data, ct, err := fetchURLBytes(r.Context(), t.Image, openGraphImageMaxBytes)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New(fmt.Sprintf("failed to fetch image from url: %v", err)))
				return
			}
			mimeType := ct
			if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
				mimeType = "image/jpeg"
			}
			imgDataURL := dataurl.New(data, mimeType)
			parsed, err := dataurl.DecodeString(imgDataURL.String())
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("could not re-encode image to base64"))
				return
			}
			filedata = parsed.Data
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Image data should start with \"data:image/png;base64,\""))
			return
		}

		uploaded, err = clientManager.GetWhatsmeowClient(txtid).Upload(context.Background(), filedata, whatsmeow.MediaImage)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to upload file: %v", err)))
			return
		}

		// decode jpeg into image.Image
		reader := bytes.NewReader(filedata)
		img, _, err := image.Decode(reader)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not decode image for thumbnail preparation: %v", err)))
			return
		}

		// resize to width 72 using Lanczos resampling and preserve aspect ratio
		m := resize.Thumbnail(72, 72, img, resize.Lanczos3)

		tmpFile, err := os.CreateTemp("", "resized-*.jpg")
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not create temp file for thumbnail: %v", err)))
			return
		}
		defer tmpFile.Close()

		// write new image to file
		if err := jpeg.Encode(tmpFile, m, nil); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to encode jpeg: %v", err)))
			return
		}

		thumbnailBytes, err = os.ReadFile(tmpFile.Name())
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to read %s: %v", tmpFile.Name(), err)))
			return
		}

		msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption:    proto.String(t.Caption),
			URL:        proto.String(uploaded.URL),
			DirectPath: proto.String(uploaded.DirectPath),
			MediaKey:   uploaded.MediaKey,
			Mimetype: proto.String(func() string {
				if t.MimeType != "" {
					return t.MimeType
				}
				return http.DetectContentType(filedata)
			}()),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			JPEGThumbnail: thumbnailBytes,
		}}

		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, it uses the old logic (empty message).
				qm = &waE2E.Message{Conversation: proto.String("")}
			}

			if msg.ImageMessage.ContextInfo == nil {
				msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: qm,
				}
			}
		}

		if t.ContextInfo.MentionedJID != nil {
			if msg.ImageMessage.ContextInfo == nil {
				msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.ImageMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.ImageMessage.ContextInfo == nil {
				msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.ImageMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "image", t.Caption, "", historyLimit)

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends Sticker message
func (s *server) SendSticker() http.HandlerFunc {

	type stickerStruct struct {
		Phone         string
		Sticker       string
		Id            string
		PngThumbnail  []byte
		MimeType      string
		PackId        string
		PackName      string
		PackPublisher string
		Emojis        []string
		ContextInfo   waE2E.ContextInfo
		QuotedMessage *waE2E.Message `json:"QuotedMessage,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t stickerStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Sticker == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Sticker in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		processedData, detectedMimeType, err := processStickerData(
			t.Sticker,
			t.MimeType,
			t.PackId,
			t.PackName,
			t.PackPublisher,
			t.Emojis,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to process sticker data")
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "failed to convert") {
				status = http.StatusInternalServerError
			}
			s.Respond(w, r, status, errors.New(err.Error()))
			return
		}

		uploaded, err := clientManager.GetWhatsmeowClient(txtid).Upload(context.Background(), processedData, whatsmeow.MediaImage)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to upload file: %v", err)))
			return
		}

		msg := &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(detectedMimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(processedData))),
			PngThumbnail:  t.PngThumbnail,
		}}

		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, it uses the old logic (empty message).
				qm = &waE2E.Message{Conversation: proto.String("")}
			}

			if msg.StickerMessage.ContextInfo == nil {
				msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: qm,
				}
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.StickerMessage.ContextInfo == nil {
				msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.StickerMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.StickerMessage.ContextInfo == nil {
				msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.StickerMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "sticker", "", "", historyLimit)

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends Video message
func (s *server) SendVideo() http.HandlerFunc {

	type imageStruct struct {
		Phone         string
		Video         string
		Caption       string
		Id            string
		JPEGThumbnail []byte
		MimeType      string
		ContextInfo   waE2E.ContextInfo
		QuotedMessage *waE2E.Message `json:"QuotedMessage,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t imageStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Video == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Video in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte

		if t.Video[0:4] == "data" {
			var dataURL, err = dataurl.DecodeString(t.Video)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data

			}
		} else if isHTTPURL(t.Video) {
			data, ct, err := fetchURLBytes(r.Context(), t.Video, openGraphImageMaxBytes)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New(fmt.Sprintf("failed to fetch image from url: %v", err)))
				return
			}
			mimeType := ct
			if !strings.HasPrefix(strings.ToLower(mimeType), "video/") {
				mimeType = "video/mpeg"
			}
			imgDataURL := dataurl.New(data, mimeType)
			parsed, err := dataurl.DecodeString(imgDataURL.String())
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("could not re-encode video to base64"))
				return
			}
			filedata = parsed.Data

		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("data should start with \"data:mime/type;base64,\""))
			return
		}

		uploaded, err = clientManager.GetWhatsmeowClient(txtid).Upload(context.Background(), filedata, whatsmeow.MediaVideo)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to upload file: %v", err)))
			return
		}

		msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			Caption:    proto.String(t.Caption),
			URL:        proto.String(uploaded.URL),
			DirectPath: proto.String(uploaded.DirectPath),
			MediaKey:   uploaded.MediaKey,
			Mimetype: proto.String(func() string {
				if t.MimeType != "" {
					return t.MimeType
				}
				return http.DetectContentType(filedata)
			}()),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			JPEGThumbnail: t.JPEGThumbnail,
		}}

		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, it uses the old logic (empty message).
				qm = &waE2E.Message{Conversation: proto.String("")}
			}

			if msg.VideoMessage.ContextInfo == nil {
				msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: qm,
				}
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.VideoMessage.ContextInfo == nil {
				msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.VideoMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.VideoMessage.ContextInfo == nil {
				msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.VideoMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}

		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "video", t.Caption, "", historyLimit)

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends Contact
func (s *server) SendContact() http.HandlerFunc {

	type contactStruct struct {
		Phone       string
		Id          string
		Name        string
		Vcard       string
		ContextInfo waE2E.ContextInfo
		QuotedMessage *waE2E.Message `json:"QuotedMessage,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t contactStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}
		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}
		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Name in Payload"))
			return
		}
		if t.Vcard == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Vcard in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		msg := &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
			DisplayName: &t.Name,
			Vcard:       &t.Vcard,
		}}

		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, it uses the old logic (empty message).
				qm = &waE2E.Message{Conversation: proto.String("")}
			}

			if msg.ContactMessage.ContextInfo == nil {
				msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: qm,
				}
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ContactMessage.ContextInfo == nil {
				msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.ContactMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.ContactMessage.ContextInfo == nil {
				msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.ContactMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}

		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "contact", t.Name, "", historyLimit)

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends location
func (s *server) SendLocation() http.HandlerFunc {

	type locationStruct struct {
		Phone       string
		Id          string
		Name        string
		Latitude    float64
		Longitude   float64
		ContextInfo waE2E.ContextInfo
		QuotedMessage *waE2E.Message `json:"QuotedMessage,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t locationStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}
		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}
		if t.Latitude == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Latitude in Payload"))
			return
		}
		if t.Longitude == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Longitude in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		msg := &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  &t.Latitude,
			DegreesLongitude: &t.Longitude,
			Name:             &t.Name,
		}}

		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, it uses the old logic (empty message).
				qm = &waE2E.Message{Conversation: proto.String("")}
			}

			if msg.LocationMessage.ContextInfo == nil {
				msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: qm,
				}
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.LocationMessage.ContextInfo == nil {
				msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.LocationMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.LocationMessage.ContextInfo == nil {
				msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.LocationMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}

		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "location", t.Name, "", historyLimit)

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sends Buttons (not implemented, does not work)
func (s *server) SendButtons() http.HandlerFunc {

	type buttonStruct struct {
		ButtonId   string
		ButtonText string
	}
	type textStruct struct {
		Phone   string
		Title   string
		Buttons []buttonStruct
		Id      string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Title == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Title in Payload"))
			return
		}

		if len(t.Buttons) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Buttons in Payload"))
			return
		}
		if len(t.Buttons) > 3 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("buttons cant more than 3"))
			return
		}

		recipient, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var buttons []*waE2E.ButtonsMessage_Button

		for _, item := range t.Buttons {
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:       proto.String(item.ButtonId),
				ButtonText:     &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: proto.String(item.ButtonText)},
				Type:           waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
				NativeFlowInfo: &waE2E.ButtonsMessage_Button_NativeFlowInfo{},
			})
		}

		msg2 := &waE2E.ButtonsMessage{
			ContentText: proto.String(t.Title),
			HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
			Buttons:     buttons,
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: msg2,
			},
		}}, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// SendList
func (s *server) SendList() http.HandlerFunc {
	type listItem struct {
		Title string `json:"title"`
		Desc  string `json:"desc"`
		RowId string `json:"RowId"`
	}
	type section struct {
		Title string     `json:"title"`
		Rows  []listItem `json:"rows"`
	}
	type listRequest struct {
		Phone      string     `json:"Phone"`
		ButtonText string     `json:"ButtonText"`
		Desc       string     `json:"Desc"`
		TopText    string     `json:"TopText"`
		Sections   []section  `json:"Sections"`
		List       []listItem `json:"List"` // compatibility
		FooterText string     `json:"FooterText"`
		Id         string     `json:"Id,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		var req listRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		// Required fields validation - FooterText is optional
		if req.Phone == "" || req.ButtonText == "" || req.Desc == "" || req.TopText == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing required fields: Phone, ButtonText, Desc, TopText"))
			return
		}

		// Priority for Sections, but accepts List for compatibility
		var sections []*waE2E.ListMessage_Section
		if len(req.Sections) > 0 {
			for _, sec := range req.Sections {
				var rows []*waE2E.ListMessage_Row
				for _, item := range sec.Rows {
					rowId := item.RowId
					if rowId == "" {
						rowId = item.Title // fallback
					}
					rows = append(rows, &waE2E.ListMessage_Row{
						RowID:       proto.String(rowId),
						Title:       proto.String(item.Title),
						Description: proto.String(item.Desc),
					})
				}
				sections = append(sections, &waE2E.ListMessage_Section{
					Title: proto.String(sec.Title),
					Rows:  rows,
				})
			}
		} else if len(req.List) > 0 {
			var rows []*waE2E.ListMessage_Row
			for _, item := range req.List {
				rowId := item.RowId
				if rowId == "" {
					rowId = item.Title // fallback
				}
				rows = append(rows, &waE2E.ListMessage_Row{
					RowID:       proto.String(rowId),
					Title:       proto.String(item.Title),
					Description: proto.String(item.Desc),
				})
			}

			// Debug: dynamic title: uses TopText if it exists, otherwise 'Menu'
			sectionTitle := req.TopText
			if sectionTitle == "" {
				sectionTitle = "Menu"
			}
			sections = append(sections, &waE2E.ListMessage_Section{
				Title: proto.String(sectionTitle),
				Rows:  rows,
			})
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("no section or list provided"))
			return
		}

		recipient, ok := parseJID(req.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
			return
		}

		msgid := req.Id
		if msgid == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		}

		// Create the message with ListMessage
		listMsg := &waE2E.ListMessage{
			Title:       proto.String(req.TopText),
			Description: proto.String(req.Desc),
			ButtonText:  proto.String(req.ButtonText),
			ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
			Sections:    sections,
		}

		// Add footer only if provided
		if req.FooterText != "" {
			listMsg.FooterText = proto.String(req.FooterText)
		}

		// Try with ViewOnceMessage wrapper as some users report this helps with error 405
		msg := &waE2E.Message{
			ViewOnceMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					ListMessage: listMsg,
				},
			},
		}

		resp, err := clientManager.GetWhatsmeowClient(txtid).SendMessage(
			context.Background(),
			recipient,
			msg,
			whatsmeow.SendRequestExtra{ID: msgid},
		)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message list sent")
		response := map[string]interface{}{
			"Details":   "Sent",
			"Timestamp": resp.Timestamp,
			"Id":        msgid,
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Sends a status text message
func (s *server) SetStatusMessage() http.HandlerFunc {

	type textStruct struct {
		Body string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Body == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Body in Payload"))
			return
		}

		msg := proto.String(t.Body)

		err = clientManager.GetWhatsmeowClient(txtid).SetStatusMessage(context.Background(), *msg)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending status message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Status message sent")
		response := map[string]interface{}{"Details": "Set"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Sends a regular text message
func (s *server) SendMessage() http.HandlerFunc {
	type textStruct struct {
		Phone         string
		Body          string
		LinkPreview   bool
		Id            string
		ContextInfo   waE2E.ContextInfo
		QuotedText    string          `json:"QuotedText,omitempty"`
		QuotedMessage *waE2E.Message  `json:"QuotedMessage,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}
		msgid := ""
		var resp whatsmeow.SendResponse
		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}
		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}
		if t.Body == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Body in Payload"))
			return
		}
		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}
		var (
			url         string
			title       string
			description string
			imageData   []byte
		)
		if t.LinkPreview {
			url = extractFirstURL(t.Body)
			if url != "" {
				title, description, imageData = getOpenGraphData(r.Context(), url, txtid)
			}
		}
		msg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:          proto.String(t.Body),
				MatchedText:   proto.String(url),
				Title:         proto.String(title),
				Description:   proto.String(description),
				JPEGThumbnail: imageData,
			},
		}
		if t.ContextInfo.StanzaID != nil {
			var qm *waE2E.Message

			// If QuotedMessage was provided, use it.
			if t.QuotedMessage != nil {
				qm = t.QuotedMessage
			} else {
				// Otherwise, use the old logic with QuotedText.
				qm = &waE2E.Message{}
				if t.QuotedText != "" {
					qm.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
						Text: proto.String(t.QuotedText),
					}
				} else {
					qm.Conversation = proto.String("")
				}
			}

			msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: qm,
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}
		if t.ContextInfo.IsForwarded != nil && *t.ContextInfo.IsForwarded {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.IsForwarded = proto.Bool(true)
		}
		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}
		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)
		s.saveOutgoingMessageToHistory(txtid, recipient.String(), msgid, "text", t.Body, "", historyLimit)
		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

func (s *server) SendPoll() http.HandlerFunc {
	type pollRequest struct {
		Group   string   `json:"group"`   // The recipient's group id (120363313346913103@g.us)
		Header  string   `json:"header"`  // The poll's headline text
		Options []string `json:"options"` // The list of poll options
		Id      string
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var req pollRequest
		err := decoder.Decode(&req)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		if req.Group == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Grouop in payload"))
			return
		}

		if req.Header == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Header in payload"))
			return
		}

		if len(req.Options) < 2 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("at least 2 options are required"))
			return
		}

		if req.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = req.Id
		}

		recipient, err := validateMessageFields(req.Group, nil, nil)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		pollMessage := clientManager.GetWhatsmeowClient(txtid).BuildPollCreation(req.Header, req.Options, 1)
		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, pollMessage, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to send poll: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Poll sent")

		response := map[string]interface{}{"Details": "Poll sent successfully", "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Delete message
func (s *server) DeleteMessage() http.HandlerFunc {

	type textStruct struct {
		Phone string
		Id    string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Id == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Id in Payload"))
			return
		}

		msgid = t.Id

		recipient, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
			return
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, clientManager.GetWhatsmeowClient(txtid).BuildRevoke(recipient, types.EmptyJID, msgid))
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message deleted")
		response := map[string]interface{}{"Details": "Deleted", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Sends a edit text message
func (s *server) SendEditMessage() http.HandlerFunc {

	type editStruct struct {
		Phone       string
		Body        string
		Id          string
		ContextInfo waE2E.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t editStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Body == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Body in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Id in Payload"))
			return
		} else {
			msgid = t.Id
		}

		msg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: &t.Body,
			},
		}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, clientManager.GetWhatsmeowClient(txtid).BuildEdit(recipient, msgid, msg))
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending edit message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp.Unix())).Str("id", msgid).Msg("Message edit sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Request History Sync
func (s *server) RequestHistorySync() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		var resp whatsmeow.SendResponse
		var err error

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// Parse query parameters
		query := r.URL.Query()

		// Default count is 50, can be overridden by query parameter
		count := 50
		if countStr := query.Get("count"); countStr != "" {
			if parsedCount, err := strconv.Atoi(countStr); err == nil && parsedCount > 0 {
				count = parsedCount
			}
		}

		// Get or create MessageInfo from cache or query parameters
		var info *types.MessageInfo
		if cachedInfo, found := lastMessageCache.Get(txtid); found {
			info = cachedInfo.(*types.MessageInfo)
		} else {
			info = &types.MessageInfo{}
		}

		// Override MessageInfo fields with query parameters if provided
		if chatJIDStr := query.Get("chat_jid"); chatJIDStr != "" {
			if chatJID, err := types.ParseJID(chatJIDStr); err == nil {
				info.Chat = chatJID
			}
		}

		if messageID := query.Get("oldest_msg_id"); messageID != "" {
			info.ID = types.MessageID(messageID)
		}

		if oldestFromMeStr := query.Get("oldest_msg_from_me"); oldestFromMeStr != "" {
			if oldestFromMe, err := strconv.ParseBool(oldestFromMeStr); err == nil {
				info.IsFromMe = oldestFromMe
			}
		}

		if timestampStr := query.Get("oldest_msg_timestamp"); timestampStr != "" {
			if timestamp, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
				info.Timestamp = time.UnixMilli(timestamp)
			}
		}

		historyMsg := clientManager.GetWhatsmeowClient(txtid).BuildHistorySyncRequest(info, count)
		if historyMsg == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("Failed to build history sync request."))
			return
		}

		targetJID := types.JID{Server: "s.whatsapp.net", User: "status"}
		log.Debug().
			Str("userID", txtid).
			Str("target", targetJID.String()).
			Int("count", count).
			Str("chat_jid", info.Chat.String()).
			Str("oldest_msg_id", string(info.ID)).
			Bool("oldest_msg_from_me", info.IsFromMe).
			Time("oldest_msg_timestamp", info.Timestamp).
			Msg("Preparing to send history sync request")

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), clientManager.GetMyClient(txtid).WAClient.Store.ID.ToNonAD(), historyMsg, whatsmeow.SendRequestExtra{Peer: true})
		if err != nil {
			log.Error().
				Str("userID", txtid).
				Err(err).
				Interface("target_jid", targetJID).
				Interface("history_msg", historyMsg).
				Msg("Failed to send history sync request")
			s.Respond(w, r, http.StatusInternalServerError, errors.New("Failed to request history sync."))
			return
		}

		log.Info().
			Str("chat_jid", info.Chat.String()).
			Str("oldest_msg_id", string(info.ID)).
			Bool("oldest_msg_from_me", info.IsFromMe).
			Time("oldest_msg_timestamp", info.Timestamp).
			Msg("History sync request sent")

		response := map[string]interface{}{
			"details":              "History sync request Sent",
			"timestamp":            resp.Timestamp.Unix(),
			"count":                count,
			"chat_jid":             info.Chat.String(),
			"oldest_msg_id":        string(info.ID),
			"oldest_msg_from_me":   info.IsFromMe,
			"oldest_msg_timestamp": info.Timestamp.UnixMilli(),
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

/*
// Sends a Template message
func (s *server) SendTemplate() http.HandlerFunc {

	type buttonStruct struct {
		DisplayText string
		Id          string
		Url         string
		PhoneNumber string
		Type        string
	}

	type templateStruct struct {
		Phone   string
		Content string
		Footer  string
		Id      string
		Buttons []buttonStruct
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientManager.GetWhatsmeowClient(userid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse
//var ts time.Time

		decoder := json.NewDecoder(r.Body)
		var t templateStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Content == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Content in Payload"))
			return
		}

		if t.Footer == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Footer in Payload"))
			return
		}

		if len(t.Buttons) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Buttons in Payload"))
			return
		}

		recipient, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
			return
		}

		if t.Id == "" {
			msgid = clientManager.GetWhatsmeowClient(txtid).GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var buttons []*waE2E.HydratedTemplateButton

		id := 1
		for _, item := range t.Buttons {
			switch item.Type {
			case "quickreply":
				var idtext string
				text := item.DisplayText
				if item.Id == "" {
					idtext = strconv.Itoa(id)
				} else {
					idtext = item.Id
				}
				buttons = append(buttons, &waE2E.HydratedTemplateButton{
					HydratedButton: &waE2E.HydratedTemplateButton_QuickReplyButton{
						QuickReplyButton: &waE2E.HydratedQuickReplyButton{
							DisplayText: &text,
							Id:          proto.String(idtext),
						},
					},
				})
			case "url":
				text := item.DisplayText
				url := item.Url
				buttons = append(buttons, &waE2E.HydratedTemplateButton{
					HydratedButton: &waE2E.HydratedTemplateButton_UrlButton{
						UrlButton: &waE2E.HydratedURLButton{
							DisplayText: &text,
							Url:         &url,
						},
					},
				})
			case "call":
				text := item.DisplayText
				phonenumber := item.PhoneNumber
				buttons = append(buttons, &waE2E.HydratedTemplateButton{
					HydratedButton: &waE2E.HydratedTemplateButton_CallButton{
						CallButton: &waE2E.HydratedCallButton{
							DisplayText: &text,
							PhoneNumber: &phonenumber,
						},
					},
				})
			default:
				text := item.DisplayText
				buttons = append(buttons, &waE2E.HydratedTemplateButton{
					HydratedButton: &waE2E.HydratedTemplateButton_QuickReplyButton{
						QuickReplyButton: &waE2E.HydratedQuickReplyButton{
							DisplayText: &text,
							Id:          proto.String(string(id)),
						},
					},
				})
			}
			id++
		}

		msg := &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{
			HydratedTemplate: &waE2E.HydratedFourRowTemplate{
				HydratedContentText: proto.String(t.Content),
				HydratedFooterText:  proto.String(t.Footer),
				HydratedButtons:     buttons,
				TemplateId:          proto.String("1"),
			},
		},
		}

		resp, err = clientManager.GetWhatsmeowClient(userid).SendMessage(context.Background(),recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp.Unix())).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}
*/

// checks if users/phones are on Whatsapp
func (s *server) CheckUser() http.HandlerFunc {

	type checkUserStruct struct {
		Phone []string
	}

	type User struct {
		Query        string
		IsInWhatsapp bool
		JID          string
		VerifiedName string
	}

	type UserCollection struct {
		Users []User
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t checkUserStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		resp, err := clientManager.GetWhatsmeowClient(txtid).IsOnWhatsApp(context.Background(), t.Phone)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to check if users are on WhatsApp: %s", err)))
			return
		}

		uc := new(UserCollection)
		for _, item := range resp {
			if item.VerifiedName != nil {
				var msg = User{Query: item.Query, IsInWhatsapp: item.IsIn, JID: fmt.Sprintf("%s", item.JID), VerifiedName: item.VerifiedName.Details.GetVerifiedName()}
				uc.Users = append(uc.Users, msg)
			} else {
				var msg = User{Query: item.Query, IsInWhatsapp: item.IsIn, JID: fmt.Sprintf("%s", item.JID), VerifiedName: ""}
				uc.Users = append(uc.Users, msg)
			}
		}
		responseJson, err := json.Marshal(uc)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Gets user information
func (s *server) GetUser() http.HandlerFunc {

	type checkUserStruct struct {
		Phone []string
	}

	type UserCollection struct {
		Users map[types.JID]types.UserInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t checkUserStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		var jids []types.JID
		for _, arg := range t.Phone {
			jid, err := types.ParseJID(arg)
			if err != nil {
				return
			}
			jids = append(jids, jid)
		}
		resp, err := clientManager.GetWhatsmeowClient(txtid).GetUserInfo(context.Background(), jids)

		if err != nil {
			msg := fmt.Sprintf("Failed to get user info: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		uc := new(UserCollection)
		uc.Users = make(map[types.JID]types.UserInfo)

		for jid, info := range resp {
			uc.Users[jid] = info
		}

		responseJson, err := json.Marshal(uc)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Sets global presence status
func (s *server) SendPresence() http.HandlerFunc {

	type PresenceRequest struct {
		Type string `json:"type" form:"type"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var pre PresenceRequest
		err := decoder.Decode(&pre)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		var presence types.Presence

		switch pre.Type {
		case "available":
			presence = types.PresenceAvailable
		case "unavailable":
			presence = types.PresenceUnavailable
		default:
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid presence type. Allowed values: 'available', 'unavailable'"))
			return
		}

		log.Info().Str("presence", pre.Type).Msg("Your global presence status")

		err = clientManager.GetWhatsmeowClient(txtid).SendPresence(context.Background(), presence)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failure sending presence to Whatsapp servers"))
			return
		}

		response := map[string]interface{}{"Details": "Presence set successfuly"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return

	}
}

// Gets avatar info for user
func (s *server) GetAvatar() http.HandlerFunc {

	type getAvatarStruct struct {
		Phone   string
		Preview bool
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t getAvatarStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		jid, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
			return
		}

		var pic *types.ProfilePictureInfo

		existingID := ""
		pic, err = clientManager.GetWhatsmeowClient(txtid).GetProfilePictureInfo(context.Background(), jid, &whatsmeow.GetProfilePictureParams{
			Preview:    t.Preview,
			ExistingID: existingID,
		})
		if err != nil {
			msg := fmt.Sprintf("failed to get avatar: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
			return
		}

		if pic == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no avatar found"))
			return
		}

		log.Info().Str("id", pic.ID).Str("url", pic.URL).Msg("Got avatar")

		responseJson, err := json.Marshal(pic)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Gets all contacts
func (s *server) GetContacts() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		result := map[types.JID]types.ContactInfo{}
		result, err := clientManager.GetWhatsmeowClient(txtid).Store.Contacts.GetAllContacts(context.Background())
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		responseJson, err := json.Marshal(result)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Sets Chat Presence (typing/paused/recording audio)
func (s *server) ChatPresence() http.HandlerFunc {

	type chatPresenceStruct struct {
		Phone string
		State string
		Media types.ChatPresenceMedia
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t chatPresenceStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if len(t.State) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing State in Payload"))
			return
		}

		jid, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).SendChatPresence(context.Background(), jid, types.ChatPresence(t.State), types.ChatPresenceMedia(t.Media))
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failure sending chat presence to Whatsapp servers"))
			return
		}

		response := map[string]interface{}{"Details": "Chat presence set successfuly"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Downloads Image and returns base64 representation
func (s *server) DownloadImage() http.HandlerFunc {

	type downloadImageStruct struct {
		Url           string
		DirectPath    string
		MediaKey      []byte
		Mimetype      string
		FileEncSHA256 []byte
		FileSHA256    []byte
		FileLength    uint64
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		mimetype := ""
		var imgdata []byte

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadImageStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String(t.Url),
			DirectPath:    proto.String(t.DirectPath),
			MediaKey:      t.MediaKey,
			Mimetype:      proto.String(t.Mimetype),
			FileEncSHA256: t.FileEncSHA256,
			FileSHA256:    t.FileSHA256,
			FileLength:    &t.FileLength,
		}}

		img := msg.GetImageMessage()

		if img != nil {
			imgdata, err = clientManager.GetWhatsmeowClient(txtid).Download(context.Background(), img)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to download image")
				msg := fmt.Sprintf("failed to download image %v", err)
				s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
				return
			}
			mimetype = img.GetMimetype()
		}

		dataURL := dataurl.New(imgdata, mimetype)
		response := map[string]interface{}{"Mimetype": mimetype, "Data": dataURL.String()}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Downloads Document and returns base64 representation
func (s *server) DownloadDocument() http.HandlerFunc {

	type downloadDocumentStruct struct {
		Url           string
		DirectPath    string
		MediaKey      []byte
		Mimetype      string
		FileEncSHA256 []byte
		FileSHA256    []byte
		FileLength    uint64
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		mimetype := ""
		var docdata []byte

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadDocumentStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		msg := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(t.Url),
			DirectPath:    proto.String(t.DirectPath),
			MediaKey:      t.MediaKey,
			Mimetype:      proto.String(t.Mimetype),
			FileEncSHA256: t.FileEncSHA256,
			FileSHA256:    t.FileSHA256,
			FileLength:    &t.FileLength,
		}}

		doc := msg.GetDocumentMessage()

		if doc != nil {
			docdata, err = clientManager.GetWhatsmeowClient(txtid).Download(context.Background(), doc)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to download document")
				msg := fmt.Sprintf("failed to download document %v", err)
				s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
				return
			}
			mimetype = doc.GetMimetype()
		}

		dataURL := dataurl.New(docdata, mimetype)
		response := map[string]interface{}{"Mimetype": mimetype, "Data": dataURL.String()}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Downloads Video and returns base64 representation
func (s *server) DownloadVideo() http.HandlerFunc {

	type downloadVideoStruct struct {
		Url           string
		DirectPath    string
		MediaKey      []byte
		Mimetype      string
		FileEncSHA256 []byte
		FileSHA256    []byte
		FileLength    uint64
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		mimetype := ""
		var docdata []byte

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadVideoStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			URL:           proto.String(t.Url),
			DirectPath:    proto.String(t.DirectPath),
			MediaKey:      t.MediaKey,
			Mimetype:      proto.String(t.Mimetype),
			FileEncSHA256: t.FileEncSHA256,
			FileSHA256:    t.FileSHA256,
			FileLength:    &t.FileLength,
		}}

		doc := msg.GetVideoMessage()

		if doc != nil {
			docdata, err = clientManager.GetWhatsmeowClient(txtid).Download(context.Background(), doc)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to download video")
				msg := fmt.Sprintf("failed to download video %v", err)
				s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
				return
			}
			mimetype = doc.GetMimetype()
		}

		dataURL := dataurl.New(docdata, mimetype)
		response := map[string]interface{}{"Mimetype": mimetype, "Data": dataURL.String()}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// Downloads Audio and returns base64 representation
func (s *server) DownloadAudio() http.HandlerFunc {

	type downloadAudioStruct struct {
		Url           string
		DirectPath    string
		MediaKey      []byte
		Mimetype      string
		FileEncSHA256 []byte
		FileSHA256    []byte
		FileLength    uint64
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		mimetype := ""
		var docdata []byte

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadAudioStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(t.Url),
			DirectPath:    proto.String(t.DirectPath),
			MediaKey:      t.MediaKey,
			Mimetype:      proto.String(t.Mimetype),
			FileEncSHA256: t.FileEncSHA256,
			FileSHA256:    t.FileSHA256,
			FileLength:    &t.FileLength,
		}}

		doc := msg.GetAudioMessage()

		if doc != nil {
			docdata, err = clientManager.GetWhatsmeowClient(txtid).Download(context.Background(), doc)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to download audio")
				msg := fmt.Sprintf("failed to download audio %v", err)
				s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
				return
			}
			mimetype = doc.GetMimetype()
		}

		dataURL := dataurl.New(docdata, mimetype)
		response := map[string]interface{}{"Mimetype": mimetype, "Data": dataURL.String()}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// React
func (s *server) React() http.HandlerFunc {

	type textStruct struct {
		Phone       string
		Body        string
		Id          string
		Participant string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}

		if t.Body == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Body in Payload"))
			return
		}

		recipient, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		if t.Id == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Id in Payload"))
			return
		} else {
			msgid = t.Id
		}

		fromMe := false
		if strings.HasPrefix(msgid, "me:") {
			fromMe = true
			msgid = msgid[len("me:"):]
		}
		reaction := t.Body
		if reaction == "remove" {
			reaction = ""
		}

		var participantJID types.JID
		if !fromMe && t.Participant != "" {
			if pj, ok := parseJID(t.Participant); ok {
				participantJID = pj
			}
		}

		key := &waCommon.MessageKey{
			RemoteJID: proto.String(recipient.String()),
			FromMe:    proto.Bool(fromMe),
			ID:        proto.String(msgid),
		}
		if !fromMe && participantJID.String() != "" {
			key.Participant = proto.String(participantJID.String())
		}

		msg := &waE2E.Message{
			ReactionMessage: &waE2E.ReactionMessage{
				Key:               key,
				Text:              proto.String(reaction),
				GroupingKey:       proto.String(reaction),
				SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
			},
		}

		resp, err = clientManager.GetWhatsmeowClient(txtid).SendMessage(context.Background(), recipient, msg)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%v", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp.Unix(), "Id": msgid}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Mark messages as read
func (s *server) MarkRead() http.HandlerFunc {

	type markReadStruct struct {
		Id          []string
		Chat        types.JID // Legacy: Kept for backward compatibility
		Sender      types.JID // Legacy: Kept for backward compatibility
		ChatPhone   string    // New standardized field (prioritized)
		SenderPhone string    // New standardized field (prioritized)
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t markReadStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		var jidChat types.JID

		if len(t.ChatPhone) > 0 {
			var ok bool
			jidChat, ok = parseJID(t.ChatPhone)
			if !ok {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse ChatPhone"))
				return
			}
		} else if t.Chat.String() != "" {
			jidChat = t.Chat
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing ChatPhone in Payload"))
			return
		}

		var jidSender types.JID

		if len(t.SenderPhone) > 0 {
			var ok bool
			jidSender, ok = parseJID(t.SenderPhone)
			if !ok {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse SenderPhone"))
				return
			}
		} else if t.Sender.String() != "" {
			jidSender = t.Sender
		}

		if len(t.Id) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Id in Payload"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).MarkRead(context.Background(), t.Id, time.Now(), jidChat, jidSender)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failure marking messages as read"))
			return
		}

		response := map[string]interface{}{"Details": "Message(s) marked as read"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// List groups
func (s *server) ListGroups() http.HandlerFunc {

	type GroupCollection struct {
		Groups []types.GroupInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		resp, err := clientManager.GetWhatsmeowClient(txtid).GetJoinedGroups(r.Context())

		if err != nil {
			msg := fmt.Sprintf("failed to get group list: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		gc := new(GroupCollection)
		for _, info := range resp {
			gc.Groups = append(gc.Groups, *info)
		}

		responseJson, err := json.Marshal(gc)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Get group info
func (s *server) GetGroupInfo() http.HandlerFunc {

	type getGroupInfoStruct struct {
		GroupJID string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// Get GroupJID from query parameter
		groupJID := r.URL.Query().Get("groupJID")
		if groupJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing groupJID parameter"))
			return
		}

		group, ok := parseJID(groupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		resp, err := clientManager.GetWhatsmeowClient(txtid).GetGroupInfo(context.Background(), group)

		if err != nil {
			msg := fmt.Sprintf("Failed to get group info: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		responseJson, err := json.Marshal(resp)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Get group invite link
func (s *server) GetGroupInviteLink() http.HandlerFunc {

	type getGroupInfoStruct struct {
		GroupJID string
		Reset    bool
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// Get GroupJID from query parameter
		groupJID := r.URL.Query().Get("groupJID")
		if groupJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing groupJID parameter"))
			return
		}

		// Get reset parameter
		resetParam := r.URL.Query().Get("reset")
		reset := false
		if resetParam != "" {
			var err error
			reset, err = strconv.ParseBool(resetParam)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("invalid reset parameter, must be true or false"))
				return
			}
		}

		group, ok := parseJID(groupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		resp, err := clientManager.GetWhatsmeowClient(txtid).GetGroupInviteLink(context.Background(), group, reset)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("Failed to get group invite link")
			msg := fmt.Sprintf("Failed to get group invite link: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"InviteLink": resp}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Join group invite link
func (s *server) GroupJoin() http.HandlerFunc {

	type joinGroupStruct struct {
		Code string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t joinGroupStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Code == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Code in Payload"))
			return
		}

		_, err = clientManager.GetWhatsmeowClient(txtid).JoinGroupWithLink(context.Background(), t.Code)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to join group")
			msg := fmt.Sprintf("failed to join group: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group joined successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Create group
func (s *server) CreateGroup() http.HandlerFunc {

	type createGroupStruct struct {
		Name         string   `json:"name"`
		Participants []string `json:"participants"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t createGroupStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Name in Payload"))
			return
		}

		if len(t.Participants) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Participants in Payload"))
			return
		}

		// Parse participant phone numbers
		participantJIDs := make([]types.JID, len(t.Participants))
		var ok bool
		for i, phone := range t.Participants {
			participantJIDs[i], ok = parseJID(phone)
			if !ok {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Participant Phone"))
				return
			}
		}

		req := whatsmeow.ReqCreateGroup{
			Name:         t.Name,
			Participants: participantJIDs,
		}

		groupInfo, err := clientManager.GetWhatsmeowClient(txtid).CreateGroup(r.Context(), req)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to create group")
			msg := fmt.Sprintf("failed to create group: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		responseJson, err := json.Marshal(groupInfo)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Set group locked
func (s *server) SetGroupLocked() http.HandlerFunc {

	type setGroupLockedStruct struct {
		GroupJID string `json:"groupjid"`
		Locked   bool   `json:"locked"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupLockedStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).SetGroupLocked(context.Background(), group, t.Locked)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to set group locked")
			msg := fmt.Sprintf("failed to set group locked: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group Locked setting updated successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Set disappearing timer (ephemeral messages)
func (s *server) SetDisappearingTimer() http.HandlerFunc {

	type setDisappearingTimerStruct struct {
		GroupJID string `json:"groupjid"`
		Duration string `json:"duration"` // "24h", "7d", "90d", "off"
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setDisappearingTimerStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		if t.Duration == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Duration in Payload"))
			return
		}

		var duration time.Duration
		switch t.Duration {
		case "24h":
			duration = 24 * time.Hour
		case "7d":
			duration = 7 * 24 * time.Hour
		case "90d":
			duration = 90 * 24 * time.Hour
		case "off":
			duration = 0
		default:
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid duration. Use: 24h, 7d, 90d, or off"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).SetDisappearingTimer(context.Background(), group, duration, time.Now())

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to set disappearing timer")
			msg := fmt.Sprintf("failed to set disappearing timer: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Disappearing timer set successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Remove group photo
func (s *server) RemoveGroupPhoto() http.HandlerFunc {

	type removeGroupPhotoStruct struct {
		GroupJID string `json:"groupjid"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t removeGroupPhotoStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		_, err = clientManager.GetWhatsmeowClient(txtid).SetGroupPhoto(context.Background(), group, nil)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to remove group photo")
			msg := fmt.Sprintf("failed to remove group photo: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group Photo removed successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// add, remove, promote and demote members group
func (s *server) UpdateGroupParticipants() http.HandlerFunc {

	type updateGroupParticipantsStruct struct {
		GroupJID string
		Phone    []string
		// Action string // add, remove, promote, demote
		Action string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t updateGroupParticipantsStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Phone in Payload"))
			return
		}
		// parse phone numbers
		phoneParsed := make([]types.JID, len(t.Phone))
		for i, phone := range t.Phone {
			phoneParsed[i], ok = parseJID(phone)
			if !ok {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
				return
			}
		}

		if t.Action == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Action in Payload"))
			return
		}

		// parse action

		var action whatsmeow.ParticipantChange
		switch t.Action {
		case "add":
			action = "add"
		case "remove":
			action = "remove"
		case "promote":
			action = "promote"
		case "demote":
			action = "demote"
		default:
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid Action in Payload"))
			return
		}

		_, err = clientManager.GetWhatsmeowClient(txtid).UpdateGroupParticipants(context.Background(), group, phoneParsed, action)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to change participant group")
			msg := fmt.Sprintf("failed to change participant group: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group Participants updated successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Get group invite info
func (s *server) GetGroupInviteInfo() http.HandlerFunc {

	type getGroupInviteInfoStruct struct {
		Code string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t getGroupInviteInfoStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Code == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Code in Payload"))
			return
		}

		groupInfo, err := clientManager.GetWhatsmeowClient(txtid).GetGroupInfoFromLink(context.Background(), t.Code)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to get group invite info")
			msg := fmt.Sprintf("failed to get group invite info: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		responseJson, err := json.Marshal(groupInfo)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Set group photo
func (s *server) SetGroupPhoto() http.HandlerFunc {

	type setGroupPhotoStruct struct {
		GroupJID string
		Image    string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupPhotoStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		if t.Image == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Image in Payload"))
			return
		}

		var filedata []byte

		// Check if the image data starts with a valid data URL format
		if len(t.Image) > 10 && t.Image[0:10] == "data:image" {
			var dataURL, err = dataurl.DecodeString(t.Image)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("image data should start with \"data:image/\" (supported formats: jpeg, png, gif, webp)"))
			return
		}

		// Validate that we have image data
		if len(filedata) == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("no image data found in payload"))
			return
		}

		// Validate JPEG format (WhatsApp requires JPEG)
		if len(filedata) < 3 || filedata[0] != 0xFF || filedata[1] != 0xD8 || filedata[2] != 0xFF {
			s.Respond(w, r, http.StatusBadRequest, errors.New("image must be in JPEG format. WhatsApp only accepts JPEG images for group photos"))
			return
		}

		picture_id, err := clientManager.GetWhatsmeowClient(txtid).SetGroupPhoto(context.Background(), group, filedata)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to set group photo")
			msg := fmt.Sprintf("failed to set group photo: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group Photo set successfully", "PictureID": picture_id}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Set group name
func (s *server) SetGroupName() http.HandlerFunc {

	type setGroupNameStruct struct {
		GroupJID string
		Name     string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupNameStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Name in Payload"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).SetGroupName(context.Background(), group, t.Name)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to set group name")
			msg := fmt.Sprintf("failed to set group name: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group Name set successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Set group topic (description)
func (s *server) SetGroupTopic() http.HandlerFunc {

	type setGroupTopicStruct struct {
		GroupJID string
		Topic    string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupTopicStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		if t.Topic == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Topic in Payload"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).SetGroupTopic(context.Background(), group, "", "", t.Topic)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to set group topic")
			msg := fmt.Sprintf("failed to set group topic: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group Topic set successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Leave group
func (s *server) GroupLeave() http.HandlerFunc {

	type groupLeaveStruct struct {
		GroupJID string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t groupLeaveStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).LeaveGroup(context.Background(), group)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to leave group")
			msg := fmt.Sprintf("failed to leave group: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group left successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// SetGroupAnnounce post
func (s *server) SetGroupAnnounce() http.HandlerFunc {

	type setGroupAnnounceStruct struct {
		GroupJID string
		Announce bool
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupAnnounceStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).SetGroupAnnounce(context.Background(), group, t.Announce)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to set group announce")
			msg := fmt.Sprintf("failed to set group announce: %v", err)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		response := map[string]interface{}{"Details": "Group Announce set successfully"}
		responseJson, err := json.Marshal(response)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// List newsletters
func (s *server) ListNewsletter() http.HandlerFunc {

	type NewsletterCollection struct {
		Newsletter []types.NewsletterMetadata
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		resp, err := clientManager.GetWhatsmeowClient(txtid).GetSubscribedNewsletters(context.Background())

		if err != nil {
			msg := fmt.Sprintf("failed to get newsletter list: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		gc := new(NewsletterCollection)
		gc.Newsletter = []types.NewsletterMetadata{}
		for _, info := range resp {
			gc.Newsletter = append(gc.Newsletter, *info)
		}

		responseJson, err := json.Marshal(gc)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// Admin List users
func (s *server) ListUsers() http.HandlerFunc {
	type usersStruct struct {
		Id         string         `db:"id"`
		Name       string         `db:"name"`
		Token      string         `db:"token"`
		Webhook    string         `db:"webhook"`
		Jid        string         `db:"jid"`
		Qrcode     string         `db:"qrcode"`
		Connected  sql.NullBool   `db:"connected"`
		Expiration sql.NullInt64  `db:"expiration"`
		ProxyURL   sql.NullString `db:"proxy_url"`
		Events     string         `db:"events"`
		History    sql.NullInt64  `db:"history"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID, hasID := vars["id"]

		var query string
		var args []interface{}

		if hasID {
			// Fetch a single user
			query = "SELECT id, name, token, webhook, jid, qrcode, connected, expiration, proxy_url, events, history FROM users WHERE id = $1"
			args = append(args, userID)
		} else {
			// Fetch all users
			query = "SELECT id, name, token, webhook, jid, qrcode, connected, expiration, proxy_url, events, history FROM users"
		}

		rows, err := s.db.Queryx(query, args...)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("problem accessing DB"))
			return
		}
		defer rows.Close()

		// Create a slice to store the user data
		users := []map[string]interface{}{}
		// Iterate over the rows and populate the user data
		for rows.Next() {
			var user usersStruct
			err := rows.StructScan(&user)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("admin DB error")
				s.Respond(w, r, http.StatusInternalServerError, errors.New("problem accessing DB"))
				return
			}

			isConnected := false
			isLoggedIn := false
			if clientManager.GetWhatsmeowClient(user.Id) != nil {
				isConnected = clientManager.GetWhatsmeowClient(user.Id).IsConnected()
				isLoggedIn = clientManager.GetWhatsmeowClient(user.Id).IsLoggedIn()
			}

			//"connected":  user.Connected.Bool,
			userMap := map[string]interface{}{
				"id":         user.Id,
				"name":       user.Name,
				"token":      user.Token,
				"webhook":    user.Webhook,
				"jid":        user.Jid,
				"qrcode":     user.Qrcode,
				"connected":  isConnected,
				"loggedIn":   isLoggedIn,
				"expiration": user.Expiration.Int64,
				"proxy_url":  user.ProxyURL.String,
				"events":     user.Events,
			}
			// Add proxy_config
			proxyURL := user.ProxyURL.String
			userMap["proxy_config"] = map[string]interface{}{
				"enabled":   proxyURL != "",
				"proxy_url": proxyURL,
			}
			// Add s3_config (search S3 fields in the database)
			var s3Enabled bool
			var s3Endpoint, s3Region, s3Bucket, s3PublicURL, s3MediaDelivery string
			var s3PathStyle bool
			var s3RetentionDays int
			// Start with safe defaults so the field is always present in the response
			s3Config := map[string]interface{}{
				"enabled":        false,
				"endpoint":       "",
				"region":         "",
				"bucket":         "",
				"access_key":     "***",
				"path_style":     false,
				"public_url":     "",
				"media_delivery": "",
				"retention_days": 0,
			}
			err = s.db.QueryRow(`SELECT COALESCE(s3_enabled, false), COALESCE(s3_endpoint, ''), COALESCE(s3_region, ''), COALESCE(s3_bucket, ''), COALESCE(s3_path_style, false), COALESCE(s3_public_url, ''), COALESCE(media_delivery, ''), COALESCE(s3_retention_days, 0) FROM users WHERE id = $1`, user.Id).Scan(&s3Enabled, &s3Endpoint, &s3Region, &s3Bucket, &s3PathStyle, &s3PublicURL, &s3MediaDelivery, &s3RetentionDays)
			if err == nil {
				// Overwrite defaults with actual values if the query succeeded
				s3Config["enabled"] = s3Enabled
				s3Config["endpoint"] = s3Endpoint
				s3Config["region"] = s3Region
				s3Config["bucket"] = s3Bucket
				s3Config["path_style"] = s3PathStyle
				s3Config["public_url"] = s3PublicURL
				s3Config["media_delivery"] = s3MediaDelivery
				s3Config["retention_days"] = s3RetentionDays
			} else {
				if err != sql.ErrNoRows {
					log.Warn().Err(err).Str("user_id", user.Id).Msg("Failed to query S3 config for user")
				}
			}
			userMap["s3_config"] = s3Config
			users = append(users, userMap)
		}
		// Check for any error that occurred during iteration
		if err := rows.Err(); err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("problem accessing DB"))
			return
		}

		// Encode users slice into a JSON string
		responseJson, err := json.Marshal(users)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}

		s.Respond(w, r, http.StatusOK, string(responseJson))

	}
}

// Add user
func (s *server) AddUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		type ProxyConfig struct {
			Enabled  bool   `json:"enabled"`
			ProxyURL string `json:"proxyURL"`
		}

		// Parse the request body
		var user struct {
			Name        string       `json:"name"`
			Token       string       `json:"token"`
			Webhook     string       `json:"webhook,omitempty"`
			Expiration  int          `json:"expiration,omitempty"`
			Events      string       `json:"events,omitempty"`
			ProxyConfig *ProxyConfig `json:"proxyConfig,omitempty"`
			S3Config    *S3Config    `json:"s3Config,omitempty"`
			HmacKey     string       `json:"hmacKey,omitempty"`
			History     int          `json:"history,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			log.Error().Err(err).Msg("Failed to decode user payload")
			s.respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{
				"code":    http.StatusBadRequest,
				"error":   "invalid request payload",
				"success": false,
			})
			return
		}

		log.Info().Interface("proxyConfig", user.ProxyConfig).Interface("s3Config", user.S3Config).Msg("Received values for proxyConfig and s3Config")
		log.Debug().Interface("user", user).Msg("Received values for user")

		// Set defaults only if nil
		if user.Events == "" {
			user.Events = ""
		}
		if user.ProxyConfig == nil {
			user.ProxyConfig = &ProxyConfig{}
		}
		if user.S3Config == nil {
			user.S3Config = &S3Config{}
		}
		if user.Webhook == "" {
			user.Webhook = ""
		}

		// Encrypt HMAC key if provided
		var encryptedHmacKey []byte
		if user.HmacKey != "" {
			// Validate HMAC key length
			if len(user.HmacKey) < 32 {
				s.respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{
					"code":    http.StatusBadRequest,
					"error":   "HMAC key must be at least 32 characters long",
					"success": false,
				})
				return
			}

			var err error
			encryptedHmacKey, err = encryptHMACKey(user.HmacKey)
			if err != nil {
				log.Error().Err(err).Msg("Failed to encrypt HMAC key")
				s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"code":    http.StatusInternalServerError,
					"error":   "failed to encrypt HMAC key",
					"success": false,
				})
				return
			}
		}

		// Check for existing user
		var count int
		if err := s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE token = $1", user.Token); err != nil {
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "database error",
				"success": false,
			})
			return
		}
		if count > 0 {
			s.respondWithJSON(w, http.StatusConflict, map[string]interface{}{
				"code":    http.StatusConflict,
				"error":   "user with this token already exists",
				"success": false,
			})
			return
		}

		// Validate events
		eventList := strings.Split(user.Events, ",")
		for _, event := range eventList {
			event = strings.TrimSpace(event)
			if event == "" {
				continue // allow empty
			}
			if !Find(supportedEventTypes, event) {
				s.respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{
					"code":    http.StatusBadRequest,
					"error":   "invalid event type",
					"success": false,
					"details": "invalid event: " + event,
				})
				return
			}
		}

		// Generate ID
		id, err := GenerateRandomID()
		if err != nil {
			log.Error().Err(err).Msg("failed to generate random ID")
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "failed to generate user ID",
				"success": false,
			})
			return
		}

		// Insert user with all proxy, S3 and HMAC fields
		if _, err = s.db.Exec(
			"INSERT INTO users (id, name, token, webhook, expiration, events, jid, qrcode, proxy_url, s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key, s3_secret_key, s3_path_style, s3_public_url, media_delivery, s3_retention_days, hmac_key, history) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)",
			id, user.Name, user.Token, user.Webhook, user.Expiration, user.Events, "", "", user.ProxyConfig.ProxyURL,
			user.S3Config.Enabled, user.S3Config.Endpoint, user.S3Config.Region, user.S3Config.Bucket, user.S3Config.AccessKey, user.S3Config.SecretKey, user.S3Config.PathStyle, user.S3Config.PublicURL, user.S3Config.MediaDelivery, user.S3Config.RetentionDays, encryptedHmacKey, user.History,
		); err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("admin DB error")
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "database error",
				"success": false,
			})
			return
		}

		// Initialize S3Manager if necessary
		if user.S3Config != nil && user.S3Config.Enabled {
			s3Config := &S3Config{
				Enabled:       user.S3Config.Enabled,
				Endpoint:      user.S3Config.Endpoint,
				Region:        user.S3Config.Region,
				Bucket:        user.S3Config.Bucket,
				AccessKey:     user.S3Config.AccessKey,
				SecretKey:     user.S3Config.SecretKey,
				PathStyle:     user.S3Config.PathStyle,
				PublicURL:     user.S3Config.PublicURL,
				MediaDelivery: user.S3Config.MediaDelivery,
				RetentionDays: user.S3Config.RetentionDays,
			}
			_ = GetS3Manager().InitializeS3Client(id, s3Config)
		}

		// Build response like GET /admin/users
		proxyConfig := map[string]interface{}{
			"enabled":   user.ProxyConfig.Enabled,
			"proxy_url": user.ProxyConfig.ProxyURL,
		}
		s3Config := map[string]interface{}{
			"enabled":        user.S3Config.Enabled,
			"endpoint":       user.S3Config.Endpoint,
			"region":         user.S3Config.Region,
			"bucket":         user.S3Config.Bucket,
			"access_key":     "***",
			"path_style":     user.S3Config.PathStyle,
			"public_url":     user.S3Config.PublicURL,
			"media_delivery": user.S3Config.MediaDelivery,
			"retention_days": user.S3Config.RetentionDays,
		}
		userMap := map[string]interface{}{
			"id":           id,
			"name":         user.Name,
			"token":        user.Token,
			"webhook":      user.Webhook,
			"expiration":   user.Expiration,
			"events":       user.Events,
			"proxy_config": proxyConfig,
			"s3_config":    s3Config,
			"hmac_key":     user.HmacKey != "",
		}
		s.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
			"code":    http.StatusCreated,
			"data":    userMap,
			"success": true,
		})
	}
}

// Edit user
func (s *server) EditUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		type ProxyConfig struct {
			Enabled  bool   `json:"enabled"`
			ProxyURL string `json:"proxyURL"`
		}

		// Get the user ID from the request URL
		vars := mux.Vars(r)
		userID := vars["id"]

		// Parse the request body
		var user struct {
			Name        string       `json:"name,omitempty"`
			Token       string       `json:"token,omitempty"`
			Webhook     string       `json:"webhook,omitempty"`
			Expiration  int          `json:"expiration,omitempty"`
			Events      string       `json:"events,omitempty"`
			ProxyConfig *ProxyConfig `json:"proxyConfig,omitempty"`
			S3Config    *S3Config    `json:"s3Config,omitempty"`
			History     int          `json:"history,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			log.Error().Err(err).Msg("Failed to decode user payload")
			s.respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{
				"code":    http.StatusBadRequest,
				"error":   "invalid request payload",
				"success": false,
			})
			return
		}

		log.Info().Interface("proxyConfig", user.ProxyConfig).Interface("s3Config", user.S3Config).Msg("Received values for proxyConfig and s3Config")
		log.Debug().Interface("user", user).Msg("Received values for user")

		// Check if user exists
		var count int
		if err := s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE id = $1", userID); err != nil {
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "database error",
				"success": false,
			})
			return
		}
		if count == 0 {
			s.respondWithJSON(w, http.StatusNotFound, map[string]interface{}{
				"code":    http.StatusNotFound,
				"error":   "user not found",
				"success": false,
			})
			return
		}

		// Validate events if provided
		if user.Events != "" {
			eventList := strings.Split(user.Events, ",")
			for _, event := range eventList {
				event = strings.TrimSpace(event)
				if event == "" {
					continue // allow empty
				}
				if !Find(supportedEventTypes, event) {
					s.respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{
						"code":    http.StatusBadRequest,
						"error":   "invalid event type",
						"success": false,
						"details": "invalid event: " + event,
					})
					return
				}
			}
		}

		// Build dynamic UPDATE query based on provided fields
		query := "UPDATE users SET "
		args := []interface{}{}
		argIndex := 1

		// Helper function to add field to query if provided
		addField := func(fieldName string, value interface{}, condition bool) {
			if condition {
				if argIndex > 1 {
					query += ", "
				}
				query += fieldName + " = $" + strconv.Itoa(argIndex)
				args = append(args, value)
				argIndex++
			}
		}

		// Add fields to update
		addField("name", user.Name, user.Name != "")
		addField("token", user.Token, user.Token != "")
		addField("webhook", user.Webhook, user.Webhook != "")
		addField("expiration", user.Expiration, user.Expiration != 0)
		addField("events", user.Events, user.Events != "")
		addField("history", user.History, user.History != 0)

		// Handle proxy config
		if user.ProxyConfig != nil {
			if user.ProxyConfig.Enabled {
				addField("proxy_url", user.ProxyConfig.ProxyURL, true)
			} else {
				addField("proxy_url", "", true)
			}
		}

		// Handle S3 config
		if user.S3Config != nil {
			addField("s3_enabled", user.S3Config.Enabled, true)
			addField("s3_endpoint", user.S3Config.Endpoint, true)
			addField("s3_region", user.S3Config.Region, true)
			addField("s3_bucket", user.S3Config.Bucket, true)
			addField("s3_access_key", user.S3Config.AccessKey, true)
			addField("s3_secret_key", user.S3Config.SecretKey, true)
			addField("s3_path_style", user.S3Config.PathStyle, true)
			addField("s3_public_url", user.S3Config.PublicURL, true)
			addField("media_delivery", user.S3Config.MediaDelivery, true)
			addField("s3_retention_days", user.S3Config.RetentionDays, true)
		}

		// If no fields to update, return early
		if argIndex == 1 {
			s.respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{
				"code":    http.StatusBadRequest,
				"error":   "no fields to update",
				"success": false,
			})
			return
		}

		// Add WHERE clause
		query += " WHERE id = $" + strconv.Itoa(argIndex)
		args = append(args, userID)

		// Execute the update
		if _, err := s.db.Exec(query, args...); err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("admin DB error")
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "database error",
				"success": false,
			})
			return
		}

		// Update S3Manager if S3 config was modified
		if user.S3Config != nil {
			if user.S3Config.Enabled {
				s3Config := &S3Config{
					Enabled:       user.S3Config.Enabled,
					Endpoint:      user.S3Config.Endpoint,
					Region:        user.S3Config.Region,
					Bucket:        user.S3Config.Bucket,
					AccessKey:     user.S3Config.AccessKey,
					SecretKey:     user.S3Config.SecretKey,
					PathStyle:     user.S3Config.PathStyle,
					PublicURL:     user.S3Config.PublicURL,
					MediaDelivery: user.S3Config.MediaDelivery,
					RetentionDays: user.S3Config.RetentionDays,
				}
				_ = GetS3Manager().InitializeS3Client(userID, s3Config)
			} else {
				// Remove S3 client if disabled
				GetS3Manager().RemoveClient(userID)
			}
		}

		// Update userinfo cache for any modified fields
		// First, get the current user token to find the cache entry
		var currentToken string
		err := s.db.Get(&currentToken, "SELECT token FROM users WHERE id = $1", userID)
		if err != nil {
			log.Error().Err(err).Str("userID", userID).Msg("Failed to get user token for cache update")
		} else {
			// Get current cached userinfo if it exists
			if cachedUserInfo, found := userinfocache.Get(currentToken); found {
				updatedUserInfo := cachedUserInfo.(Values)

				// Update cache fields that were modified
				if user.Name != "" {
					updatedUserInfo = updateUserInfo(updatedUserInfo, "Name", user.Name).(Values)
				}
				if user.Token != "" {
					// If token changed, we need to update the cache key
					updatedUserInfo = updateUserInfo(updatedUserInfo, "Token", user.Token).(Values)
					// Remove old cache entry and add new one with new token
					userinfocache.Delete(currentToken)
					currentToken = user.Token
				}
				if user.Webhook != "" {
					updatedUserInfo = updateUserInfo(updatedUserInfo, "Webhook", user.Webhook).(Values)
				}
				if user.Events != "" {
					updatedUserInfo = updateUserInfo(updatedUserInfo, "Events", user.Events).(Values)
				}
				if user.History != 0 {
					updatedUserInfo = updateUserInfo(updatedUserInfo, "History", strconv.Itoa(user.History)).(Values)
				}
				if user.ProxyConfig != nil {
					if user.ProxyConfig.Enabled {
						updatedUserInfo = updateUserInfo(updatedUserInfo, "Proxy", user.ProxyConfig.ProxyURL).(Values)
					} else {
						updatedUserInfo = updateUserInfo(updatedUserInfo, "Proxy", "").(Values)
					}
				}

				// Update the cache
				userinfocache.Set(currentToken, updatedUserInfo, cache.NoExpiration)
				log.Info().Str("userID", userID).Msg("User info cache updated after edit")
			}
		}

		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"code":    http.StatusOK,
			"message": "user updated successfully",
			"success": true,
		})
	}
}

// Delete user
func (s *server) DeleteUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// Get the user ID from the request URL
		vars := mux.Vars(r)
		userID := vars["id"]

		// Delete the user from the database
		result, err := s.db.Exec("DELETE FROM users WHERE id=$1", userID)
		if err != nil {
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "database error",
				"success": false,
			})
			return
		}

		// Check if the user was deleted
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "Failed to verify deletion",
				"success": false,
			})
			return
		}
		if rowsAffected == 0 {
			s.respondWithJSON(w, http.StatusNotFound, map[string]interface{}{
				"code":    http.StatusNotFound,
				"error":   "user not found",
				"success": false,
				"details": fmt.Sprintf("No user found with ID: %s", userID),
			})
			return
		}
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"code":    http.StatusOK,
			"data":    map[string]string{"id": userID},
			"success": true,
			"details": "user deleted successfully",
		})
	}
}

// Delete user complete
func (s *server) DeleteUserComplete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		vars := mux.Vars(r)
		id := vars["id"]

		// Validate ID
		if id == "" {
			s.respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{
				"code":    http.StatusBadRequest,
				"error":   "missing ID",
				"success": false,
			})
			return
		}

		// Check if user exists
		var exists bool
		err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", id).Scan(&exists)
		if err != nil {
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "database error",
				"success": false,
				"details": "problem checking user existence",
			})
			return
		}
		if !exists {
			s.respondWithJSON(w, http.StatusNotFound, map[string]interface{}{
				"code":    http.StatusNotFound,
				"error":   "user not found",
				"success": false,
				"details": fmt.Sprintf("No user found with ID: %s", id),
			})
			return
		}

		// Get user info before deletion
		var uname, jid, token string
		err = s.db.QueryRow("SELECT name, jid, token FROM users WHERE id = $1", id).Scan(&uname, &jid, &token)
		if err != nil {
			log.Error().Err(err).Str("id", id).Msg("problem retrieving user information")
			// Continue anyway since we have the ID
		}

		// 1. Logout and disconnect instance
		if client := clientManager.GetWhatsmeowClient(id); client != nil {
			if client.IsConnected() {
				log.Info().Str("id", id).Msg("Logging out user")
				client.Logout(context.Background())
			}
			log.Info().Str("id", id).Msg("Disconnecting from WhatsApp")
			client.Disconnect()
		}

		// 2. Remove from DB
		_, err = s.db.Exec("DELETE FROM users WHERE id = $1", id)
		if err != nil {
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"code":    http.StatusInternalServerError,
				"error":   "database error",
				"success": false,
				"details": "failed to delete user from database",
			})
			return
		}

		// 3. Cleanup from memory
		clientManager.DeleteWhatsmeowClient(id)
		clientManager.DeleteMyClient(id)
		clientManager.DeleteHTTPClient(id)
		userinfocache.Delete(token)

		// 4. Remove media files
		userDirectory := filepath.Join(s.exPath, "files", id)
		if stat, err := os.Stat(userDirectory); err == nil && stat.IsDir() {
			log.Info().Str("dir", userDirectory).Msg("deleting media and history files from disk")
			err = os.RemoveAll(userDirectory)
			if err != nil {
				log.Error().Err(err).Str("dir", userDirectory).Msg("error removing media directory")
			}
		}

		// 5. Remove files from S3 (if enabled)
		var s3Enabled bool
		err = s.db.QueryRow("SELECT s3_enabled FROM users WHERE id = $1", id).Scan(&s3Enabled)
		if err == nil && s3Enabled {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			errS3 := GetS3Manager().DeleteAllUserObjects(ctx, id)
			if errS3 != nil {
				log.Error().Err(errS3).Str("id", id).Msg("error removing user files from S3")
			} else {
				log.Info().Str("id", id).Msg("user files from S3 removed successfully")
			}
		}

		log.Info().Str("id", id).Str("name", uname).Str("jid", jid).Msg("user deleted successfully")

		// Success response
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"code": http.StatusOK,
			"data": map[string]interface{}{
				"id":   id,
				"name": uname,
				"jid":  jid,
			},
			"success": true,
			"details": "user instance removed completely",
		})
	}
}

// Respond to client
func (s *server) Respond(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	dataenvelope := map[string]interface{}{"code": status}
	if err, ok := data.(error); ok {
		dataenvelope["error"] = err.Error()
		dataenvelope["success"] = false
	} else {
		// Try to unmarshal into a map first
		var mydata map[string]interface{}
		if err := json.Unmarshal([]byte(data.(string)), &mydata); err == nil {
			dataenvelope["data"] = mydata
		} else {
			// If unmarshaling into a map fails, try as a slice
			var mySlice []interface{}
			if err := json.Unmarshal([]byte(data.(string)), &mySlice); err == nil {
				dataenvelope["data"] = mySlice
			} else {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("error unmarshalling JSON")
			}
		}
		dataenvelope["success"] = true
	}

	if err := json.NewEncoder(w).Encode(dataenvelope); err != nil {
		panic("respond: " + err.Error())
	}
}

// Validate message fields
func validateMessageFields(phone string, stanzaid *string, participant *string) (types.JID, error) {

	recipient, ok := parseJID(phone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("could not parse Phone")
	}

	if stanzaid != nil {
		if participant == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing Participant in ContextInfo")
		}
	}

	if participant != nil {
		if stanzaid == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing StanzaID in ContextInfo")
		}
	}

	return recipient, nil
}

// Set history
func (s *server) SetHistory() http.HandlerFunc {
	type historyStruct struct {
		History int `json:"history"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		// Check if client exists and is connected

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t historyStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		// Validate history value
		if t.History < 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("history cannot be negative"))
			return
		}

		// Store history configuration in database
		_, err = s.db.Exec("UPDATE users SET history = $1 WHERE id = $2", t.History, txtid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to save history configuration"))
			return
		}

		token := r.Context().Value("userinfo").(Values).Get("Token")
		if cachedUserInfo, found := userinfocache.Get(token); found {
			updatedUserInfo := cachedUserInfo.(Values)
			// Update history in cache
			updatedUserInfo = updateUserInfo(updatedUserInfo, "History", strconv.Itoa(t.History)).(Values)
			userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
			log.Info().Str("userID", txtid).Msg("User info cache updated with History configuration")
		}

		response := map[string]interface{}{
			"Details": "History configured successfully",
			"History": t.History,
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Set proxy
func (s *server) SetProxy() http.HandlerFunc {
	type proxyStruct struct {
		ProxyURL string `json:"proxy_url"` // Format: "socks5://user:pass@host:port" or "http://host:port"
		Enable   bool   `json:"enable"`    // Whether to enable or disable proxy
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		// Check if client exists and is connected

		if clientManager.GetWhatsmeowClient(txtid) != nil && clientManager.GetWhatsmeowClient(txtid).IsConnected() {
			s.Respond(w, r, http.StatusBadRequest, errors.New("cannot set proxy while connected. Please disconnect first"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t proxyStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		// If enable is false, remove proxy configuration
		if !t.Enable {
			_, err = s.db.Exec("UPDATE users SET proxy_url = '' WHERE id = $1", txtid)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to remove proxy configuration"))
				return
			}

			token := r.Context().Value("userinfo").(Values).Get("Token")
			if cachedUserInfo, found := userinfocache.Get(token); found {
				updatedUserInfo := cachedUserInfo.(Values)
				// Update proxy in cache
				updatedUserInfo = updateUserInfo(updatedUserInfo, "Proxy", "").(Values)
				userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
				log.Info().Str("userID", txtid).Msg("User info cache updated with Proxy configuration")
			}

			response := map[string]interface{}{"Details": "Proxy disabled successfully"}
			responseJson, err := json.Marshal(response)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
			} else {
				s.Respond(w, r, http.StatusOK, string(responseJson))
			}
			return
		}

		// Validate proxy URL
		if t.ProxyURL == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing proxy_url in payload"))
			return
		}

		proxyURL, err := url.Parse(t.ProxyURL)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid proxy URL format"))
			return
		}

		// Only allow http and socks5 proxies
		if proxyURL.Scheme != "http" && proxyURL.Scheme != "socks5" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("only HTTP and SOCKS5 proxies are supported"))
			return
		}

		// Store proxy configuration in database
		_, err = s.db.Exec("UPDATE users SET proxy_url = $1 WHERE id = $2", t.ProxyURL, txtid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to save proxy configuration"))
			return
		}

		token := r.Context().Value("userinfo").(Values).Get("Token")
		if cachedUserInfo, found := userinfocache.Get(token); found {
			updatedUserInfo := cachedUserInfo.(Values)
			// Update proxy in cache
			updatedUserInfo = updateUserInfo(updatedUserInfo, "Proxy", t.ProxyURL).(Values)
			userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
			log.Info().Str("userID", txtid).Msg("User info cache updated with Proxy configuration")
		}

		response := map[string]interface{}{
			"Details":  "Proxy configured successfully",
			"ProxyURL": t.ProxyURL,
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Configure S3
func (s *server) ConfigureS3() http.HandlerFunc {
	type s3ConfigStruct struct {
		Enabled       bool   `json:"enabled"`
		Endpoint      string `json:"endpoint"`
		Region        string `json:"region"`
		Bucket        string `json:"bucket"`
		AccessKey     string `json:"access_key"`
		SecretKey     string `json:"secret_key"`
		PathStyle     bool   `json:"path_style"`
		PublicURL     string `json:"public_url"`
		MediaDelivery string `json:"media_delivery"`
		RetentionDays int    `json:"retention_days"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		decoder := json.NewDecoder(r.Body)
		var t s3ConfigStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		// Validate media_delivery
		if t.MediaDelivery != "" && t.MediaDelivery != "base64" && t.MediaDelivery != "s3" && t.MediaDelivery != "both" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("media_delivery must be 'base64', 's3', or 'both'"))
			return
		}

		if t.MediaDelivery == "" {
			t.MediaDelivery = "base64"
		}

		// Update database
		_, err = s.db.Exec(`
			UPDATE users SET 
				s3_enabled = $1,
				s3_endpoint = $2,
				s3_region = $3,
				s3_bucket = $4,
				s3_access_key = $5,
				s3_secret_key = $6,
				s3_path_style = $7,
				s3_public_url = $8,
				media_delivery = $9,
				s3_retention_days = $10
			WHERE id = $11`,
			t.Enabled, t.Endpoint, t.Region, t.Bucket, t.AccessKey, t.SecretKey,
			t.PathStyle, t.PublicURL, t.MediaDelivery, t.RetentionDays, txtid)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to save S3 configuration"))
			return
		}

		// Initialize S3 client if enabled
		if t.Enabled {
			s3Config := &S3Config{
				Enabled:       t.Enabled,
				Endpoint:      t.Endpoint,
				Region:        t.Region,
				Bucket:        t.Bucket,
				AccessKey:     t.AccessKey,
				SecretKey:     t.SecretKey,
				PathStyle:     t.PathStyle,
				PublicURL:     t.PublicURL,
				RetentionDays: t.RetentionDays,
			}

			err = GetS3Manager().InitializeS3Client(txtid, s3Config)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to initialize S3 client: %v", err)))
				return
			}
		} else {
			GetS3Manager().RemoveClient(txtid)
		}

		// Update userinfocache with S3 configuration
		token := r.Context().Value("userinfo").(Values).Get("Token")
		if cachedUserInfo, found := userinfocache.Get(token); found {
			updatedUserInfo := cachedUserInfo.(Values)

			// Update S3-related fields in cache
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3Enabled", strconv.FormatBool(t.Enabled)).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3Endpoint", t.Endpoint).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3Region", t.Region).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3Bucket", t.Bucket).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3AccessKey", t.AccessKey).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3SecretKey", t.SecretKey).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3PathStyle", strconv.FormatBool(t.PathStyle)).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3PublicURL", t.PublicURL).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "MediaDelivery", t.MediaDelivery).(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "S3RetentionDays", strconv.Itoa(t.RetentionDays)).(Values)

			userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
			log.Info().Str("userID", txtid).Msg("User info cache updated with S3 configuration")
		}

		response := map[string]interface{}{
			"Details": "S3 configuration saved successfully",
			"Enabled": t.Enabled,
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Get S3 Configuration
func (s *server) GetS3Config() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		var config struct {
			Enabled       bool   `json:"enabled" db:"enabled"`
			Endpoint      string `json:"endpoint" db:"endpoint"`
			Region        string `json:"region" db:"region"`
			Bucket        string `json:"bucket" db:"bucket"`
			AccessKey     string `json:"access_key" db:"access_key"`
			PathStyle     bool   `json:"path_style" db:"path_style"`
			PublicURL     string `json:"public_url" db:"public_url"`
			MediaDelivery string `json:"media_delivery" db:"media_delivery"`
			RetentionDays int    `json:"retention_days" db:"retention_days"`
		}

		err := s.db.Get(&config, `
			SELECT 
				s3_enabled as enabled,
				s3_endpoint as endpoint,
				s3_region as region,
				s3_bucket as bucket,
				s3_access_key as access_key,
				s3_path_style as path_style,
				s3_public_url as public_url,
				media_delivery,
				s3_retention_days as retention_days
			FROM users WHERE id = $1`, txtid)

		if err != nil {
			log.Error().Err(err).Str("userID", txtid).Msg("Failed to get S3 configuration from database")
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to get S3 configuration"))
			return
		}

		log.Debug().Str("userID", txtid).Bool("enabled", config.Enabled).Str("endpoint", config.Endpoint).Str("bucket", config.Bucket).Msg("Retrieved S3 configuration from database")

		// Don't return secret key for security
		config.AccessKey = "***" // Mask access key

		responseJson, err := json.Marshal(config)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Test S3 Connection
func (s *server) TestS3Connection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		// Get S3 config from database
		var config struct {
			Enabled       bool   `db:"enabled"`
			Endpoint      string `db:"endpoint"`
			Region        string `db:"region"`
			Bucket        string `db:"bucket"`
			AccessKey     string `db:"access_key"`
			SecretKey     string `db:"secret_key"`
			PathStyle     bool   `db:"path_style"`
			PublicURL     string `db:"public_url"`
			RetentionDays int    `db:"retention_days"`
		}

		err := s.db.Get(&config, `
			SELECT 
				s3_enabled as enabled,
				s3_endpoint as endpoint,
				s3_region as region,
				s3_bucket as bucket,
				s3_access_key as access_key,
				s3_secret_key as secret_key,
				s3_path_style as path_style,
				s3_public_url as public_url,
				s3_retention_days as retention_days
			FROM users WHERE id = $1`, txtid)

		if err != nil {
			log.Error().Err(err).Str("userID", txtid).Msg("Failed to get S3 configuration from database for test connection")
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to get S3 configuration"))
			return
		}

		log.Debug().Str("userID", txtid).Bool("enabled", config.Enabled).Str("endpoint", config.Endpoint).Str("bucket", config.Bucket).Msg("Retrieved S3 configuration from database for test connection")

		if !config.Enabled {
			s.Respond(w, r, http.StatusBadRequest, errors.New("S3 is not enabled for this user"))
			return
		}

		// Initialize S3 client
		s3Config := &S3Config{
			Enabled:       config.Enabled,
			Endpoint:      config.Endpoint,
			Region:        config.Region,
			Bucket:        config.Bucket,
			AccessKey:     config.AccessKey,
			SecretKey:     config.SecretKey,
			PathStyle:     config.PathStyle,
			PublicURL:     config.PublicURL,
			RetentionDays: config.RetentionDays,
		}

		err = GetS3Manager().InitializeS3Client(txtid, s3Config)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to initialize S3 client: %v", err)))
			return
		}

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = GetS3Manager().TestConnection(ctx, txtid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("S3 connection test failed: %v", err)))
			return
		}

		response := map[string]interface{}{
			"Details": "S3 connection test successful",
			"Bucket":  config.Bucket,
			"Region":  config.Region,
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Delete S3 Configuration
func (s *server) DeleteS3Config() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		// Update database to remove S3 configuration
		_, err := s.db.Exec(`
			UPDATE users SET 
				s3_enabled = false,
				s3_endpoint = '',
				s3_region = '',
				s3_bucket = '',
				s3_access_key = '',
				s3_secret_key = '',
				s3_path_style = true,
				s3_public_url = '',
				media_delivery = 'base64',
				s3_retention_days = 30
			WHERE id = $1`, txtid)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to delete S3 configuration"))
			return
		}

		// Remove S3 client
		GetS3Manager().RemoveClient(txtid)

		response := map[string]interface{}{"Details": "S3 configuration deleted successfully"}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// Get chat history
func (s *server) GetHistory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		historyStr := r.Context().Value("userinfo").(Values).Get("History")
		historyLimit, _ := strconv.Atoi(historyStr)

		// Debug logging
		log.Info().Str("userId", txtid).Str("historyStr", historyStr).Int("historyLimit", historyLimit).Msg("GetHistory debug info")

		if historyLimit == 0 {
			// Before returning error, try refreshing the cache in case the DB was updated
			token := r.Context().Value("userinfo").(Values).Get("Token")
			log.Info().Str("userId", txtid).Str("token", token).Msg("History is 0, invalidating cache and trying fresh DB lookup")
			userinfocache.Delete(token)

			// Re-fetch from database
			var newHistoryValue sql.NullInt64
			err := s.db.QueryRow("SELECT COALESCE(history, 0) FROM users WHERE id = $1", txtid).Scan(&newHistoryValue)
			if err != nil {
				log.Error().Err(err).Str("userId", txtid).Msg("Failed to fetch history from database")
			} else {
				newHistoryLimit := int(newHistoryValue.Int64)
				log.Info().Str("userId", txtid).Int("newHistoryLimit", newHistoryLimit).Msg("Fresh DB lookup result")
				if newHistoryLimit > 0 {
					// Update the context for this request
					historyLimit = newHistoryLimit
					log.Info().Str("userId", txtid).Int("historyLimit", historyLimit).Msg("Using fresh history value from DB")
				}
			}

			if historyLimit == 0 {
				s.Respond(w, r, http.StatusNotImplemented, errors.New("message history is disabled for this user"))
				return
			}
		}
		chatJID := r.URL.Query().Get("chat_jid")
		if chatJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("chat_jid is required"))
			return
		}

		// If chat_jid is "index", return mapping of all instances to their chat_jids
		if chatJID == "index" {
			var query string
			if s.db.DriverName() == "postgres" {
				query = `
					SELECT user_id, chat_jid, MAX(timestamp) as last_message_time
					FROM message_history 
					GROUP BY user_id, chat_jid 
					ORDER BY user_id, last_message_time DESC`
			} else { // sqlite
				query = `
					SELECT user_id, chat_jid, MAX(timestamp) as last_message_time
					FROM message_history 
					GROUP BY user_id, chat_jid 
					ORDER BY user_id, last_message_time DESC`
			}

			type ChatMapping struct {
				UserID          string `json:"user_id" db:"user_id"`
				ChatJID         string `json:"chat_jid" db:"chat_jid"`
				LastMessageTime string `json:"last_message_time" db:"last_message_time"`
			}

			var mappings []ChatMapping
			err := s.db.Select(&mappings, query)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get chat mappings: %w", err))
				return
			}

			// Build the response map with chats ordered by most recent message
			type ChatInfo struct {
				ChatJID     string `json:"chat_jid"`
				LastUpdated string `json:"last_updated"`
			}

			result := make(map[string][]ChatInfo)
			for _, mapping := range mappings {
				// Parse the timestamp and format it properly to remove monotonic clock info
				var formattedTime string
				if parsedTime, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", mapping.LastMessageTime); err == nil {
					formattedTime = parsedTime.Format(time.RFC3339Nano)
				} else if parsedTime, err := time.Parse(time.RFC3339Nano, mapping.LastMessageTime); err == nil {
					formattedTime = parsedTime.Format(time.RFC3339Nano)
				} else {
					// If parsing fails, clean up the monotonic clock part manually
					formattedTime = strings.Split(mapping.LastMessageTime, " m=")[0]
				}

				chatInfo := ChatInfo{
					ChatJID:     mapping.ChatJID,
					LastUpdated: formattedTime,
				}
				result[mapping.UserID] = append(result[mapping.UserID], chatInfo)
			}

			responseJson, err := json.Marshal(result)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
			} else {
				s.Respond(w, r, http.StatusOK, string(responseJson))
			}
			return
		}

		limitStr := r.URL.Query().Get("limit")
		limit := 50 // Default limit
		if limitStr != "" {
			var err error
			limit, err = strconv.Atoi(limitStr)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("invalid limit"))
				return
			}
		}

		var query string
		if s.db.DriverName() == "postgres" {
			query = `
                SELECT id, user_id, chat_jid, sender_jid, message_id, timestamp, message_type, text_content, media_link, COALESCE(quoted_message_id, '') as quoted_message_id, COALESCE(datajson, '') as datajson
                FROM message_history
                WHERE user_id = $1 AND chat_jid = $2
                ORDER BY timestamp DESC
                LIMIT $3`
		} else { // sqlite
			query = `
                SELECT id, user_id, chat_jid, sender_jid, message_id, timestamp, message_type, text_content, media_link, COALESCE(quoted_message_id, '') as quoted_message_id, COALESCE(datajson, '') as datajson
                FROM message_history
                WHERE user_id = ? AND chat_jid = ?
                ORDER BY timestamp DESC
                LIMIT ?`
		}

		var messages []HistoryMessage
		err := s.db.Select(&messages, query, txtid, chatJID, limit)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get message history: %w", err))
			return
		}

		responseJson, err := json.Marshal(messages)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// syncHistoryForChat syncs history for a specific chat
func (s *server) syncHistoryForChat(ctx context.Context, userID string, chatJID types.JID, count int) error {
	chatJIDStr := chatJID.String()

	// Try to get last message info for this chat from database
	var query string
	if s.db.DriverName() == "postgres" {
		query = `
			SELECT message_id, chat_jid, sender_jid
			FROM message_history
			WHERE user_id = $1 AND chat_jid = $2
			ORDER BY timestamp DESC
			LIMIT 1`
	} else {
		query = `
			SELECT message_id, chat_jid, sender_jid
			FROM message_history
			WHERE user_id = ? AND chat_jid = ?
			ORDER BY timestamp DESC
			LIMIT 1`
	}

	var lastMsg struct {
		MessageID string `db:"message_id"`
		ChatJID   string `db:"chat_jid"`
		SenderJID string `db:"sender_jid"`
	}

	var lastMessageInfo *types.MessageInfo
	err := s.db.Get(&lastMsg, query, userID, chatJIDStr)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to get last message from history: %w", err)
	}

	if err == nil && lastMsg.MessageID != "" {
		// Parse sender JID
		var senderJID types.JID
		if lastMsg.SenderJID != "" && lastMsg.SenderJID != "me" {
			var pErr error
			senderJID, pErr = types.ParseJID(lastMsg.SenderJID)
			if pErr != nil {
				log.Warn().Err(pErr).Str("senderJID", lastMsg.SenderJID).Msg("Failed to parse sender JID from history, using empty JID")
				senderJID = types.EmptyJID
			}
		} else {
			senderJID = types.EmptyJID
		}

		// MessageInfo embeds MessageSource which contains Chat, Sender, IsGroup
		lastMessageInfo = &types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:    chatJID,
				Sender:  senderJID,
				IsGroup: chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer,
			},
			ID: lastMsg.MessageID,
		}
	} else {
		// If no last message found, create MessageInfo with just the chat
		lastMessageInfo = &types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:    chatJID,
				IsGroup: chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer,
			},
		}
	}

	// Build history sync request
	historyMsg := clientManager.GetWhatsmeowClient(userID).BuildHistorySyncRequest(lastMessageInfo, count)
	if historyMsg == nil {
		return errors.New("failed to build history sync request")
	}

	// Send the history sync request
	myClient := clientManager.GetMyClient(userID)
	if myClient == nil || myClient.WAClient == nil || myClient.WAClient.Store == nil || myClient.WAClient.Store.ID == nil {
		return errors.New("client store not available")
	}

	_, err = clientManager.GetWhatsmeowClient(userID).SendMessage(
		ctx,
		myClient.WAClient.Store.ID.ToNonAD(),
		historyMsg,
		whatsmeow.SendRequestExtra{Peer: true},
	)

	if err != nil {
		log.Error().
			Str("userID", userID).
			Str("chatJID", chatJIDStr).
			Err(err).
			Msg("Failed to send WhatsApp history sync request")
		return fmt.Errorf("failed to send history sync request: %w", err)
	}

	log.Info().
		Str("userID", userID).
		Str("chatJID", chatJIDStr).
		Int("count", count).
		Msg("WhatsApp history sync request sent successfully")

	return nil
}

// save outgoing message to history
func (s *server) saveOutgoingMessageToHistory(userID, chatJID, messageID, messageType, textContent, mediaLink string, historyLimit int) {
	if historyLimit > 0 {
		err := s.saveMessageToHistory(userID, chatJID, "me", messageID, messageType, textContent, mediaLink, "", "")
		if err != nil {
			log.Error().Err(err).Msg("Failed to save outgoing message to history")
		} else {
			err = s.trimMessageHistory(userID, chatJID, historyLimit)
			if err != nil {
				log.Error().Err(err).Msg("Failed to trim message history")
			}
		}
	}
}

// Configure HMAC
func (s *server) ConfigureHmac() http.HandlerFunc {
	type hmacConfigStruct struct {
		HmacKey string `json:"hmac_key"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")

		decoder := json.NewDecoder(r.Body)
		var t hmacConfigStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode payload"))
			return
		}

		// Validate HMAC key (minimum 32 characters for security)
		if len(t.HmacKey) < 32 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("HMAC key must be at least 32 characters long"))
			return
		}

		// Encrypt HMAC key before storing
		encryptedHmacKey, err := encryptHMACKey(t.HmacKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt HMAC key")
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to encrypt HMAC key"))
			return
		}

		// Update database with ENCRYPTED key
		_, err = s.db.Exec(`
            UPDATE users SET hmac_key = $1 WHERE id = $2`,
			encryptedHmacKey, txtid)

		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("failed to save HMAC configuration"))
			return
		}

		if cachedUserInfo, found := userinfocache.Get(token); found {
			updatedUserInfo := cachedUserInfo.(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "HasHmac", "true").(Values)
			hmacKeyEncrypted := base64.StdEncoding.EncodeToString(encryptedHmacKey)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "HmacKeyEncrypted", hmacKeyEncrypted).(Values)
			userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
			log.Info().Str("userID", txtid).Msg("User info cache updated with HMAC configuration")
		}

		response := map[string]interface{}{
			"Details": "HMAC configuration saved successfully",
		}
		s.respondWithJSON(w, http.StatusOK, response)
	}
}

// Get HMAC Configuration
func (s *server) GetHmacConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		var hmacKey []byte
		err := s.db.QueryRow(`SELECT hmac_key FROM users WHERE id = $1`, txtid).Scan(&hmacKey)

		if err != nil {
			if err == sql.ErrNoRows {
				s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
					"hmac_key": "",
				})
				return
			}

			log.Error().Err(err).Str("userID", txtid).Msg("Failed to get HMAC configuration from database")
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "failed to get HMAC configuration",
			})
			return
		}

		log.Debug().Str("userID", txtid).Bool("hasKey", len(hmacKey) > 0).Msg("Retrieved HMAC configuration from database")

		response := map[string]interface{}{
			"hmac_key": "",
		}

		if len(hmacKey) > 0 {
			response["hmac_key"] = "***" // Mask HMAC key
		}

		s.respondWithJSON(w, http.StatusOK, response)
	}
}

// Delete HMAC Configuration
func (s *server) DeleteHmacConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token") // ← Pegar o token

		// Clear HMAC key
		_, err := s.db.Exec(`UPDATE users SET hmac_key = NULL WHERE id = $1`, txtid)

		if err != nil {
			s.respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "failed to delete HMAC configuration",
			})
			return
		}

		if cachedUserInfo, found := userinfocache.Get(token); found {
			updatedUserInfo := cachedUserInfo.(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "HasHmac", "false").(Values)
			updatedUserInfo = updateUserInfo(updatedUserInfo, "HmacKeyEncrypted", "").(Values)
			userinfocache.Set(token, updatedUserInfo, cache.NoExpiration)
			log.Info().Str("userID", txtid).Msg("User info cache updated - HMAC configuration removed")
		}

		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"Details": "HMAC configuration deleted successfully",
		})
	}
}

// RejectCall rejects an incoming call
func (s *server) RejectCall() http.HandlerFunc {

	type rejectCallStruct struct {
		CallFrom string `json:"call_from"`
		CallID   string `json:"call_id"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t rejectCallStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.CallFrom == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing call_from in Payload"))
			return
		}

		if t.CallID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing call_id in Payload"))
			return
		}

		callFrom, ok := parseJID(t.CallFrom)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse call_from"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).RejectCall(context.Background(), callFrom, t.CallID)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("error rejecting call: %v", err)))
			return
		}

		log.Info().Str("call_id", t.CallID).Str("call_from", t.CallFrom).Msg("Call rejected")
		response := map[string]interface{}{"Details": "Call rejected", "CallID": t.CallID}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}

// GetUserLID retrieves the Local ID (LID) for a given JID/Phone Number
func (s *server) GetUserLID() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// Get JID from URL parameter
		vars := mux.Vars(r)
		jidParam := vars["jid"]

		if jidParam == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing jid parameter"))
			return
		}

		// Parse the JID (phone number)
		jid, ok := parseJID(jidParam)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid jid format"))
			return
		}

		client := clientManager.GetWhatsmeowClient(txtid)

		// Get the LID for this phone number from the store
		lid, err := client.Store.LIDs.GetLIDForPN(context.Background(), jid)
		if err != nil {
			log.Error().Err(err).Str("jid", jidParam).Msg("Failed to get LID for phone number")
			s.Respond(w, r, http.StatusNotFound, errors.New(fmt.Sprintf("LID not found for this number: %v", err)))
			return
		}

		if lid.IsEmpty() {
			s.Respond(w, r, http.StatusNotFound, errors.New("LID not found for this number"))
			return
		}

		// Return the LID
		response := map[string]interface{}{
			"jid": jid.String(),
			"lid": lid.String(),
		}

		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// RequestUnavailableMessage requests a copy of a message that couldn't be decrypted
func (s *server) RequestUnavailableMessage() http.HandlerFunc {

	type requestUnavailableMessageStruct struct {
		Chat   string `json:"chat"`   // Chat JID (e.g., "5511999999999@s.whatsapp.net" or "120363123456789012@g.us")
		Sender string `json:"sender"` // Sender JID (e.g., "5511999999999@s.whatsapp.net")
		ID     string `json:"id"`     // Message ID
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		client := clientManager.GetWhatsmeowClient(txtid)

		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t requestUnavailableMessageStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		// Validate required fields
		if t.Chat == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Chat in Payload"))
			return
		}

		if t.Sender == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Sender in Payload"))
			return
		}

		if t.ID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing ID in Payload"))
			return
		}

		// Parse JIDs
		chatJID, err := types.ParseJID(t.Chat)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid Chat JID format"))
			return
		}

		senderJID, err := types.ParseJID(t.Sender)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid Sender JID format"))
			return
		}

		// Build the unavailable message request
		unavailableMessage := client.BuildUnavailableMessageRequest(chatJID, senderJID, t.ID)

		// Send the request with Peer: true as required by the documentation
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.SendMessage(ctx, chatJID, unavailableMessage, whatsmeow.SendRequestExtra{Peer: true})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to send unavailable message request: %s", err)))
			return
		}

		response := map[string]interface{}{
			"success":    true,
			"message":    "Unavailable message request sent successfully",
			"request_id": resp.ID,
			"chat":       t.Chat,
			"sender":     t.Sender,
			"message_id": t.ID,
			"timestamp":  resp.Timestamp.Unix(),
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

func (s *server) ArchiveChat() http.HandlerFunc {

	type requestArchiveStruct struct {
		Jid     string `json:"jid"`
		Archive bool   `json:"archive"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		client := clientManager.GetWhatsmeowClient(txtid)

		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t requestArchiveStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		// Validate required fields
		if t.Jid == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing jid in Payload"))
			return
		}

		chatJID, err := types.ParseJID(t.Jid)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("invalid Chat JID format"))
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err = client.SendAppState(ctx, appstate.BuildArchive(chatJID, t.Archive, time.Time{}, nil))
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("failed to archive chat: %s", err)))
			return
		}
		statusText := "Chat archived"
		if !t.Archive {
			statusText = "Chat unarchived"
		}
		response := map[string]interface{}{
			"success": true,
			"message": statusText,
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}

}

// Downloads Sticker and returns base64 representation
func (s *server) DownloadSticker() http.HandlerFunc {

	type downloadStickerStruct struct {
		Url           string
		DirectPath    string
		MediaKey      []byte
		Mimetype      string
		FileEncSHA256 []byte
		FileSHA256    []byte
		FileLength    uint64
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		mimetype := ""
		var stickerdata []byte

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadStickerStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		msg := &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
			URL:           proto.String(t.Url),
			DirectPath:    proto.String(t.DirectPath),
			MediaKey:      t.MediaKey,
			Mimetype:      proto.String(t.Mimetype),
			FileEncSHA256: t.FileEncSHA256,
			FileSHA256:    t.FileSHA256,
			FileLength:    &t.FileLength,
		}}

		sticker := msg.GetStickerMessage()

		if sticker != nil {
			stickerdata, err = clientManager.GetWhatsmeowClient(txtid).Download(context.Background(), sticker)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to download sticker")
				msg := fmt.Sprintf("failed to download sticker %v", err)
				s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
				return
			}
			mimetype = sticker.GetMimetype()
		}

		dataURL := dataurl.New(stickerdata, mimetype)
		response := map[string]interface{}{"Mimetype": mimetype, "Data": dataURL.String()}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
		return
	}
}
