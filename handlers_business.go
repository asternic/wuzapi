package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.mau.fi/whatsmeow/types"
)

type businessProfileLookup func(context.Context, string, types.JID) (*types.BusinessProfile, error)

func lookupAPIBusinessProfile(ctx context.Context, user string, jid types.JID) (*types.BusinessProfile, error) {
	client := clientManager.GetWhatsmeowClient(user)
	if client == nil {
		return nil, fmt.Errorf("no session")
	}
	return client.GetBusinessProfile(ctx, jid)
}

func (s *server) businessProfileHandler(lookup businessProfileLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Phone string }
		if err := decodeAPIRequest(r.Body, &request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		jid, err := apiChatJID(request.Phone)
		if err != nil || jid.Server == types.GroupServer {
			s.Respond(w, r, http.StatusBadRequest, fmt.Errorf("Phone must identify an individual account"))
			return
		}
		profile, err := lookup(r.Context(), r.Context().Value("userinfo").(Values).Get("Id"), jid)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		if profile == nil {
			s.Respond(w, r, http.StatusNotFound, fmt.Errorf("business profile unavailable"))
			return
		}
		data, err := json.Marshal(profile)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Respond(w, r, http.StatusOK, string(data))
	}
}
