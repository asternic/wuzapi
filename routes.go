package main

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	logOutput := os.Stdout
	if s.mode == Stdio {
		logOutput = os.Stderr
	}
	if *logType == "json" {
		routerLog = zerolog.New(logOutput).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Str("host", *address).
			Logger()
	} else {
		output := zerolog.ConsoleWriter{
			Out:        logOutput,
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

	s.router.Handle("/health", s.GetHealth()).Methods("GET")

	adminRoutes := s.router.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(s.authadmin)
	adminRoutes.Handle("/users", s.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users/{id}", s.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users", s.AddUser()).Methods("POST")
	adminRoutes.Handle("/users/{id}", s.EditUser()).Methods("PUT")
	adminRoutes.Handle("/users/{id}", s.DeleteUser()).Methods("DELETE")
	adminRoutes.Handle("/users/{id}/full", s.DeleteUserComplete()).Methods("DELETE")

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

	s.router.Handle("/session/connect", c.Then(s.Connect())).Methods("POST")
	s.router.Handle("/session/disconnect", c.Then(s.Disconnect())).Methods("POST")
	s.router.Handle("/session/logout", c.Then(s.Logout())).Methods("POST")
	s.router.Handle("/session/status", c.Then(s.GetStatus())).Methods("GET")
	s.router.Handle("/session/qr", c.Then(s.GetQR())).Methods("GET")
	s.router.Handle("/session/pairphone", c.Then(s.PairPhone())).Methods("POST")
	s.router.Handle("/session/history", c.Then(s.RequestHistorySync())).Methods("GET")

	s.router.Handle("/webhook", c.Then(s.SetWebhook())).Methods("POST")
	s.router.Handle("/webhook", c.Then(s.GetWebhook())).Methods("GET")
	s.router.Handle("/webhook", c.Then(s.DeleteWebhook())).Methods("DELETE")
	s.router.Handle("/webhook", c.Then(s.UpdateWebhook())).Methods("PUT")

	s.router.Handle("/session/proxy", c.Then(s.SetProxy())).Methods("POST")
	s.router.Handle("/session/history", c.Then(s.SetHistory())).Methods("POST")

	s.router.Handle("/session/s3/config", c.Then(s.ConfigureS3())).Methods("POST")
	s.router.Handle("/session/s3/config", c.Then(s.GetS3Config())).Methods("GET")
	s.router.Handle("/session/s3/config", c.Then(s.DeleteS3Config())).Methods("DELETE")
	s.router.Handle("/session/s3/test", c.Then(s.TestS3Connection())).Methods("POST")

	s.router.Handle("/session/hmac/config", c.Then(s.ConfigureHmac())).Methods("POST")
	s.router.Handle("/session/hmac/config", c.Then(s.GetHmacConfig())).Methods("GET")
	s.router.Handle("/session/hmac/config", c.Then(s.DeleteHmacConfig())).Methods("DELETE")

	s.router.Handle("/chat/send/text", c.Then(s.SendMessage())).Methods("POST")
	s.router.Handle("/chat/delete", c.Then(s.DeleteMessage())).Methods("POST")
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
	s.router.Handle("/chat/send/poll", c.Then(s.SendPoll())).Methods("POST")
	s.router.Handle("/chat/send/edit", c.Then(s.SendEditMessage())).Methods("POST")
	s.router.Handle("/chat/history", c.Then(s.GetHistory())).Methods("GET")
	s.router.Handle("/chat/request-unavailable-message", c.Then(s.RequestUnavailableMessage())).Methods("POST")
	s.router.Handle("/chat/archive", c.Then(s.ArchiveChat())).Methods("POST")

	s.router.Handle("/status/set/text", c.Then(s.SetStatusMessage())).Methods("POST")

	s.router.Handle("/call/reject", c.Then(s.RejectCall())).Methods("POST")

	s.router.Handle("/user/presence", c.Then(s.SendPresence())).Methods("POST")
	s.router.Handle("/user/info", c.Then(s.GetUser())).Methods("POST")
	s.router.Handle("/user/check", c.Then(s.CheckUser())).Methods("POST")
	s.router.Handle("/user/avatar", c.Then(s.GetAvatar())).Methods("POST")
	s.router.Handle("/user/contacts", c.Then(s.GetContacts())).Methods("GET")
	s.router.Handle("/user/lid/{jid}", c.Then(s.GetUserLID())).Methods("GET")

	s.router.Handle("/chat/presence", c.Then(s.ChatPresence())).Methods("POST")
	s.router.Handle("/chat/markread", c.Then(s.MarkRead())).Methods("POST")
	s.router.Handle("/chat/downloadimage", c.Then(s.DownloadImage())).Methods("POST")
	s.router.Handle("/chat/downloadvideo", c.Then(s.DownloadVideo())).Methods("POST")
	s.router.Handle("/chat/downloadaudio", c.Then(s.DownloadAudio())).Methods("POST")
	s.router.Handle("/chat/downloaddocument", c.Then(s.DownloadDocument())).Methods("POST")
	s.router.Handle("/chat/downloadsticker", c.Then(s.DownloadSticker())).Methods("POST")

	s.router.Handle("/group/create", c.Then(s.CreateGroup())).Methods("POST")
	s.router.Handle("/group/list", c.Then(s.ListGroups())).Methods("GET")
	s.router.Handle("/group/info", c.Then(s.GetGroupInfo())).Methods("GET")
	s.router.Handle("/group/invitelink", c.Then(s.GetGroupInviteLink())).Methods("GET")
	s.router.Handle("/group/photo", c.Then(s.SetGroupPhoto())).Methods("POST")
	s.router.Handle("/group/photo/remove", c.Then(s.RemoveGroupPhoto())).Methods("POST")
	s.router.Handle("/group/leave", c.Then(s.GroupLeave())).Methods("POST")
	s.router.Handle("/group/name", c.Then(s.SetGroupName())).Methods("POST")
	s.router.Handle("/group/topic", c.Then(s.SetGroupTopic())).Methods("POST")
	s.router.Handle("/group/announce", c.Then(s.SetGroupAnnounce())).Methods("POST")
	s.router.Handle("/group/locked", c.Then(s.SetGroupLocked())).Methods("POST")
	s.router.Handle("/group/ephemeral", c.Then(s.SetDisappearingTimer())).Methods("POST")
	s.router.Handle("/group/join", c.Then(s.GroupJoin())).Methods("POST")
	s.router.Handle("/group/inviteinfo", c.Then(s.GetGroupInviteInfo())).Methods("POST")
	s.router.Handle("/group/updateparticipants", c.Then(s.UpdateGroupParticipants())).Methods("POST")

	s.router.Handle("/newsletter/list", c.Then(s.ListNewsletter())).Methods("GET")

	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir(exPath + "/static/")))
}
