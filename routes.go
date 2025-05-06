package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type Middleware = alice.Constructor

func (s *server) routes() {

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	var routerLog zerolog.Logger
	if *logType == "json" {
		routerLog = zerolog.New(os.Stdout).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Str("host", *address).
			Logger()
	} else {
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    !*colorOutput,
		}
		routerLog = zerolog.New(output).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Str("host", *address).
			Logger()
	}

	// Middleware CORS global para todas as rotas
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Configuração CORS
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, token, Instance-Id")
			
			// Responder imediatamente a solicitações OPTIONS
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	})

	// API V1 Router
	apiV1 := s.router.PathPrefix("/api/v1").Subrouter()

	// Admin routes with authentication middleware
	adminRoutes := apiV1.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(s.authadmin)
	adminRoutes.Handle("/users", s.ListUsers()).Methods("GET", "OPTIONS")
	adminRoutes.Handle("/users", s.AddUser()).Methods("POST", "OPTIONS")
	adminRoutes.Handle("/users/{id}", s.DeleteUser()).Methods("DELETE", "OPTIONS")
																																																																																													adminRoutes.Handle("/users/{id}/delete-complete", s.DeleteUserComplete()).Methods("DELETE", "OPTIONS")
	// Rota para verificar se o usuário é admin
	adminRoutes.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"isAdmin": true,
				"details": "Admin access confirmed",
			},
		})
	}).Methods("GET", "OPTIONS")

	// Rotas adicionais para o dashboard - mapeiam para as mesmas funções
	apiV1.Handle("/admin/users/list", s.authadmin(s.ListUsers())).Methods("GET", "OPTIONS")
	apiV1.Handle("/admin/user/create", s.authadmin(s.AddUser())).Methods("POST", "OPTIONS")
	apiV1.Handle("/admin/user/{id}", s.authadmin(s.DeleteUser())).Methods("DELETE", "OPTIONS")
	apiV1.Handle("/admin/user/{id}/delete-complete", s.authadmin(s.DeleteUserComplete())).Methods("DELETE", "OPTIONS")

	// Rotas alternativas para o front-end (sem o /api/v1)
	s.router.Handle("/admin/users", s.authadmin(s.ListUsers())).Methods("GET", "OPTIONS")
	s.router.Handle("/admin/users", s.authadmin(s.AddUser())).Methods("POST", "OPTIONS")
	s.router.Handle("/admin/user/{id}", s.authadmin(s.DeleteUser())).Methods("DELETE", "OPTIONS")
	s.router.Handle("/admin/user/{id}/delete-complete", s.authadmin(s.DeleteUserComplete())).Methods("DELETE", "OPTIONS")
	s.router.HandleFunc("/admin/status", func(w http.ResponseWriter, r *http.Request) {
		// Verificar token
		authHeader := r.Header.Get("Authorization")
		var token string
		
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			token = authHeader
		}
		
		// Verificar se o token é válido
		if token == "" || token != *adminToken {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error": "Não autorizado. Token de administrador requerido.",
			})
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"isAdmin": true,
				"details": "Admin access confirmed",
			},
		})
	}).Methods("GET", "OPTIONS")

	// Rota pública para obter token de admin (apenas para localhost ou com senha)
	apiV1.HandleFunc("/admin/token", func(w http.ResponseWriter, r *http.Request) {
		// Verificar se a requisição vem de localhost ou da mesma máquina
		clientIP := r.RemoteAddr
		password := r.URL.Query().Get("password")
		isLocalhost := strings.HasPrefix(clientIP, "127.0.0.1") || strings.HasPrefix(clientIP, "::1") || strings.HasPrefix(clientIP, "[::1]") || strings.HasPrefix(clientIP, "localhost")
		
		if isLocalhost || password == "wuzadmin" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"token":   *adminToken,
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Não autorizado. Use parâmetro password ou acesse localmente.",
			})
		}
	}).Methods("GET")

	c := alice.New()
	c = c.Append(s.authalice)
	c = c.Append(hlog.NewHandler(routerLog))

	c = c.Append(hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Info().
			Str("method", r.Method).
			Stringer("url", r.URL).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Str("userid", r.Context().Value("userinfo").(Values).Get("Id")).
			Msg("Got API Request")
	}))
	c = c.Append(hlog.RemoteAddrHandler("ip"))
	c = c.Append(hlog.UserAgentHandler("user_agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(hlog.RequestIDHandler("req_id", "Request-Id"))

	// Session endpoints
	apiV1.Handle("/session/connect", c.Then(s.Connect())).Methods("POST")
	apiV1.Handle("/session/disconnect", c.Then(s.Disconnect())).Methods("POST")
	apiV1.Handle("/session/logout", c.Then(s.Logout())).Methods("POST")
	apiV1.Handle("/session/status", c.Then(s.GetStatus())).Methods("GET")
	apiV1.Handle("/session/qr", c.Then(s.GetQR())).Methods("GET")
	apiV1.Handle("/session/pairphone", c.Then(s.PairPhone())).Methods("POST")
	apiV1.Handle("/session/proxy", c.Then(s.SetProxy())).Methods("POST")

	// Webhook endpoints
	apiV1.Handle("/webhook", c.Then(s.SetWebhook())).Methods("POST")
	apiV1.Handle("/webhook", c.Then(s.GetWebhook())).Methods("GET")
	apiV1.Handle("/webhook", c.Then(s.DeleteWebhook())).Methods("DELETE")
	apiV1.Handle("/webhook/update", c.Then(s.UpdateWebhook())).Methods("PUT")

	// Instances endpoints (mapeados para os usuários)
	apiV1.Handle("/instances", c.Then(s.ListUsers())).Methods("GET")
	apiV1.Handle("/instances/create", c.Then(s.AddUser())).Methods("POST")
	apiV1.Handle("/instances/{id}/delete", c.Then(s.DeleteUser())).Methods("DELETE")
	apiV1.Handle("/instances/{id}/delete-complete", c.Then(s.DeleteUserComplete())).Methods("DELETE")
	apiV1.Handle("/instances/{id}/status", c.Then(s.GetStatus())).Methods("GET")
	apiV1.Handle("/instances/{id}/webhook", c.Then(s.GetWebhook())).Methods("GET")
	apiV1.Handle("/instances/{id}/webhook", c.Then(s.SetWebhook())).Methods("POST")
	apiV1.Handle("/instances/{id}/webhook/delete", c.Then(s.DeleteWebhook())).Methods("DELETE")
	apiV1.Handle("/instances/{id}/connect", c.Then(s.Connect())).Methods("POST")
	apiV1.Handle("/instances/{id}/disconnect", c.Then(s.Disconnect())).Methods("POST")
	apiV1.Handle("/instances/{id}/logout", c.Then(s.Logout())).Methods("POST")
	apiV1.Handle("/instances/{id}/qr", c.Then(s.GetQR())).Methods("GET")

	// Rotas adicionais para instâncias específicas
	apiV1.Handle("/instances/{id}/pairphone", c.Then(s.PairPhone())).Methods("POST")
	// Atualização do nome de uma instância
	apiV1.HandleFunc("/instances/{id}/update", s.auth(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		
		// Decodifica a requisição
		var data struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "Erro ao decodificar payload", http.StatusBadRequest)
			return
		}
		
		// Atualiza o nome do usuário no banco de dados
		_, err := s.db.Exec("UPDATE users SET name = $1 WHERE id = $2", data.Name, id)
		if err != nil {
			http.Error(w, "Erro ao atualizar nome da instância", http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Nome da instância atualizado com sucesso",
		})
	})).Methods("POST")

	// Versão 1 das rotas - manter para retrocompatibilidade
	s.router.Handle("/session/connect", c.Then(s.Connect())).Methods("POST")
	s.router.Handle("/session/disconnect", c.Then(s.Disconnect())).Methods("POST")
	s.router.Handle("/session/logout", c.Then(s.Logout())).Methods("POST")
	s.router.Handle("/session/status", c.Then(s.GetStatus())).Methods("GET")
	s.router.Handle("/session/qr", c.Then(s.GetQR())).Methods("GET")
	s.router.Handle("/session/pairphone", c.Then(s.PairPhone())).Methods("POST")

	s.router.Handle("/webhook", c.Then(s.SetWebhook())).Methods("POST")
	s.router.Handle("/webhook", c.Then(s.GetWebhook())).Methods("GET")
	s.router.Handle("/webhook", c.Then(s.DeleteWebhook())).Methods("DELETE")
	s.router.Handle("/webhook/update", c.Then(s.UpdateWebhook())).Methods("PUT")

	s.router.Handle("/session/proxy", c.Then(s.SetProxy())).Methods("POST")

	s.router.Handle("/chat/send/text", c.Then(s.SendMessage())).Methods("POST")
	s.router.Handle("/chat/send/image", c.Then(s.SendImage())).Methods("POST")
	s.router.Handle("/chat/send/audio", c.Then(s.SendAudio())).Methods("POST")
	s.router.Handle("/chat/send/document", c.Then(s.SendDocument())).Methods("POST")
	//	s.router.Handle("/chat/send/template", c.Then(s.SendTemplate())).Methods("POST")
	s.router.Handle("/chat/send/video", c.Then(s.SendVideo())).Methods("POST")
	s.router.Handle("/chat/send/sticker", c.Then(s.SendSticker())).Methods("POST")
	s.router.Handle("/chat/send/location", c.Then(s.SendLocation())).Methods("POST")
	s.router.Handle("/chat/send/contact", c.Then(s.SendContact())).Methods("POST")
	s.router.Handle("/chat/react", c.Then(s.React())).Methods("POST")
	s.router.Handle("/chat/send/buttons", c.Then(s.SendButtons())).Methods("POST")
	s.router.Handle("/chat/send/list", c.Then(s.SendList())).Methods("POST")
	s.router.Handle("/user/presence", c.Then(s.SendPresence())).Methods("POST")

	s.router.Handle("/user/info", c.Then(s.GetUser())).Methods("POST")
	s.router.Handle("/user/check", c.Then(s.CheckUser())).Methods("POST")
	s.router.Handle("/user/avatar", c.Then(s.GetAvatar())).Methods("POST")
	s.router.Handle("/user/contacts", c.Then(s.GetContacts())).Methods("GET")

	s.router.Handle("/chat/presence", c.Then(s.ChatPresence())).Methods("POST")
	s.router.Handle("/chat/markread", c.Then(s.MarkRead())).Methods("POST")
	s.router.Handle("/chat/downloadimage", c.Then(s.DownloadImage())).Methods("POST")
	s.router.Handle("/chat/downloadvideo", c.Then(s.DownloadVideo())).Methods("POST")
	s.router.Handle("/chat/downloadaudio", c.Then(s.DownloadAudio())).Methods("POST")
	s.router.Handle("/chat/downloaddocument", c.Then(s.DownloadDocument())).Methods("POST")

	s.router.Handle("/group/list", c.Then(s.ListGroups())).Methods("GET")
	s.router.Handle("/group/info", c.Then(s.GetGroupInfo())).Methods("GET")
	s.router.Handle("/group/invitelink", c.Then(s.GetGroupInviteLink())).Methods("GET")
	s.router.Handle("/group/photo", c.Then(s.SetGroupPhoto())).Methods("POST")
	s.router.Handle("/group/name", c.Then(s.SetGroupName())).Methods("POST")

	s.router.Handle("/newsletter/list", c.Then(s.ListNewsletter())).Methods("GET")

	// Adicionar também as versões com prefixo /api/v1 para estas rotas
	apiV1.Handle("/chat/send/text", c.Then(s.SendMessage())).Methods("POST")
	apiV1.Handle("/chat/send/image", c.Then(s.SendImage())).Methods("POST")
	apiV1.Handle("/chat/send/audio", c.Then(s.SendAudio())).Methods("POST")
	apiV1.Handle("/chat/send/document", c.Then(s.SendDocument())).Methods("POST")
	apiV1.Handle("/chat/send/video", c.Then(s.SendVideo())).Methods("POST")
	apiV1.Handle("/chat/send/sticker", c.Then(s.SendSticker())).Methods("POST")
	apiV1.Handle("/chat/send/location", c.Then(s.SendLocation())).Methods("POST")
	apiV1.Handle("/chat/send/contact", c.Then(s.SendContact())).Methods("POST")
	apiV1.Handle("/chat/react", c.Then(s.React())).Methods("POST")
	apiV1.Handle("/chat/send/buttons", c.Then(s.SendButtons())).Methods("POST")
	apiV1.Handle("/chat/send/list", c.Then(s.SendList())).Methods("POST")
	apiV1.Handle("/user/presence", c.Then(s.SendPresence())).Methods("POST")
	apiV1.Handle("/user/info", c.Then(s.GetUser())).Methods("POST")
	apiV1.Handle("/user/check", c.Then(s.CheckUser())).Methods("POST")
	apiV1.Handle("/user/avatar", c.Then(s.GetAvatar())).Methods("POST")
	apiV1.Handle("/user/contacts", c.Then(s.GetContacts())).Methods("GET")
	apiV1.Handle("/chat/presence", c.Then(s.ChatPresence())).Methods("POST")
	apiV1.Handle("/chat/markread", c.Then(s.MarkRead())).Methods("POST")
	apiV1.Handle("/chat/downloadimage", c.Then(s.DownloadImage())).Methods("POST")
	apiV1.Handle("/chat/downloadvideo", c.Then(s.DownloadVideo())).Methods("POST")
	apiV1.Handle("/chat/downloadaudio", c.Then(s.DownloadAudio())).Methods("POST")
	apiV1.Handle("/chat/downloaddocument", c.Then(s.DownloadDocument())).Methods("POST")
	apiV1.Handle("/group/list", c.Then(s.ListGroups())).Methods("GET")
	apiV1.Handle("/group/info", c.Then(s.GetGroupInfo())).Methods("GET")
	apiV1.Handle("/group/invitelink", c.Then(s.GetGroupInviteLink())).Methods("GET")
	apiV1.Handle("/group/photo", c.Then(s.SetGroupPhoto())).Methods("POST")
	apiV1.Handle("/group/name", c.Then(s.SetGroupName())).Methods("POST")
	apiV1.Handle("/newsletter/list", c.Then(s.ListNewsletter())).Methods("GET")

	// Servir arquivos estáticos
	// O dashboard de gerenciamento está disponível em /dashboard
	// A interface de login está disponível em /login
	// A documentação da API está disponível em /api
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir(exPath + "/static/")))
}
