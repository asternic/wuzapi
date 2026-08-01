package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// parseNewsletterJID builds a newsletter JID from a bare id (no @) or parses a full jid.
func parseNewsletterJID(arg string) (types.JID, bool) {
	if arg == "" {
		return types.JID{}, false
	}
	if len(arg) > 0 && arg[0] != '+' && !containsAt(arg) {
		return types.NewJID(arg, types.NewsletterServer), true
	}
	return parseJID(arg)
}

func containsAt(s string) bool {
	for _, c := range s {
		if c == '@' {
			return true
		}
	}
	return false
}

// CreateNewsletter creates a new WhatsApp channel (newsletter).
func (s *server) CreateNewsletter() http.HandlerFunc {

	type createNewsletterStruct struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Picture     string `json:"picture"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t createNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Name in Payload"))
			return
		}

		var picture []byte
		if t.Picture != "" {
			if len(t.Picture) > 10 && t.Picture[0:10] == "data:image" {
				dataURL, err := dataurl.DecodeString(t.Picture)
				if err != nil {
					s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode base64 encoded Picture from payload"))
					return
				}
				picture = dataURL.Data
			}
		}

		// Aceita o ToS de criação de newsletter antes do create — o fork não faz isso automaticamente
		// (diferente do Baileys/zapo-js, que auto-aceitam). Falha silenciosa se já aceito antes.
		_ = client.AcceptTOSNotice(r.Context(), "20601218", "5")

		info, err := client.CreateNewsletter(r.Context(), whatsmeow.CreateNewsletterParams{
			Name:        t.Name,
			Description: t.Description,
			Picture:     picture,
		})

		if err != nil {
			msg := fmt.Sprintf("failed to create newsletter: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		responseJson, err := json.Marshal(info)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// GetNewsletterInfo fetches a newsletter's metadata by JID or invite code.
func (s *server) GetNewsletterInfo() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		id := r.URL.Query().Get("id")
		invite := r.URL.Query().Get("invite")

		var info *types.NewsletterMetadata
		var err error

		if invite != "" {
			info, err = client.GetNewsletterInfoWithInvite(r.Context(), invite)
		} else if id != "" {
			jid, ok := parseNewsletterJID(id)
			if !ok {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
				return
			}
			info, err = client.GetNewsletterInfo(r.Context(), jid)
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing id or invite parameter"))
			return
		}

		if err != nil {
			msg := fmt.Sprintf("failed to get newsletter info: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		responseJson, err := json.Marshal(info)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// FollowNewsletter subscribes the current account to a newsletter.
func (s *server) FollowNewsletter() http.HandlerFunc {
	return s.newsletterFollowHandler(true)
}

// UnfollowNewsletter unsubscribes the current account from a newsletter.
func (s *server) UnfollowNewsletter() http.HandlerFunc {
	return s.newsletterFollowHandler(false)
}

func (s *server) newsletterFollowHandler(follow bool) http.HandlerFunc {

	type followNewsletterStruct struct {
		Id string `json:"id"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t followNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		jid, ok := parseNewsletterJID(t.Id)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
			return
		}

		if follow {
			err = client.FollowNewsletter(context.Background(), jid)
		} else {
			err = client.UnfollowNewsletter(context.Background(), jid)
		}

		if err != nil {
			msg := fmt.Sprintf("failed to update newsletter subscription: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		s.Respond(w, r, http.StatusOK, map[string]bool{"ok": true})
		return
	}
}

// MuteNewsletter mutes or unmutes a newsletter for the current account.
func (s *server) MuteNewsletter() http.HandlerFunc {

	type muteNewsletterStruct struct {
		Id   string `json:"id"`
		Mute bool   `json:"mute"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t muteNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		jid, ok := parseNewsletterJID(t.Id)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
			return
		}

		err = client.NewsletterToggleMute(context.Background(), jid, t.Mute)
		if err != nil {
			msg := fmt.Sprintf("failed to toggle newsletter mute: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		s.Respond(w, r, http.StatusOK, map[string]bool{"ok": true})
		return
	}
}
