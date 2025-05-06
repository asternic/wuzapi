package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nfnt/resize"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type Values struct {
	m map[string]string
}

func (v Values) Get(key string) string {
	return v.m[key]
}

var messageTypes = []string{"Message", "ReadReceipt", "Presence", "HistorySync", "ChatPresence", "All"}

func (s *server) authadmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Obter token do cabeçalho Authorization
		authHeader := r.Header.Get("Authorization")
		var token string
		
		// Formatos aceitos: "Bearer TOKEN", "TOKEN", ou via query param "token=TOKEN"
		if strings.HasPrefix(authHeader, "Bearer ") {
			// Formato "Bearer TOKEN"
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if authHeader != "" {
			// Formato "TOKEN" diretamente
			token = authHeader
		} else {
			// Tentar via query param
			token = r.URL.Query().Get("token")
		}
		
		// Verificar se o token é válido
		if token == "" || token != *adminToken {
			// Adicionar cabeçalhos CORS para permitir requisições de diferentes origens
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, token, Instance-Id")
			
			s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
				"success": false,
				"error": "Não autorizado. Token de administrador requerido.",
			})
			return
		}
		
		// Adicionar informações de admin no contexto para rotas que precisam
		adminValues := Values{map[string]string{
			"Id":      "admin",
			"Token":   token,
			"Jid":     "",
			"Webhook": "",
			"Events":  "All",
		}}
		
		ctx := context.WithValue(r.Context(), "userinfo", adminValues)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *server) authalice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ctx context.Context
		userid := 0
		txtid := ""
		webhook := ""
		jid := ""
		events := ""

		// Get token from headers or uri parameters - aceita vários formatos
		var token string
		
		// 1. Verificar Authorization header (com ou sem Bearer)
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if authHeader != "" {
			token = authHeader
		}
		
		// 2. Verificar header "token" (formato legacy)
		if token == "" {
			token = r.Header.Get("token")
		}
		
		// 3. Verificar query parameter "token"
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		myuserinfo, found := userinfocache.Get(token)
		if !found {
			log.Info().Msg("Looking for user information in DB")
			// Checks DB from matching user and store user values in context
			rows, err := s.db.Query("SELECT id,webhook,jid,events FROM users WHERE token=$1 LIMIT 1", token)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				err = rows.Scan(&txtid, &webhook, &jid, &events)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, err)
					return
				}
				userid, _ = strconv.Atoi(txtid)
				v := Values{map[string]string{
					"Id":      txtid,
					"Jid":     jid,
					"Webhook": webhook,
					"Token":   token,
					"Events":  events,
				}}

				userinfocache.Set(token, v, cache.NoExpiration)
				ctx = context.WithValue(r.Context(), "userinfo", v)
			}
		} else {
			ctx = context.WithValue(r.Context(), "userinfo", myuserinfo)
			userid, _ = strconv.Atoi(myuserinfo.(Values).Get("Id"))
		}

		if userid == 0 {
			// Adicionar headers CORS para permitir requisições de diferentes origens
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, token")
			
			s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
				"success": false,
				"error": "Token inválido ou não fornecido",
			})
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Middleware: Authenticate connections based on Token header/uri parameter
func (s *server) auth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the token out of the request header
		token := r.Header.Get("token")
		
		// Check for Instance-Id header
		instanceId := r.Header.Get("Instance-Id")
		
		// Se não tem instanceId no header, verifica se tem na URL (para rotas /instances/{id}/...)
		if instanceId == "" {
			// Extrair ID da URL se for uma rota /instances/{id}/...
			pathParts := strings.Split(r.URL.Path, "/")
			for i, part := range pathParts {
				if part == "instances" && i+1 < len(pathParts) {
					// Verificar se o próximo segmento após "instances" é um ID
					potentialId := pathParts[i+1]
					if potentialId != "create" && potentialId != "" {
						instanceId = potentialId
						break
					}
				}
			}
		}
		
		if token == "" {
			s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
				"code":    http.StatusUnauthorized,
				"error":   "Token de autenticação necessário",
				"success": false,
			})
			return
		}
		
		// If we don't find the token in our cache, look it up in our database
		v, found := userinfocache.Get(token)
		if !found {
			userdetails, err := s.validateToken(token)
			if err != nil {
				s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
					"code":    http.StatusUnauthorized,
					"error":   "Token inválido",
					"success": false,
				})
				return
			}
			v := Values{map[string]string{
				"Id":      userdetails.Id,
				"Jid":     userdetails.Jid,
				"Webhook": userdetails.Webhook,
				"Token":   token,
				"Events":  userdetails.Events,
			}}
			userinfocache.Set(token, v, cache.NoExpiration)
		}
		
		// Se temos um instanceId específico mas não somos o admin, precisamos verificar se a instância pertence ao usuário
		if instanceId != "" && token != *adminToken {
			userID := v.(Values).Get("Id")
			if userID != instanceId {
				// Verificar se o usuário admin tem acesso a esta instância
				s.Respond(w, r, http.StatusForbidden, map[string]interface{}{
					"code":    http.StatusForbidden,
					"error":   "Acesso negado a esta instância",
					"success": false,
				})
				return
			}
		}
		
		ctx := context.WithValue(r.Context(), "userinfo", v)
		handler(w, r.WithContext(ctx))
	}
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
		userid, _ := strconv.Atoi(txtid)
		eventstring := ""

		// Decodes request BODY looking for events to subscribe
		decoder := json.NewDecoder(r.Body)
		var t connectStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if clientPointer[userid] != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("Already Connected"))
			return
		} else {

			var subscribedEvents []string
			if len(t.Subscribe) < 1 {
				if !Find(subscribedEvents, "All") {
					subscribedEvents = append(subscribedEvents, "All")
				}
			} else {
				for _, arg := range t.Subscribe {
					if !Find(messageTypes, arg) {
						log.Warn().Str("Type", arg).Msg("Message type discarded")
						continue
					}
					if !Find(subscribedEvents, arg) {
						subscribedEvents = append(subscribedEvents, arg)
					}
				}
			}
			eventstring = strings.Join(subscribedEvents, ",")
			_, err = s.db.Exec("UPDATE users SET events=$1 WHERE id=$2", eventstring, userid)
			if err != nil {
				log.Warn().Msg("Could not set events in users table")
			}
			log.Info().Str("events", eventstring).Msg("Setting subscribed events")
			v := updateUserInfo(r.Context().Value("userinfo"), "Events", eventstring)
			userinfocache.Set(token, v, cache.NoExpiration)

			log.Info().Str("jid", jid).Msg("Attempt to connect")
			killchannel[userid] = make(chan bool)
			go s.startClient(userid, jid, token, subscribedEvents)

			if t.Immediate == false {
				log.Warn().Msg("Waiting 10 seconds")
				time.Sleep(10000 * time.Millisecond)

				if clientPointer[userid] != nil {
					if !clientPointer[userid].IsConnected() {
						s.Respond(w, r, http.StatusInternalServerError, errors.New("Failed to Connect"))
						return
					}
				} else {
					s.Respond(w, r, http.StatusInternalServerError, errors.New("Failed to Connect"))
					return
				}
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}
		if clientPointer[userid].IsConnected() == true {
			if clientPointer[userid].IsLoggedIn() == true {
				log.Info().Str("jid", jid).Msg("Disconnection successfull")
				killchannel[userid] <- true
				_, err := s.db.Exec("UPDATE users SET events=$1 WHERE id=$2", "", userid)
				if err != nil {
					log.Warn().Str("userid", txtid).Msg("Could not set events in users table")
				}
				v := updateUserInfo(r.Context().Value("userinfo"), "Events", "")
				userinfocache.Set(token, v, cache.NoExpiration)

				response := map[string]interface{}{"Details": "Disconnected"}
				responseJson, err := json.Marshal(response)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, err)
				} else {
					s.Respond(w, r, http.StatusOK, string(responseJson))
				}
				return
			} else {
				log.Warn().Str("jid", jid).Msg("Ignoring disconnect as it was not connected")
				s.Respond(w, r, http.StatusInternalServerError, errors.New("Cannot disconnect because it is not logged in"))
				return
			}
		} else {
			log.Warn().Str("jid", jid).Msg("Ignoring disconnect as it was not connected")
			s.Respond(w, r, http.StatusInternalServerError, errors.New("Cannot disconnect because it is not logged in"))
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
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not get webhook: %v", err)))
			return
		}
		defer rows.Close()
		for rows.Next() {
			err = rows.Scan(&webhook, &events)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not get webhook: %s", fmt.Sprintf("%s", err))))
				return
			}
		}
		err = rows.Err()
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not get webhook: %s", fmt.Sprintf("%s", err))))
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
		userid, _ := strconv.Atoi(txtid)

		// Update the database to remove the webhook and clear events
		_, err := s.db.Exec("UPDATE users SET webhook='', events='' WHERE id=$1", userid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not delete webhook: %v", err)))
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
		Events     []string `json:"events"`
		Active     bool     `json:"active"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")
		userid, _ := strconv.Atoi(txtid)

		decoder := json.NewDecoder(r.Body)
		var t updateWebhookStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode payload"))
			return
		}

		webhook := t.WebhookURL
		events := strings.Join(t.Events, ",")
		if !t.Active {
			webhook = ""
			events = ""
		}

		_, err = s.db.Exec("UPDATE users SET webhook=?, events=? WHERE id=?", webhook, events, userid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not update webhook: %v", err)))
			return
		}

		v := updateUserInfo(r.Context().Value("userinfo"), "Webhook", webhook)
		v = updateUserInfo(v, "Events", events)
		userinfocache.Set(token, v, cache.NoExpiration)

		response := map[string]interface{}{"webhook": webhook, "events": t.Events, "active": t.Active}
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
		WebhookURL string   `json:"webhook"`
		Events     []string `json:"events"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		token := r.Context().Value("userinfo").(Values).Get("Token")
		userid, _ := strconv.Atoi(txtid)

		decoder := json.NewDecoder(r.Body)
		var t webhookStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode payload"))
			return
		}

		webhook := t.WebhookURL
		events := strings.Join(t.Events, ",")

		_, err = s.db.Exec("UPDATE users SET webhook=$1, events=$2 WHERE id=$3", webhook, events, userid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not set webhook: %v", err)))
			return
		}

		v := updateUserInfo(r.Context().Value("userinfo"), "Webhook", webhook)
		v = updateUserInfo(v, "Events", events)
		userinfocache.Set(token, v, cache.NoExpiration)

		response := map[string]interface{}{"webhook": webhook, "events": t.Events}
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
		userid, _ := strconv.Atoi(txtid)
		code := ""

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		} else {
			if clientPointer[userid].IsConnected() == false {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("Not connected"))
				return
			}
			rows, err := s.db.Query("SELECT qrcode AS code FROM users WHERE id=$1 LIMIT 1", userid)
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
			if clientPointer[userid].IsLoggedIn() == true {
				s.Respond(w, r, http.StatusInternalServerError, errors.New("Already Loggedin"))
				return
			}
		}

		log.Info().Str("userid", txtid).Str("qrcode", code).Msg("Get QR successful")
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		} else {
			if clientPointer[userid].IsLoggedIn() == true && clientPointer[userid].IsConnected() == true {
				err := clientPointer[userid].Logout()
				if err != nil {
					log.Error().Str("jid", jid).Msg("Could not perform logout")
					s.Respond(w, r, http.StatusInternalServerError, errors.New("Could not perform logout"))
					return
				} else {
					log.Info().Str("jid", jid).Msg("Logged out")
					killchannel[userid] <- true
				}
			} else {
				if clientPointer[userid].IsConnected() == true {
					log.Warn().Str("jid", jid).Msg("Ignoring logout as it was not logged in")
					s.Respond(w, r, http.StatusInternalServerError, errors.New("Could not disconnect as it was not logged in"))
					return
				} else {
					log.Warn().Str("jid", jid).Msg("Ignoring logout as it was not connected")
					s.Respond(w, r, http.StatusInternalServerError, errors.New("Could not disconnect as it was not connected"))
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t pairStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		isLoggedIn := clientPointer[userid].IsLoggedIn()
		if isLoggedIn {
			log.Error().Msg(fmt.Sprintf("%s", "Already paired"))
			s.Respond(w, r, http.StatusBadRequest, errors.New("Already paired"))
			return
		}

		linkingCode, err := clientPointer[userid].PairPhone(t.Phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
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

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		isConnected := clientPointer[userid].IsConnected()
		isLoggedIn := clientPointer[userid].IsLoggedIn()

		response := map[string]interface{}{"Connected": isConnected, "LoggedIn": isLoggedIn}
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
		ContextInfo waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t documentStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Document == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Document in Payload"))
			return
		}

		if t.FileName == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing FileName in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte

		if t.Document[0:29] == "data:application/octet-stream" {
			dataURL, err := dataurl.DecodeString(t.Document)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
				uploaded, err = clientPointer[userid].Upload(context.Background(), filedata, whatsmeow.MediaDocument)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to upload file: %v", err)))
					return
				}
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Document data should start with \"data:application/octet-stream;base64,\""))
			return
		}

		msg := &waProto.Message{DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			FileName:      &t.FileName,
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(http.DetectContentType(filedata)),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			Caption:       proto.String(t.Caption),
		}}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waProto.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		ContextInfo waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t audioStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Audio == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Audio in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte

		if t.Audio[0:14] == "data:audio/ogg" {
			dataURL, err := dataurl.DecodeString(t.Audio)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
				uploaded, err = clientPointer[userid].Upload(context.Background(), filedata, whatsmeow.MediaAudio)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to upload file: %v", err)))
					return
				}
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Audio data should start with \"data:audio/ogg;base64,\""))
			return
		}

		ptt := true
		mime := "audio/ogg; codecs=opus"

		msg := &waProto.Message{AudioMessage: &waProto.AudioMessage{
			URL:        proto.String(uploaded.URL),
			DirectPath: proto.String(uploaded.DirectPath),
			MediaKey:   uploaded.MediaKey,
			//Mimetype:      proto.String(http.DetectContentType(filedata)),
			Mimetype:      &mime,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			PTT:           &ptt,
		}}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waProto.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		Phone       string
		Image       string
		Caption     string
		Id          string
		ContextInfo waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t imageStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Image == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Image in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte
		var thumbnailBytes []byte

		if t.Image[0:10] == "data:image" {
			dataURL, err := dataurl.DecodeString(t.Image)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
				uploaded, err = clientPointer[userid].Upload(context.Background(), filedata, whatsmeow.MediaImage)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to upload file: %v", err)))
					return
				}
			}

			// decode jpeg into image.Image
			reader := bytes.NewReader(filedata)
			img, _, err := image.Decode(reader)
			if err != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not decode image for thumbnail preparation: %v", err)))
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

		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Image data should start with \"data:image/png;base64,\""))
			return
		}

		msg := &waProto.Message{ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(t.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(http.DetectContentType(filedata)),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			JPEGThumbnail: thumbnailBytes,
		}}

		if t.ContextInfo.StanzaID != nil {
			if msg.ImageMessage.ContextInfo == nil {
				msg.ImageMessage.ContextInfo = &waProto.ContextInfo{
					StanzaID:      proto.String(*t.ContextInfo.StanzaID),
					Participant:   proto.String(*t.ContextInfo.Participant),
					QuotedMessage: &waProto.Message{Conversation: proto.String("")},
				}
			}
		}

		if t.ContextInfo.MentionedJID != nil {
			msg.ImageMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		Phone        string
		Sticker      string
		Id           string
		PngThumbnail []byte
		ContextInfo  waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t stickerStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Sticker == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Sticker in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte

		if t.Sticker[0:4] == "data" {
			dataURL, err := dataurl.DecodeString(t.Sticker)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
				uploaded, err = clientPointer[userid].Upload(context.Background(), filedata, whatsmeow.MediaImage)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to upload file: %v", err)))
					return
				}
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Data should start with \"data:mime/type;base64,\""))
			return
		}

		msg := &waProto.Message{StickerMessage: &waProto.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(http.DetectContentType(filedata)),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			PngThumbnail:  t.PngThumbnail,
		}}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waProto.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		ContextInfo   waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)
		msgid := ""
		var resp whatsmeow.SendResponse

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t imageStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Video == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Video in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var uploaded whatsmeow.UploadResponse
		var filedata []byte

		if t.Video[0:4] == "data" {
			dataURL, err := dataurl.DecodeString(t.Video)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
				uploaded, err = clientPointer[userid].Upload(context.Background(), filedata, whatsmeow.MediaVideo)
				if err != nil {
					s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to upload file: %v", err)))
					return
				}
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Data should start with \"data:mime/type;base64,\""))
			return
		}

		msg := &waProto.Message{VideoMessage: &waProto.VideoMessage{
			Caption:       proto.String(t.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(http.DetectContentType(filedata)),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(filedata))),
			JPEGThumbnail: t.JPEGThumbnail,
		}}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waProto.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		ContextInfo waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t contactStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}
		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}
		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Name in Payload"))
			return
		}
		if t.Vcard == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Vcard in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		msg := &waProto.Message{ContactMessage: &waProto.ContactMessage{
			DisplayName: &t.Name,
			Vcard:       &t.Vcard,
		}}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waProto.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		ContextInfo waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t locationStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}
		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}
		if t.Latitude == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Latitude in Payload"))
			return
		}
		if t.Longitude == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Longitude in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		msg := &waProto.Message{LocationMessage: &waProto.LocationMessage{
			DegreesLatitude:  &t.Latitude,
			DegreesLongitude: &t.Longitude,
			Name:             &t.Name,
		}}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waProto.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Title == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Title in Payload"))
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
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Phone"))
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var buttons []*waProto.ButtonsMessage_Button

		for _, item := range t.Buttons {
			buttons = append(buttons, &waProto.ButtonsMessage_Button{
				ButtonID:       proto.String(item.ButtonId),
				ButtonText:     &waProto.ButtonsMessage_Button_ButtonText{DisplayText: proto.String(item.ButtonText)},
				Type:           waProto.ButtonsMessage_Button_RESPONSE.Enum(),
				NativeFlowInfo: &waProto.ButtonsMessage_Button_NativeFlowInfo{},
			})
		}

		msg2 := &waProto.ButtonsMessage{
			ContentText: proto.String(t.Title),
			HeaderType:  waProto.ButtonsMessage_EMPTY.Enum(),
			Buttons:     buttons,
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, &waProto.Message{ViewOnceMessage: &waProto.FutureProofMessage{
			Message: &waProto.Message{
				ButtonsMessage: msg2,
			},
		}}, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
// https://github.com/tulir/whatsmeow/issues/305
func (s *server) SendList() http.HandlerFunc {

	type rowsStruct struct {
		RowId       string
		Title       string
		Description string
	}

	type sectionsStruct struct {
		Title string
		Rows  []rowsStruct
	}

	type listStruct struct {
		Phone       string
		Title       string
		Description string
		ButtonText  string
		FooterText  string
		Sections    []sectionsStruct
		Id          string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t listStruct
		err := decoder.Decode(&t)
		marshal, _ := json.Marshal(t)
		fmt.Println(string(marshal))
		if err != nil {
			fmt.Println(err)
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

		if t.Description == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Description in Payload"))
			return
		}

		if t.ButtonText == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing ButtonText in Payload"))
			return
		}

		if len(t.Sections) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Sections in Payload"))
			return
		}
		recipient, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Phone"))
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var sections []*waProto.ListMessage_Section

		for _, item := range t.Sections {
			var rows []*waProto.ListMessage_Row
			id := 1
			for _, row := range item.Rows {
				var idtext string
				if row.RowId == "" {
					idtext = strconv.Itoa(id)
				} else {
					idtext = row.RowId
				}
				rows = append(rows, &waProto.ListMessage_Row{
					RowID:       proto.String(idtext),
					Title:       proto.String(row.Title),
					Description: proto.String(row.Description),
				})
			}

			sections = append(sections, &waProto.ListMessage_Section{
				Title: proto.String(item.Title),
				Rows:  rows,
			})
		}
		msg1 := &waProto.ListMessage{
			Title:       proto.String(t.Title),
			Description: proto.String(t.Description),
			ButtonText:  proto.String(t.ButtonText),
			ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
			Sections:    sections,
			FooterText:  proto.String(t.FooterText),
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, &waProto.Message{
			ViewOnceMessage: &waProto.FutureProofMessage{
				Message: &waProto.Message{
					ListMessage: msg1,
				},
			}}, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		Phone       string
		Body        string
		Id          string
		ContextInfo waProto.ContextInfo
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Body == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Body in Payload"))
			return
		}

		recipient, err := validateMessageFields(t.Phone, t.ContextInfo.StanzaID, t.ContextInfo.Participant)
		if err != nil {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		//	msg := &waProto.Message{Conversation: &t.Body}

		msg := &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: &t.Body,
			},
		}

		if t.ContextInfo.StanzaID != nil {
			msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{
				StanzaID:      proto.String(*t.ContextInfo.StanzaID),
				Participant:   proto.String(*t.ContextInfo.Participant),
				QuotedMessage: &waProto.Message{Conversation: proto.String("")},
			}
		}
		if t.ContextInfo.MentionedJID != nil {
			if msg.ExtendedTextMessage.ContextInfo == nil {
				msg.ExtendedTextMessage.ContextInfo = &waProto.ContextInfo{}
			}
			msg.ExtendedTextMessage.ContextInfo.MentionedJID = t.ContextInfo.MentionedJID
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse
//var ts time.Time

		decoder := json.NewDecoder(r.Body)
		var t templateStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Content == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Content in Payload"))
			return
		}

		if t.Footer == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Footer in Payload"))
			return
		}

		if len(t.Buttons) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Buttons in Payload"))
			return
		}

		recipient, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Phone"))
			return
		}

		if t.Id == "" {
			msgid = whatsmeow.GenerateMessageID()
		} else {
			msgid = t.Id
		}

		var buttons []*waProto.HydratedTemplateButton

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
				buttons = append(buttons, &waProto.HydratedTemplateButton{
					HydratedButton: &waProto.HydratedTemplateButton_QuickReplyButton{
						QuickReplyButton: &waProto.HydratedQuickReplyButton{
							DisplayText: &text,
							Id:          proto.String(idtext),
						},
					},
				})
			case "url":
				text := item.DisplayText
				url := item.Url
				buttons = append(buttons, &waProto.HydratedTemplateButton{
					HydratedButton: &waProto.HydratedTemplateButton_UrlButton{
						UrlButton: &waProto.HydratedURLButton{
							DisplayText: &text,
							Url:         &url,
						},
					},
				})
			case "call":
				text := item.DisplayText
				phonenumber := item.PhoneNumber
				buttons = append(buttons, &waProto.HydratedTemplateButton{
					HydratedButton: &waProto.HydratedTemplateButton_CallButton{
						CallButton: &waProto.HydratedCallButton{
							DisplayText: &text,
							PhoneNumber: &phonenumber,
						},
					},
				})
			default:
				text := item.DisplayText
				buttons = append(buttons, &waProto.HydratedTemplateButton{
					HydratedButton: &waProto.HydratedTemplateButton_QuickReplyButton{
						QuickReplyButton: &waProto.HydratedQuickReplyButton{
							DisplayText: &text,
							Id:          proto.String(string(id)),
						},
					},
				})
			}
			id++
		}

		msg := &waProto.Message{TemplateMessage: &waProto.TemplateMessage{
			HydratedTemplate: &waProto.HydratedFourRowTemplate{
				HydratedContentText: proto.String(t.Content),
				HydratedFooterText:  proto.String(t.Footer),
				HydratedButtons:     buttons,
				TemplateId:          proto.String("1"),
			},
		},
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(),recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t checkUserStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		resp, err := clientPointer[userid].IsOnWhatsApp(t.Phone)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Failed to check if users are on WhatsApp: %s", err)))
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t checkUserStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
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
		resp, err := clientPointer[userid].GetUserInfo(jids)

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

func (s *server) SendPresence() http.HandlerFunc {

	type PresenceRequest struct {
		Type string `json:"type" form:"type"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var pre PresenceRequest
		err := decoder.Decode(&pre)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		var presence types.Presence

		switch pre.Type {
		case "available":
			presence = types.PresenceAvailable
		case "unavailable":
			presence = types.PresenceUnavailable
		default:
			s.Respond(w, r, http.StatusBadRequest, errors.New("Invalid presence type. Allowed values: 'available', 'unavailable'"))
			return
		}

		log.Info().Str("presence", pre.Type).Msg("Your global presence status")

		err = clientPointer[userid].SendPresence(presence)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("Failure sending presence to Whatsapp servers"))
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t getAvatarStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		jid, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Phone"))
			return
		}

		var pic *types.ProfilePictureInfo

		existingID := ""
		pic, err = clientPointer[userid].GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
			Preview:    t.Preview,
			ExistingID: existingID,
		})
		if err != nil {
			msg := fmt.Sprintf("Failed to get avatar: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
			return
		}

		if pic == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No avatar found"))
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		result := map[types.JID]types.ContactInfo{}
		result, err := clientPointer[userid].Store.Contacts.GetAllContacts()
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t chatPresenceStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if len(t.Phone) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if len(t.State) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing State in Payload"))
			return
		}

		jid, ok := parseJID(t.Phone)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Phone"))
			return
		}

		err = clientPointer[userid].SendChatPresence(jid, types.ChatPresence(t.State), types.ChatPresenceMedia(t.Media))
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("Failure sending chat presence to Whatsapp servers"))
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
		userid, _ := strconv.Atoi(txtid)

		mimetype := ""
		var imgdata []byte

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadImageStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		msg := &waProto.Message{ImageMessage: &waProto.ImageMessage{
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
			imgdata, err = clientPointer[userid].Download(img)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("Failed to download image")
				msg := fmt.Sprintf("Failed to download image %v", err)
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
		userid, _ := strconv.Atoi(txtid)

		mimetype := ""
		var docdata []byte

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadDocumentStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		msg := &waProto.Message{DocumentMessage: &waProto.DocumentMessage{
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
			docdata, err = clientPointer[userid].Download(doc)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("Failed to download document")
				msg := fmt.Sprintf("Failed to download document %v", err)
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
		userid, _ := strconv.Atoi(txtid)

		mimetype := ""
		var docdata []byte

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadVideoStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		msg := &waProto.Message{VideoMessage: &waProto.VideoMessage{
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
			docdata, err = clientPointer[userid].Download(doc)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("Failed to download video")
				msg := fmt.Sprintf("Failed to download video %v", err)
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
		userid, _ := strconv.Atoi(txtid)

		mimetype := ""
		var docdata []byte

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		// check/creates user directory for files
		userDirectory := filepath.Join(s.exPath, "files", "user_"+txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Could not create user directory (%s)", userDirectory)))
				return
			}
		}

		decoder := json.NewDecoder(r.Body)
		var t downloadAudioStruct
		err = decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		msg := &waProto.Message{AudioMessage: &waProto.AudioMessage{
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
			docdata, err = clientPointer[userid].Download(doc)
			if err != nil {
				log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("Failed to download audio")
				msg := fmt.Sprintf("Failed to download audio %v", err)
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
		Phone string
		Body  string
		Id    string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		msgid := ""
		var resp whatsmeow.SendResponse

		decoder := json.NewDecoder(r.Body)
		var t textStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Phone == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Phone in Payload"))
			return
		}

		if t.Body == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Body in Payload"))
			return
		}

		recipient, ok := parseJID(t.Phone)
		if !ok {
			log.Error().Msg(fmt.Sprintf("%s", err))
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Group JID"))
			return
		}

		if t.Id == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Id in Payload"))
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

		msg := &waProto.Message{
			ReactionMessage: &waProto.ReactionMessage{
				Key: &waProto.MessageKey{
					RemoteJID: proto.String(recipient.String()),
					FromMe:    proto.Bool(fromMe),
					ID:        proto.String(msgid),
				},
				Text:              proto.String(reaction),
				GroupingKey:       proto.String(reaction),
				SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
			},
		}

		resp, err = clientPointer[userid].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgid})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New(fmt.Sprintf("Error sending message: %v", err)))
			return
		}

		log.Info().Str("timestamp", fmt.Sprintf("%d", resp.Timestamp)).Str("id", msgid).Msg("Message sent")
		response := map[string]interface{}{"Details": "Sent", "Timestamp": resp.Timestamp, "Id": msgid}
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
		Id     []string
		Chat   types.JID
		Sender types.JID
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t markReadStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		if t.Chat.String() == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Chat in Payload"))
			return
		}

		if len(t.Id) < 1 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Id in Payload"))
			return
		}

		err = clientPointer[userid].MarkRead(t.Id, time.Now(), t.Chat, t.Sender)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("Failure marking messages as read"))
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		resp, err := clientPointer[userid].GetJoinedGroups()

		if err != nil {
			msg := fmt.Sprintf("Failed to get group list: %v", err)
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		// Get GroupJID from query parameter
		groupJID := r.URL.Query().Get("groupJID")
		if groupJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing groupJID parameter"))
			return
		}

		group, ok := parseJID(groupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Group JID"))
			return
		}

		resp, err := clientPointer[userid].GetGroupInfo(group)

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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		// Get GroupJID from query parameter
		groupJID := r.URL.Query().Get("groupJID")
		if groupJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing groupJID parameter"))
			return
		}

		// Get reset parameter
		resetParam := r.URL.Query().Get("reset")
		reset := false
		if resetParam != "" {
			var err error
			reset, err = strconv.ParseBool(resetParam)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Invalid reset parameter, must be true or false"))
				return
			}
		}
		group, ok := parseJID(groupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Group JID"))
			return
		}

		resp, err := clientPointer[userid].GetGroupInviteLink(group, reset)

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

// Set group photo
func (s *server) SetGroupPhoto() http.HandlerFunc {

	type setGroupPhotoStruct struct {
		GroupJID string
		Image    string
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupPhotoStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Group JID"))
			return
		}

		if t.Image == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Image in Payload"))
			return
		}

		var filedata []byte

		if t.Image[0:13] == "data:image/jp" {
			dataURL, err := dataurl.DecodeString(t.Image)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode base64 encoded data from payload"))
				return
			} else {
				filedata = dataURL.Data
			}
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Image data should start with \"data:image/jpeg;base64,\""))
			return
		}

		picture_id, err := clientPointer[userid].SetGroupPhoto(group, filedata)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("Failed to set group photo")
			msg := fmt.Sprintf("Failed to set group photo: %v", err)
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
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t setGroupNameStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not parse Group JID"))
			return
		}

		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing Name in Payload"))
			return
		}

		err = clientPointer[userid].SetGroupName(group, t.Name)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("Failed to set group name")
			msg := fmt.Sprintf("Failed to set group name: %v", err)
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

// List newsletters
func (s *server) ListNewsletter() http.HandlerFunc {

	type NewsletterCollection struct {
		Newsletter []types.NewsletterMetadata
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		if clientPointer[userid] == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("No session"))
			return
		}

		resp, err := clientPointer[userid].GetSubscribedNewsletters()

		if err != nil {
			msg := fmt.Sprintf("Failed to get newsletter list: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		gc := new(NewsletterCollection)
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
		Id         int          `db:"id" json:"id"`
		Name       string       `db:"name" json:"name"`
		Token      string       `db:"token" json:"token"`
		Webhook    string       `db:"webhook" json:"webhook,omitempty"`
		Jid        string       `db:"jid" json:"jid,omitempty"`
		Qrcode     string       `db:"qrcode" json:"qrcode,omitempty"`
		Connected  *int         `db:"connected" json:"connected"`
		Expiration *int         `db:"expiration" json:"expiration"`
		ProxyUrl   *string      `db:"proxy_url" json:"proxy_url,omitempty"`
		Events     string       `db:"events" json:"events,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Verificar caminho para adaptar o formato da resposta
		isInstancesEndpoint := strings.Contains(r.URL.Path, "/instances")
		isAdminEndpoint := strings.Contains(r.URL.Path, "/admin/users") || strings.Contains(r.URL.Path, "/admin/users/list")
		
		// Determinar se é uma requisição de admin direto
		isDirectAdminRequest := strings.Contains(r.URL.Path, "/admin/")
		var token string
		var userid string

		// Para requisições diretas de admin, usar o token do header Authorization
		if isDirectAdminRequest {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				token = authHeader
			}
			userid = "admin"
		} else {
			// Requisições normais (instâncias) usam o contexto
			ctx := r.Context()
			if ctx.Value("userinfo") == nil {
				s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
					"success": false,
					"error":   "Autenticação necessária",
				})
				return
			}
			v := ctx.Value("userinfo").(Values)
			userid = v.Get("Id")
			token = v.Get("Token")
		}
		
		var rows *sqlx.Rows
		var err error
		
		// Se for admin, pode ver todas as instâncias
		if token == *adminToken {
			rows, err = s.db.Queryx("SELECT id,name,token,webhook,jid,qrcode,connected,expiration,proxy_url,events FROM users")
		} else {
			// Caso contrário, ver apenas o próprio usuário
			rows, err = s.db.Queryx("SELECT id,name,token,webhook,jid,qrcode,connected,expiration,proxy_url,events FROM users WHERE id=$1", userid)
		}
		
		if err != nil {
			log.Error().Err(err).Msg("Erro ao consultar usuários")
			s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Erro ao consultar banco de dados: " + err.Error(),
			})
			return
		}
		defer rows.Close()
		
		var users []usersStruct
		for rows.Next() {
			var u usersStruct
			if err := rows.StructScan(&u); err != nil {
				log.Error().Err(err).Msg("Erro ao mapear dados do usuário")
				s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
					"success": false,
					"error":   "Erro ao processar dados: " + err.Error(),
				})
				return
			}
			
			// Para endpoints não-admin, não retornar o token por segurança
			if !isAdminEndpoint {
				u.Token = "" 
			}
			
			// Garantir que valores NULL sejam tratados adequadamente
			if u.Connected == nil {
				zero := 0
				u.Connected = &zero
			}
			
			if u.Expiration == nil {
				zero := 0
				u.Expiration = &zero
			}
			
			users = append(users, u)
		}
		
		if err := rows.Err(); err != nil {
			log.Error().Err(err).Msg("Erro durante iteração dos resultados")
			s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Erro ao processar resultados: " + err.Error(),
			})
			return
		}
		
		// Adaptar formato da resposta dependendo do endpoint
		if isInstancesEndpoint {
			// Formato para /instances - transformar para o formato que o frontend espera
			instances := make([]map[string]interface{}, 0)
			
			for _, u := range users {
				// Verificar status real de conexão pelo clientPointer
				isConnected := false
				isLoggedIn := false
				if clientPointer[u.Id] != nil {
					isConnected = clientPointer[u.Id].IsConnected()
					isLoggedIn = clientPointer[u.Id].IsLoggedIn()
				}
				
				// Obter valor do campo expiration, considerando que pode ser nulo
				var expiration int64
				if u.Expiration != nil {
					expiration = int64(*u.Expiration)
				}
				
				instance := map[string]interface{}{
					"id":        strconv.Itoa(u.Id),
					"name":      u.Name,
					"connected": isConnected,
					"loggedIn":  isLoggedIn,
					"phone":     extractPhoneFromJid(u.Jid),
					"webhook":   u.Webhook,
					"events":    strings.Split(u.Events, ","),
					"expiration": expiration,
				}
				instances = append(instances, instance)
			}
			
			s.Respond(w, r, http.StatusOK, map[string]interface{}{
				"success": true,
				"data":    instances,
			})
		} else {
			// Formato para /admin/users
			s.Respond(w, r, http.StatusOK, map[string]interface{}{
				"success": true,
				"data":    users,
			})
		}
	}
}

// Extrair número de telefone do JID
func extractPhoneFromJid(jid string) string {
	if jid == "" {
		return ""
	}
	
	// Verificar se há o caractere ":" que indica recursos específicos no JID
	colonIndex := strings.Index(jid, ":")
	if colonIndex > 0 {
		jid = jid[:colonIndex]
	}
	
	// Formato típico: "5511999999999@s.whatsapp.net"
	parts := strings.Split(jid, "@")
	if len(parts) > 0 {
		return parts[0]
	}
	
	return ""
}

func (s *server) AddUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var requestData struct {
			Name         string      `json:"name"`
			Token        string      `json:"token"` // Token é opcional para criação de instância
			Webhook      string      `json:"webhook"` // URL do webhook
			Events       interface{} `json:"events"` // Eventos para inscrição (aceita string ou array)
			Expiration   int64       `json:"expiration"` // Tempo de expiração
			ProxyUrl     string      `json:"proxy_url"` // URL do proxy
			ProxyEnabled bool        `json:"proxy_enabled"` // Se o proxy está habilitado
		}
		
		// Verificar se este é um endpoint /instances/create ou admin/user/create
		isInstancesCreate := strings.Contains(r.URL.Path, "/instances/create")
		isAdminCreate := strings.Contains(r.URL.Path, "/admin/user/create") || strings.Contains(r.URL.Path, "/admin/users")
		
		if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
			log.Error().Err(err).Msg("Erro ao decodificar payload")
			s.Respond(w, r, http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"error":   "Erro ao decodificar corpo da requisição: " + err.Error(),
			})
			return
		}
		
		// Obter informações do usuário do contexto
		var token string
		if ctx := r.Context(); ctx.Value("userinfo") != nil {
			v := ctx.Value("userinfo").(Values)
			token = v.Get("Token")
		} else if isAdminCreate {
			// Para criação direta via admin, verificar o token no header
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				token = authHeader
			}
			
			if token != *adminToken {
				s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
					"success": false,
					"error":   "Token de administrador inválido",
				})
				return
			}
		} else {
			s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
				"success": false,
				"error":   "Autenticação necessária",
			})
			return
		}
		
		// Validações básicas
		if requestData.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"error":   "Nome é obrigatório",
			})
			return
		}
		
		// Apenas o admin pode criar usuários com token específico
		if token != *adminToken && requestData.Token != "" && !isInstancesCreate {
			s.Respond(w, r, http.StatusForbidden, map[string]interface{}{
				"success": false,
				"error":   "Apenas admin pode definir tokens",
			})
			return
		}
		
		// Para criação de instância, gerar token aleatório se não fornecido
		// Tentar até 3 vezes para garantir que o token seja único
		maxAttempts := 3
		if requestData.Token == "" {
			for attempts := 0; attempts < maxAttempts; attempts++ {
				// Gerar um token aleatório
				requestData.Token = generateRandomToken(32)
				
				// Verificar se já existe token igual
				var count int
				err := s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE token = $1", requestData.Token)
				if err != nil {
					log.Error().Err(err).Msg("Erro ao verificar token existente")
					s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
						"success": false,
						"error":   "Erro ao verificar token: " + err.Error(),
					})
					return
				}
				
				// Se o token for único, podemos continuar
				if count == 0 {
					break
				}
				
				// Se chegamos à última tentativa e ainda não temos um token único
				if attempts == maxAttempts-1 {
					s.Respond(w, r, http.StatusConflict, map[string]interface{}{
						"success": false,
						"error":   "Não foi possível gerar um token único após várias tentativas",
					})
					return
				}
				
				// Limpar o token para a próxima tentativa
				requestData.Token = ""
			}
		} else {
			// Verificar se já existe token igual apenas se foi fornecido pelo usuário
			var count int
			err := s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE token = $1", requestData.Token)
			if err != nil {
				log.Error().Err(err).Msg("Erro ao verificar token existente")
				s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
					"success": false,
					"error":   "Erro ao verificar token: " + err.Error(),
				})
				return
			}
			
			if count > 0 {
				s.Respond(w, r, http.StatusConflict, map[string]interface{}{
					"success": false,
					"error":   "Já existe uma instância com este token",
				})
				return
			}
		}
		
		// Preparar os campos de eventos, suporta string ou array de strings
		eventsStr := "Message"
		
		// Verificar se Events é uma string ou um array
		switch events := requestData.Events.(type) {
		case string:
			if events != "" {
				eventsStr = events
			}
		case []interface{}:
			if len(events) > 0 {
				// Converter array de interface{} para array de strings
				strEvents := make([]string, len(events))
				for i, v := range events {
					if s, ok := v.(string); ok {
						strEvents[i] = s
					}
				}
				eventsStr = strings.Join(strEvents, ",")
			}
		case []string:
			if len(events) > 0 {
				eventsStr = strings.Join(events, ",")
			}
		}
		
		// Definir valor do proxy URL
		var proxyUrl sql.NullString
		if requestData.ProxyUrl != "" && requestData.ProxyEnabled {
			proxyUrl = sql.NullString{String: requestData.ProxyUrl, Valid: true}
		}
		
		// Definir valor de expiração
		expiration := 0
		if requestData.Expiration > 0 {
			expiration = int(requestData.Expiration)
		}
		
		// Inserir o novo usuário sem o campo webhook_active que não existe
		var newUserId int
		err := s.db.QueryRow(
			"INSERT INTO users (name, token, events, jid, qrcode, webhook, connected, expiration, proxy_url) VALUES ($1, $2, $3, '', '', $4, 0, $5, $6) RETURNING id",
			requestData.Name, requestData.Token, eventsStr, requestData.Webhook, expiration, proxyUrl,
		).Scan(&newUserId)
		
		if err != nil {
			log.Error().Err(err).Msg("Erro ao inserir novo usuário")
			s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Erro ao criar usuário: " + err.Error(),
			})
			return
		}
		
		// Log do token gerado para depuração
		log.Info().Str("token", requestData.Token).Int("userId", newUserId).Msg("Usuário criado com token")
		
		// Preparar resposta apropriada
		responseData := map[string]interface{}{
			"id":    strconv.Itoa(newUserId),
			"name":  requestData.Name,
			"token": requestData.Token,
		}
		
		// Adicionar campos extras na resposta se fornecidos
		if requestData.Webhook != "" {
			responseData["webhook"] = requestData.Webhook
			responseData["events"] = eventsStr
		}
		
		if requestData.ProxyUrl != "" && requestData.ProxyEnabled {
			responseData["proxy_url"] = requestData.ProxyUrl
			responseData["proxy_enabled"] = true
		}
		
		if expiration > 0 {
			responseData["expiration"] = expiration
		}
		
		if isInstancesCreate || isAdminCreate {
			s.Respond(w, r, http.StatusCreated, map[string]interface{}{
				"success": true,
				"data":    responseData,
			})
		} else {
			s.Respond(w, r, http.StatusCreated, map[string]interface{}{
				"success": true,
				"data":    responseData,
			})
		}
	}
}

func (s *server) DeleteUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the user ID from the request URL
		vars := mux.Vars(r)
		userID := vars["id"]
		
		// Verificar se é uma requisição de admin
		isDirectAdminRequest := strings.Contains(r.URL.Path, "/admin/user/")
		if isDirectAdminRequest {
			// Verificar o token de admin no header Authorization
			authHeader := r.Header.Get("Authorization")
			var token string
			
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				token = authHeader
			}
			
			if token != *adminToken {
				s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
					"success": false,
					"error":   "Token de administrador inválido",
				})
				return
			}
		} else {
			// Verificar se o usuário está autenticado via contexto
			ctx := r.Context()
			if ctx.Value("userinfo") == nil {
				s.Respond(w, r, http.StatusUnauthorized, map[string]interface{}{
					"success": false,
					"error":   "Autenticação necessária",
				})
				return
			}
			
			// Para requisições normais, verificar se o usuário tem permissão
			v := ctx.Value("userinfo").(Values)
			token := v.Get("Token")
			requestorID := v.Get("Id")
			
			// Apenas admin ou o próprio usuário podem excluir
			if token != *adminToken && requestorID != userID {
				s.Respond(w, r, http.StatusForbidden, map[string]interface{}{
					"success": false,
					"error":   "Sem permissão para excluir este usuário",
				})
				return
			}
		}
		
		// Verificar se o usuário existe
		var exists bool
		err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", userID).Scan(&exists)
		if err != nil {
			log.Error().Err(err).Msg("Erro ao verificar existência do usuário")
			s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Erro ao verificar usuário: " + err.Error(),
			})
			return
		}
		
		if !exists {
			s.Respond(w, r, http.StatusNotFound, map[string]interface{}{
				"success": false,
				"error":   "Instância não encontrada",
			})
			return
		}
		
		// Desconectar cliente WhatsApp se estiver conectado
		userIDInt, _ := strconv.Atoi(userID)
		if clientPointer[userIDInt] != nil {
			if clientPointer[userIDInt].IsConnected() {
				clientPointer[userIDInt].Disconnect()
			}
			delete(clientPointer, userIDInt)
		}
		
		// Delete the user from the database
		result, err := s.db.Exec("DELETE FROM users WHERE id=$1", userID)
		if err != nil {
			log.Error().Err(err).Msg("Erro ao excluir usuário")
			s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Erro ao excluir usuário: " + err.Error(),
			})
			return
		}
		
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Error().Err(err).Msg("Erro ao obter linhas afetadas")
			s.Respond(w, r, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Erro ao obter linhas afetadas: " + err.Error(),
			})
			return
		}
		
		if rowsAffected == 0 {
			s.Respond(w, r, http.StatusNotFound, map[string]interface{}{
				"success": false,
				"error":   "Instância não encontrada ou já removida",
			})
			return
		}
		
		// Verificar se é uma rota de instância
		isInstanceEndpoint := strings.Contains(r.URL.Path, "/instances/")
		
		if isInstanceEndpoint {
			s.Respond(w, r, http.StatusOK, map[string]interface{}{
				"success": true,
				"message": "Instância removida com sucesso",
			})
		} else {
			s.Respond(w, r, http.StatusOK, map[string]interface{}{
				"success": true,
				"message": "Usuário removido com sucesso",
			})
		}
	}
}

// Writes JSON response to API clients
func (s *server) Respond(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	var err error
	var response []byte

	// Verificar o tipo de dado recebido
	switch v := data.(type) {
	case string:
		// Se for uma string, assume que já está em formato JSON
		response = []byte(v)
	case error:
		// Se for um erro, criar um JSON com a mensagem de erro
		errResp := map[string]interface{}{
			"success": false,
			"code":    status,
			"error":   v.Error(),
		}
		response, err = json.Marshal(errResp)
		if err != nil {
			log.Error().Err(err).Msg("Erro ao serializar resposta de erro")
			w.Write([]byte(`{"success":false,"error":"Erro interno ao processar resposta"}`))
			return
		}
	default:
		// Para outros tipos (mapas, slices, etc), serializar para JSON
		response, err = json.Marshal(data)
		if err != nil {
			log.Error().Err(err).Msg("Erro ao serializar resposta")
			w.Write([]byte(`{"success":false,"error":"Erro interno ao processar resposta"}`))
			return
		}
	}

	w.Write(response)
}

func validateMessageFields(phone string, stanzaid *string, participant *string) (types.JID, error) {

	recipient, ok := parseJID(phone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("Could not parse Phone")
	}

	if stanzaid != nil {
		if participant == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("Missing Participant in ContextInfo")
		}
	}

	if participant != nil {
		if stanzaid == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("Missing StanzaID in ContextInfo")
		}
	}

	return recipient, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func (s *server) SetProxy() http.HandlerFunc {
	type proxyStruct struct {
		ProxyURL string `json:"proxy_url"` // Format: "socks5://user:pass@host:port" or "http://host:port"
		Enable   bool   `json:"enable"`    // Whether to enable or disable proxy
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		userid, _ := strconv.Atoi(txtid)

		decoder := json.NewDecoder(r.Body)
		var t proxyStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Could not decode Payload"))
			return
		}

		// Validate ProxyURL if enabled
		if t.Enable && t.ProxyURL == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Missing ProxyURL in Payload"))
			return
		}

		if t.Enable {
			// Validate proxy URL format
			_, err := url.Parse(t.ProxyURL)
			if err != nil {
				s.Respond(w, r, http.StatusBadRequest, errors.New("Invalid proxy URL format"))
				return
			}

			// Update proxy in database
			_, err = s.db.Exec("UPDATE users SET proxy_url=$1 WHERE id=$2", t.ProxyURL, userid)
			if err != nil {
				log.Error().Err(err).Msg("Could not set proxy_url in users table")
				s.Respond(w, r, http.StatusInternalServerError, errors.New("Failed to update proxy settings in database"))
				return
			}

			// Apply proxy to current session if active
			if clientPointer[userid] != nil && clientHttp[userid] != nil {
				clientHttp[userid].SetProxy(t.ProxyURL)
				log.Info().Str("proxy", t.ProxyURL).Msg("Proxy set for user")
			}
		} else {
			// Disable proxy
			_, err = s.db.Exec("UPDATE users SET proxy_url='' WHERE id=$1", userid)
			if err != nil {
				log.Error().Err(err).Msg("Could not clear proxy_url in users table")
				s.Respond(w, r, http.StatusInternalServerError, errors.New("Failed to update proxy settings in database"))
				return
			}

			// Remove proxy from current session if active
			if clientPointer[userid] != nil && clientHttp[userid] != nil {
				clientHttp[userid].RemoveProxy()
				log.Info().Msg("Proxy removed for user")
			}
		}

		response := map[string]interface{}{
			"Details": "Proxy settings updated successfully",
			"Enabled": t.Enable,
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

// generateRandomToken creates a random token string of specified length
func generateRandomToken(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	
	// Initialize the random source
	rand.Seed(time.Now().UnixNano())
	
	// Create the token
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	
	return string(b)
}

/**
 * Valida um token de usuário e retorna os detalhes
 */
type UserDetails struct {
	Id      string
	Jid     string
	Webhook string
	Events  string
}

func (s *server) validateToken(token string) (UserDetails, error) {
	var details UserDetails
	
	// Verifica se é o token de admin
	if token == *adminToken {
		details = UserDetails{
			Id:      "admin",
			Jid:     "",
			Webhook: "",
			Events:  "All",
		}
		return details, nil
	}
	
	// Consulta o banco de dados
	var txtid string
	var webhook string
	var jid string
	var events string
	
	err := s.db.QueryRow("SELECT id, webhook, jid, events FROM users WHERE token=$1 LIMIT 1", token).
		Scan(&txtid, &webhook, &jid, &events)
	
	if err != nil {
		log.Error().Err(err).Msg("Error validating token")
		return details, err
	}
	
	details = UserDetails{
		Id:      txtid,
		Jid:     jid,
		Webhook: webhook,
		Events:  events,
	}
	
	return details, nil
}

/**
 * DeleteUserComplete apaga completamente um usuário, incluindo arquivos de sessão do Whatsmeow
 */
func (s *server) DeleteUserComplete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		if id == "" {
			s.Respond(w, r, http.StatusBadRequest, map[string]string{"error": "ID não informado"})
			return
		}

		// Converter o ID para um int
		userID, err := strconv.Atoi(id)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, map[string]string{"error": "ID inválido"})
			return
		}

		// Verificar se o usuário existe
		var exists bool
		err = s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", id).Scan(&exists)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, map[string]string{"error": "Erro ao verificar existência do usuário"})
			return
		}
		if !exists {
			s.Respond(w, r, http.StatusNotFound, map[string]string{"error": "Usuário não encontrado"})
			return
		}

		// Buscar informações do usuário antes de excluir
		var uname, jid, token string
		err = s.db.QueryRow("SELECT name, jid, token FROM users WHERE id = $1", id).Scan(&uname, &jid, &token)
		if err != nil {
			log.Error().Err(err).Str("id", id).Msg("Erro ao recuperar informações do usuário")
		}

		// 1. Primeiro desconectar a instância se estiver conectada
		// Obter o cliente de wmiau.go
		clientPtr, exists := clientPointer[userID]
		if exists && clientPtr != nil {
			// Se estiver conectado, primeiro fazer logout
			if clientPtr.IsConnected() {
				log.Info().Str("id", id).Msg("Fazendo logout da instância antes de excluir")
				clientPtr.Logout()
			}
			// Desconectar
			log.Info().Str("id", id).Msg("Desconectando instância antes de excluir")
			clientPtr.Disconnect()
		}

		// 2. Excluir arquivos de sessão do Whatsmeow
		// Usar o caminho de execução do servidor como base
		sessionDir := filepath.Join(s.exPath, "store", id)
		if stat, err := os.Stat(sessionDir); err == nil && stat.IsDir() {
			log.Info().Str("dir", sessionDir).Msg("Removendo diretório de sessão")
			err = os.RemoveAll(sessionDir)
			if err != nil {
				log.Error().Err(err).Str("dir", sessionDir).Msg("Erro ao remover diretório de sessão")
			}
		}

		// 3. Excluir registros do banco de dados
		_, err = s.db.Exec("DELETE FROM users WHERE id = $1", id)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, map[string]string{"error": "Erro ao excluir usuário do banco de dados"})
			return
		}

		// 4. Limpar memória e outras referências
		delete(clientPointer, userID)
		delete(clientHttp, userID)
		// Remover do cache de usuários se existir
		userinfocache.Delete(token)

		// 5. Remover arquivos de mídia se existirem
		mediaDir := filepath.Join(s.exPath, "media", id)
		if stat, err := os.Stat(mediaDir); err == nil && stat.IsDir() {
			log.Info().Str("dir", mediaDir).Msg("Removendo diretório de mídia")
			err = os.RemoveAll(mediaDir)
			if err != nil {
				log.Error().Err(err).Str("dir", mediaDir).Msg("Erro ao remover diretório de mídia")
			}
		}

		log.Info().Str("id", id).Str("name", uname).Str("jid", jid).Msg("Usuário completamente excluído com sucesso")
		s.Respond(w, r, http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]string{
				"Details": "Instância completamente excluída incluindo todos os arquivos de sessão",
				"Id":      id,
			},
		})
	}
}


