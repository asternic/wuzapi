package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"encoding/json"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// CreateCommunity creates a WhatsApp community (a parent group). The linked announcement
// group is created automatically by the server, per whatsmeow's ReqCreateGroup docs.
func (s *server) CreateCommunity() http.HandlerFunc {

	type createCommunityStruct struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t createCommunityStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Name in Payload"))
			return
		}

		req := whatsmeow.ReqCreateGroup{
			Name: t.Name,
		}
		req.IsParent = true

		groupInfo, err := clientManager.GetWhatsmeowClient(txtid).CreateGroup(r.Context(), req)

		if err != nil {
			log.Error().Str("error", fmt.Sprintf("%v", err)).Msg("failed to create community")
			msg := fmt.Sprintf("failed to create community: %v", err)
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

// LinkCommunityGroup links an existing group as a sub-group of a community.
func (s *server) LinkCommunityGroup() http.HandlerFunc {

	type linkCommunityGroupStruct struct {
		CommunityJID string `json:"communityJID"`
		GroupJID     string `json:"groupJID"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t linkCommunityGroupStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		community, ok := parseJID(t.CommunityJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Community JID"))
			return
		}
		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).LinkGroup(context.Background(), community, group)
		if err != nil {
			msg := fmt.Sprintf("failed to link group to community: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		s.Respond(w, r, http.StatusOK, map[string]bool{"linked": true})
		return
	}
}

// UnlinkCommunityGroup removes a group from a community.
func (s *server) UnlinkCommunityGroup() http.HandlerFunc {

	type unlinkCommunityGroupStruct struct {
		CommunityJID string `json:"communityJID"`
		GroupJID     string `json:"groupJID"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t unlinkCommunityGroupStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		community, ok := parseJID(t.CommunityJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Community JID"))
			return
		}
		group, ok := parseJID(t.GroupJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Group JID"))
			return
		}

		err = clientManager.GetWhatsmeowClient(txtid).UnlinkGroup(context.Background(), community, group)
		if err != nil {
			msg := fmt.Sprintf("failed to unlink group from community: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		s.Respond(w, r, http.StatusOK, map[string]bool{"unlinked": true})
		return
	}
}

// GetCommunitySubGroups lists the sub-groups linked to a community.
func (s *server) GetCommunitySubGroups() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")

		if clientManager.GetWhatsmeowClient(txtid) == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		communityJID := r.URL.Query().Get("communityJID")
		if communityJID == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing communityJID parameter"))
			return
		}

		community, ok := parseJID(communityJID)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse Community JID"))
			return
		}

		resp, err := clientManager.GetWhatsmeowClient(txtid).GetSubGroups(context.Background(), community)
		if err != nil {
			msg := fmt.Sprintf("failed to get community sub-groups: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		type SubGroupCollection struct {
			Groups []types.GroupLinkTarget
		}
		gc := new(SubGroupCollection)
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
