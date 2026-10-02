package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

func mediaSignature(body *mediaFile, encrypted []byte) (string, error) {
	if len(encrypted) == 0 {
		return "", nil
	}
	key, err := decryptHMACKey(encrypted)
	if err != nil {
		return "", err
	}
	release, err := body.store.acquire(body.store.ctx)
	if err != nil {
		return "", err
	}
	defer release()
	r, err := body.Open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	h := hmac.New(sha256.New, []byte(key))
	if _, err = io.Copy(h, contextReader{body.store.ctx, r}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func postMediaBody(ctx context.Context, client *http.Client, target, contentType, signature string, body *mediaFile, headers ...http.Header) (int, error) {
	r, err := body.Open()
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, r)
	if err != nil {
		r.Close()
		return 0, err
	}
	req.ContentLength = body.Size
	req.GetBody = func() (io.ReadCloser, error) { return body.Open() }
	if len(headers) > 0 && headers[0] != nil {
		req.Header = headers[0].Clone()
	}
	req.Header.Set("Content-Type", contentType)
	if contentType == "application/json" && req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", contentType)
	}
	// Match Resty's default request headers, using its configured transport below.
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "go-resty/"+resty.Version+" (https://github.com/go-resty/resty)")
	}
	if signature != "" {
		req.Header.Set("x-hmac-signature", signature)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// Successful responses are drained for keepalive without buffering.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Preserve the existing error-queue response text. Do not log this body.
		response, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return resp.StatusCode, readErr
		}
		return resp.StatusCode, fmt.Errorf("unexpected status code: %d. Body: %s", resp.StatusCode, response)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, err
}
func deliverMediaHook(ctx context.Context, target, userID string, key []byte, event, enhanced *mediaFile, instance string) {
	client := clientManager.GetHTTPClient(userID)
	if client == nil {
		return
	}
	body := enhanced
	contentType := "application/json"
	payload := interface{}(fileJSON{file: enhanced})
	if os.Getenv("WEBHOOK_FORMAT") != "json" {
		var err error
		body, err = prepareMediaForm(event, instance, userID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare media form")
			return
		}
		defer body.Close()
		contentType = "application/x-www-form-urlencoded"
		payload = map[string]interface{}{"jsonData": fileJSON{file: event, quoted: true}, "instanceName": instance, "userID": userID}
	}
	signature, err := mediaSignature(body, key)
	if err != nil {
		log.Error().Err(err).Msg("Failed to sign media webhook")
	} else {
		attempts := 1
		if *webhookRetryEnabled {
			attempts = *webhookRetryCount
		}
		for attempt := 0; attempt < attempts; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(time.Duration(*webhookRetryDelaySeconds) * time.Second * time.Duration(1<<uint(attempt-1)))
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			var status int
			status, err = postMediaBody(ctx, client.GetClient(), target, contentType, signature, body, client.Header)
			if err == nil {
				log.Info().Int("status", status).Int64("size", body.Size).Msg("Media webhook delivered")
				return
			}
			log.Warn().Int("attempt", attempt+1).Int("status", status).Int64("size", body.Size).Msg("Media webhook delivery failed")
			if ctx.Err() != nil {
				return
			}
		}
	}
	if err == nil {
		return
	}
	// Match WebhookErrorPayload's field names and values without materializing its payload.
	failure, prepareErr := prepareMediaJSON(map[string]interface{}{
		"url": target, "payload": payload, "userID": userID, "encryptedHmacKey": hex.EncodeToString(key), "attemptTime": time.Now(), "errorMessage": err.Error(),
	})
	if prepareErr != nil {
		log.Error().Err(prepareErr).Msg("Failed to prepare media error queue payload")
		return
	}
	defer failure.Close()
	if publishErr := publishMediaToRabbit(failure, *webhookErrorQueueName); publishErr != nil {
		log.Error().Err(publishErr).Msg("Failed to publish media error")
	}
}
func sendMediaEvent(mycli *MyClient, postmap map[string]interface{}, userURL string) {
	instance := ""
	var key []byte
	if info, ok := userinfocache.Get(mycli.token); ok {
		instance = info.(Values).Get("Name")
		key, _ = base64.StdEncoding.DecodeString(info.(Values).Get("HmacKeyEncrypted"))
	}
	event, err := prepareMediaJSON(postmap)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare media event")
		return
	}
	copyMap := make(map[string]interface{}, len(postmap)+2)
	for k, v := range postmap {
		copyMap[k] = v
	}
	copyMap["userID"] = mycli.userID
	copyMap["instanceName"] = instance
	enhanced, err := prepareMediaJSON(copyMap)
	if err != nil {
		event.Close()
		log.Error().Err(err).Msg("Failed to prepare media delivery")
		return
	}
	safeGo("deliverMediaEvent", func() {
		defer event.Close()
		defer enhanced.Close()
		ctx, cancel, err := mediaContext(context.Background())
		if err != nil {
			return
		}
		defer cancel()
		var deliveries sync.WaitGroup
		for _, hook := range []struct {
			url string
			key []byte
		}{{userURL, key}, {*globalWebhook, globalHMACKeyEncrypted}} {
			if hook.url == "" {
				continue
			}
			deliveries.Add(1)
			safeGo("mediaWebhook", func() {
				defer deliveries.Done()
				deliverMediaHook(ctx, hook.url, mycli.userID, hook.key, event, enhanced, instance)
			})
		}
		if err := publishMediaToRabbit(enhanced); err != nil {
			log.Error().Err(err).Msg("Failed to publish media event")
		}
		deliveries.Wait()
	})
}
