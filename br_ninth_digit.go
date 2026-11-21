package main

// Brazilian ninth-digit resolution.
//
// Many Brazilian mobile accounts were registered before the ninth digit and
// still live under the 12-digit number (55 + DDD + 8 digits), while CRMs store
// the 13-digit form (or the reverse). Sending to the wrong form silently goes
// nowhere. For bare Brazilian mobile numbers that are not already mapped to a
// LID, ask WhatsApp which form exists and send to that one. Results are cached;
// when WhatsApp cannot answer, the number is used as given (legacy behavior).

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types"
)

type onWhatsAppFunc func(context.Context, []string) ([]types.IsOnWhatsAppResponse, error)

var brNumberCache = cache.New(24*time.Hour, time.Hour)

// brazilianVariants returns the number and its alternate ninth-digit form, or
// nil when the number is not a Brazilian mobile number.
func brazilianVariants(user string) []string {
	if len(user) < 12 || user[:2] != "55" || user[2] == '0' || user[3] == '0' {
		return nil
	}
	switch len(user) {
	case 13: // 55 DD 9XXXXXXXX
		if user[4] == '9' && user[5] >= '6' {
			return []string{user, user[:4] + user[5:]}
		}
	case 12: // 55 DD XXXXXXXX (mobiles start with 6-9)
		if user[4] >= '6' {
			return []string{user, user[:4] + "9" + user[4:]}
		}
	}
	return nil
}

func resolveBrazilianNumber(ctx context.Context, check onWhatsAppFunc, recipient types.JID) types.JID {
	if check == nil || recipient.Server != types.DefaultUserServer || recipient.Device != 0 {
		return recipient
	}
	variants := brazilianVariants(recipient.User)
	if variants == nil {
		return recipient
	}
	if cached, ok := brNumberCache.Get(recipient.User); ok {
		if jid, err := types.ParseJID(cached.(string)); err == nil {
			return jid
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	query := make([]string, len(variants))
	for i, v := range variants {
		query[i] = "+" + v
	}
	resp, err := check(ctx, query)
	if err != nil {
		log.Warn().Err(err).Msg("Brazilian number check failed; sending to the number as given")
		return recipient
	}
	// WhatsApp answers with the phone-number JID of the form that exists or,
	// increasingly, only with the account's LID (one answer for both forms).
	// Prefer the phone form the caller sent, then the other phone form, then the LID.
	var found types.JID
	for _, v := range variants {
		for _, r := range resp {
			if r.IsIn && r.JID.Server == types.DefaultUserServer && r.JID.User == v {
				found = r.JID
				break
			}
		}
		if !found.IsEmpty() {
			break
		}
	}
	if found.IsEmpty() {
		for _, r := range resp {
			if r.IsIn && r.JID.User != "" && (r.JID.Server == types.DefaultUserServer || r.JID.Server == types.HiddenUserServer) {
				found = r.JID.ToNonAD()
				break
			}
		}
	}
	if found.IsEmpty() {
		log.Debug().Str("phone", recipient.User).Msg("Brazilian number not found on WhatsApp in either form; sending as given")
		brNumberCache.Set(recipient.User, recipient.String(), 30*time.Minute)
		return recipient
	}
	for _, v := range variants {
		brNumberCache.Set(v, found.String(), cache.DefaultExpiration)
	}
	if found != recipient {
		log.Info().Str("given", recipient.User).Str("resolved", found.String()).Msg("Brazilian number resolved")
	}
	return found
}
