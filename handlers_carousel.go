package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const (
	carouselMaxRequestBytes = 24 * 1024 * 1024
	carouselPrepareTimeout  = 45 * time.Second
	carouselSendTimeout     = 60 * time.Second
)

type carouselButton struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	ID          string `json:"id"`
	URL         string `json:"url"`
	PhoneNumber string `json:"phone_number"`
}

type carouselCard struct {
	Body    string           `json:"Body"`
	Image   string           `json:"Image"`
	Buttons []carouselButton `json:"Buttons"`
}

type carouselRequest struct {
	Phone string         `json:"Phone"`
	Id    string         `json:"Id"`
	Body  string         `json:"Body"`
	Cards []carouselCard `json:"Cards"`
	// stdio forwards transport credentials in the body; auth uses headers only.
	Token      string `json:"token"`
	AdminToken string `json:"adminToken"`
}

type preparedCarouselCard struct {
	media   *mediaFile
	image   *waE2E.ImageMessage
	body    string
	buttons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
}

// Keep the underlying cause for status selection without exposing local paths.
type carouselMediaError struct {
	card      int
	operation string
	cause     error
}

func (e *carouselMediaError) Error() string {
	return fmt.Sprintf("Cards[%d].Image: %s", e.card, e.operation)
}
func (e *carouselMediaError) Unwrap() error { return e.cause }

func carouselErrorStatus(err error, fallback int) int {
	var pathError *os.PathError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		return http.StatusServiceUnavailable
	case errors.As(err, &pathError):
		return http.StatusInternalServerError
	default:
		return fallback
	}
}

func (s *server) respondCarouselFailure(w http.ResponseWriter, r *http.Request, err error, dispatched bool) {
	status := http.StatusInternalServerError
	message := errors.New("carousel delivery unconfirmed; do not automatically resend")
	if !dispatched {
		status = carouselErrorStatus(err, status)
		message = fmt.Errorf("%v; message not sent", err)
	}
	s.Respond(w, r, status, message)
}

func closeCarouselMedia(cards []preparedCarouselCard) {
	for _, card := range cards {
		card.media.Close()
	}
}

func carouselText(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= max
}

func carouselDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func validateCarouselRecipient(jid types.JID) error {
	if (jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer) ||
		jid.Device != 0 || jid.RawAgent != 0 || !carouselDigits(jid.User) || jid.IsBot() {
		return errors.New("Phone must identify a direct PN or LID chat")
	}
	return nil
}

// Validate every card before fetching media or uploading anything.
func validateCarousel(request carouselRequest) ([][]*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, error) {
	for _, c := range request.Id {
		if c <= ' ' || c >= 127 {
			return nil, errors.New("Id must contain printable ASCII without whitespace")
		}
	}
	if len(request.Id) > 128 {
		return nil, errors.New("Id exceeds 128 bytes")
	}
	if !carouselText(request.Body, 1024) || len(request.Cards) < 2 || len(request.Cards) > 10 {
		return nil, errors.New("Body requires 1-1024 characters and Cards requires 2-10 entries")
	}
	buttons := make([][]*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, len(request.Cards))
	replyIDs := make(map[string]bool)
	for i, card := range request.Cards {
		if !carouselText(card.Body, 160) || card.Image == "" || len(card.Buttons) < 1 || len(card.Buttons) > 3 {
			return nil, fmt.Errorf("Cards[%d] requires Body (1-160 characters), Image and 1-3 Buttons", i)
		}
		for j, button := range card.Buttons {
			title := button.Title
			if !carouselText(title, 20) || strings.IndexFunc(title, unicode.IsControl) >= 0 {
				return nil, fmt.Errorf("Cards[%d].Buttons[%d].title requires 1-20 characters without controls", i, j)
			}
			name := button.Type
			params := map[string]string{"display_text": title}
			valid := false
			switch name {
			case "", "reply":
				valid = carouselText(button.ID, 200) && strings.IndexFunc(button.ID, unicode.IsControl) < 0 &&
					!replyIDs[button.ID] && button.URL == "" && button.PhoneNumber == ""
				replyIDs[button.ID] = true
				name, params["id"] = "quick_reply", button.ID
			case "cta_url":
				u, err := url.Parse(button.URL)
				valid = err == nil && len(button.URL) <= 2048 && u.Hostname() != "" && u.User == nil &&
					(u.Scheme == "https" || u.Scheme == "http") &&
					strings.IndexFunc(button.URL, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) }) < 0 &&
					button.ID == "" && button.PhoneNumber == ""
				params["url"], params["merchant_url"] = button.URL, button.URL
			case "cta_call":
				phone := strings.TrimPrefix(button.PhoneNumber, "+")
				valid = carouselDigits(phone) && len(phone) >= 6 && len(phone) <= 15 && button.ID == "" && button.URL == ""
				params["phone_number"] = button.PhoneNumber
			}
			if !valid {
				return nil, fmt.Errorf("Cards[%d].Buttons[%d] has an invalid action or duplicate reply id", i, j)
			}
			encoded, _ := json.Marshal(params)
			buttons[i] = append(buttons[i], &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
				Name: proto.String(name), ButtonParamsJSON: proto.String(string(encoded)),
			})
		}
	}
	return buttons, nil
}

func prepareCarousel(ctx context.Context, request carouselRequest) (_ []preparedCarouselCard, err error) {
	buttons, err := validateCarousel(request)
	if err != nil {
		return nil, err
	}
	var cards []preparedCarouselCard
	defer func() {
		if err != nil {
			closeCarouselMedia(cards)
		}
	}()
	remaining := int64(fetchImageMaxBytes)
	for i, card := range request.Cards {
		if err := ctx.Err(); err != nil {
			return nil, &carouselMediaError{i, "preparation canceled", err}
		}
		media, readErr := readOutgoingMedia(ctx, card.Image, remaining)
		if readErr != nil {
			log.Error().Err(readErr).Int("card", i).Msg("Failed to read carousel image")
			return nil, &carouselMediaError{i, "could not read image", readErr}
		}
		cards = append(cards, preparedCarouselCard{media: media, body: card.Body, buttons: buttons[i]})
		if media.Size == 0 {
			return nil, fmt.Errorf("Cards[%d].Image: empty image", i)
		}
		if media.Size > remaining {
			return nil, fmt.Errorf("Cards[%d].Image: images exceed the 16 MiB total limit", i)
		}
		remaining -= media.Size
		reader, openErr := media.Open()
		if openErr != nil {
			log.Error().Err(openErr).Int("card", i).Msg("Failed to open carousel image")
			return nil, &carouselMediaError{i, "could not read image", openErr}
		}
		config, format, decodeErr := image.DecodeConfig(reader)
		reader.Close()
		if decodeErr != nil {
			return nil, &carouselMediaError{i, "could not decode image", decodeErr}
		}
		if (format != "jpeg" && format != "png") || config.Width <= 0 || config.Height <= 0 ||
			config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 16000000 {
			return nil, fmt.Errorf("Cards[%d].Image: requires JPEG/PNG up to 4096px per side and 16 megapixels", i)
		}
		thumbnail, thumbnailErr := mediaThumbnail(ctx, media)
		if thumbnailErr != nil {
			log.Error().Err(thumbnailErr).Int("card", i).Msg("Failed to decode carousel image")
			return nil, &carouselMediaError{i, "could not decode image", thumbnailErr}
		}
		cards[i].image = &waE2E.ImageMessage{
			Mimetype: proto.String("image/" + format), JPEGThumbnail: thumbnail,
			Width: proto.Uint32(uint32(config.Width)), Height: proto.Uint32(uint32(config.Height)),
		}
	}
	return cards, nil
}

type carouselSender interface {
	mediaUploader
	messageSender
}

func sendCarousel(ctx context.Context, userID string, client carouselSender, recipient types.JID, id, body string, prepared []preparedCarouselCard) (whatsmeow.SendResponse, *waE2E.Message, error) {
	cards := make([]*waE2E.InteractiveMessage, 0, len(prepared))
	for i, card := range prepared {
		if err := ctx.Err(); err != nil {
			return whatsmeow.SendResponse{}, nil, &carouselMediaError{i, "preparation canceled", err}
		}
		uploaded, err := uploadMedia(ctx, client, card.media, whatsmeow.MediaImage)
		if err != nil {
			log.Error().Err(err).Int("card", i).Msg("Failed to upload carousel image")
			return whatsmeow.SendResponse{}, nil, &carouselMediaError{i, "upload failed", err}
		}
		img := proto.Clone(card.image).(*waE2E.ImageMessage)
		img.URL, img.DirectPath = proto.String(uploaded.URL), proto.String(uploaded.DirectPath)
		img.MediaKey, img.FileSHA256, img.FileEncSHA256 = uploaded.MediaKey, uploaded.FileSHA256, uploaded.FileEncSHA256
		img.FileLength = proto.Uint64(uint64(card.media.Size))
		cards = append(cards, &waE2E.InteractiveMessage{
			Header: &waE2E.InteractiveMessage_Header{HasMediaAttachment: proto.Bool(true), Media: &waE2E.InteractiveMessage_Header_ImageMessage{ImageMessage: img}},
			Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(card.body)},
			InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons: card.buttons, MessageVersion: proto.Int32(1),
			}},
		})
	}
	message := &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{Text: proto.String(body)},
		InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
			Cards: cards, MessageVersion: proto.Int32(1), CarouselCardType: waE2E.InteractiveMessage_CarouselMessage_HSCROLL_CARDS.Enum(),
		}},
	}}
	// Match SendButtons' native-flow envelope. whatsmeow does not retain these
	// AdditionalNodes when responding to a recipient's retry receipt.
	nodes := []waBinary.Node{{Tag: "biz", Content: []waBinary.Node{{
		Tag: "interactive", Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
		Content: []waBinary.Node{{Tag: "native_flow", Attrs: waBinary.Attrs{"v": "9", "name": "mixed"}}},
	}}}}
	if err := ctx.Err(); err != nil {
		return whatsmeow.SendResponse{}, nil, fmt.Errorf("carousel preparation canceled: %w", err)
	}
	// Once dispatched, finish the ACK/history path even if HTTP disconnects.
	// An ACK timeout is still ambiguous; never replay the send here.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), carouselSendTimeout)
	defer cancel()
	response, err := sendMessageWithRecovery(userID, client, sendCtx, recipient, message,
		whatsmeow.SendRequestExtra{ID: id, AdditionalNodes: &nodes, Timeout: carouselSendTimeout})
	return response, message, err
}

// SendCarousel sends ordered image cards as one native interactive message.
func (s *server) SendCarousel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := r.Context().Value("userinfo").(Values)
		userID := info.Get("Id")
		client := clientManager.GetWhatsmeowClient(userID)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), carouselPrepareTimeout)
		defer cancel()
		r.Body = http.MaxBytesReader(w, r.Body, carouselMaxRequestBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request carouselRequest
		if err := decoder.Decode(&request); err != nil {
			status := http.StatusBadRequest
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			s.Respond(w, r, status, fmt.Errorf("could not decode Payload: %v", err))
			return
		}
		if decoder.Decode(new(interface{})) != io.EOF {
			s.Respond(w, r, http.StatusBadRequest, errors.New("Payload must contain one JSON object"))
			return
		}
		recipient, err := validateMessageFields(ctx, client, request.Phone, nil, nil)
		if err == nil {
			err = validateCarouselRecipient(recipient)
		}
		if err != nil {
			s.Respond(w, r, carouselErrorStatus(err, http.StatusBadRequest), err)
			return
		}
		cards, err := prepareCarousel(ctx, request)
		if err != nil {
			s.Respond(w, r, carouselErrorStatus(err, http.StatusBadRequest), err)
			return
		}
		defer closeCarouselMedia(cards)
		id := request.Id
		if id == "" {
			id = client.GenerateMessageID()
		}
		response, message, err := sendCarousel(ctx, userID, client, recipient, id, request.Body, cards)
		if err != nil {
			log.Error().Err(err).Str("id", id).Bool("dispatched", message != nil).Msg("Carousel send failed")
			s.respondCarouselFailure(w, r, err, message != nil)
			return
		}
		historyLimit, _ := strconv.Atoi(info.Get("History"))
		s.saveOutgoingMessageToHistory(userID, recipient.String(), id, "carousel", request.Body, "", historyLimit)
		s.publishSentMessageEvent(info.Get("Token"), userID, userID, recipient, id, message, response.Timestamp, "carousel")
		responseJSON, _ := json.Marshal(map[string]interface{}{"Details": "Sent", "Timestamp": response.Timestamp.Unix(), "Id": id})
		s.Respond(w, r, http.StatusOK, string(responseJSON))
	}
}
