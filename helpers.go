package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"

	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"

	"github.com/patrickmn/go-cache"

	_ "golang.org/x/image/webp"

	"github.com/PuerkitoBio/goquery"
	"github.com/jmoiron/sqlx"
	"github.com/nfnt/resize"
	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
)

const (
	openGraphFetchTimeout    = 5 * time.Second
	openGraphPageMaxBytes    = 2 * 1024 * 1024  // 2MB
	openGraphImageMaxBytes   = 10 * 1024 * 1024 // 10MB
	openGraphThumbnailWidth  = 100
	openGraphThumbnailHeight = 100
	openGraphJpegQuality     = 80
	openGraphMaxImageDim     = 4000 // Max width or height for Open Graph images
	openGraphUserFetchLimit  = 20   // Limit concurrent Open Graph fetches per user
)

type WebhookFileErrorPayload struct {
	URL              string                 `json:"url"`
	Payload          map[string]interface{} `json:"payload"`
	UserID           string                 `json:"userID"`
	EncryptedHmacKey string                 `json:"encryptedHmacKey"`
	FilePath         string                 `json:"filePath"`
	AttemptTime      time.Time              `json:"attemptTime"`
	ErrorMessage     string                 `json:"errorMessage"`
}

type WebhookErrorPayload struct {
	URL              string                 `json:"url"`
	Payload          map[string]interface{} `json:"payload"`
	UserID           string                 `json:"userID"`
	EncryptedHmacKey string                 `json:"encryptedHmacKey"`
	AttemptTime      time.Time              `json:"attemptTime"`
	ErrorMessage     string                 `json:"errorMessage"`
}
type openGraphResult struct {
	Title       string
	Description string
	ImageData   []byte
}

type UserSemaphoreManager struct {
	pools sync.Map
}

func NewUserSemaphoreManager() *UserSemaphoreManager {
	return &UserSemaphoreManager{}
}

func (usm *UserSemaphoreManager) ForUser(userID string) chan struct{} {
	// LoadOrStore provides an atomic way to get or create a semaphore.
	pool, _ := usm.pools.LoadOrStore(userID, make(chan struct{}, openGraphUserFetchLimit))
	return pool.(chan struct{})
}

var (
	urlRegex = regexp.MustCompile(`https?://[^\s"']*[^\"'\s\.,!?()[\]{}]`)

	userSemaphoreManager = NewUserSemaphoreManager()

	openGraphGroup singleflight.Group

	openGraphCache = cache.New(5*time.Minute, 10*time.Minute) // Cache Open Graph data for 5 minutes, cleanup every 10 minutes

)

var (
	webhookHTTPClient     *resty.Client
	webhookHTTPClientOnce sync.Once
)

func getWebhookHTTPClient() *resty.Client {
	webhookHTTPClientOnce.Do(func() {
		c := resty.New()
		c.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
		if *waDebug == "DEBUG" {
			c.SetDebug(true)
		}
		c.SetTimeout(30 * time.Second)
		c.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		c.OnError(func(req *resty.Request, err error) {
			if v, ok := err.(*resty.ResponseError); ok {
				log.Debug().Str("response", v.Response.String()).Msg("resty error")
				log.Error().Err(v.Err).Msg("resty error")
			}
		})
		webhookHTTPClient = c
	})
	return webhookHTTPClient
}

func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func isHTTPURL(input string) bool {
	parsed, err := url.ParseRequestURI(input)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}
func fetchURLBytes(ctx context.Context, resourceURL string, limit int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := globalHTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	lr := io.LimitReader(resp.Body, limit+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("response exceeds allowed size (%d bytes)", limit)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	return data, contentType, nil
}

func getOpenGraphData(ctx context.Context, urlStr string, userID string) (title, description string, imageData []byte) {
	// Check cache first
	if cachedData, found := openGraphCache.Get(urlStr); found {
		if data, ok := cachedData.(openGraphResult); ok {
			log.Debug().Str("url", urlStr).Msg("Open Graph data fetched from cache")
			return data.Title, data.Description, data.ImageData
		}
	}

	v, err, _ := openGraphGroup.Do(urlStr, func() (res any, err error) {
		ctx, cancel := context.WithTimeout(ctx, openGraphFetchTimeout)
		defer cancel()

		// Acquire a token from the semaphore pool
		userPool := userSemaphoreManager.ForUser(userID)
		select {
		case userPool <- struct{}{}:
			defer func() { <-userPool }()
		case <-ctx.Done():
			log.Warn().Str("url", urlStr).Msg("Open Graph data fetch timed out while waiting for a worker")
			return nil, ctx.Err()
		}

		// Recover from panics and convert to error
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error().
					Interface("panic_info", r).
					Str("url", urlStr).
					Bytes("stack", stack).
					Msg("Panic recovered while fetching Open Graph data")
				err = fmt.Errorf("panic: %v", r)
			}
		}()

		// Fetch Open Graph data
		title, description, imageData := fetchOpenGraphData(ctx, urlStr)

		// Store in cache
		openGraphCache.Set(urlStr, openGraphResult{title, description, imageData}, cache.DefaultExpiration)

		return openGraphResult{title, description, imageData}, nil
	})

	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("Error fetching Open Graph data via singleflight")
		return "", "", nil
	}

	if v == nil {
		return "", "", nil
	}

	data := v.(openGraphResult)
	return data.Title, data.Description, data.ImageData
}

// Update entry in User map
func updateUserInfo(values interface{}, field string, value string) interface{} {
	log.Debug().Str("field", field).Str("value", value).Msg("User info updated")
	values.(Values).m[field] = value
	return values
}

// webhook for regular messages
func callHook(myurl string, payload map[string]string, userID string) {
	callHookWithHmac(myurl, payload, userID, nil)
}

// webhook for regular messages with HMAC
func callHookWithHmac(myurl string, payload map[string]string, userID string, encryptedHmacKey []byte) {
	log.Info().Str("url", myurl).Str("userID", userID).Msg("Sending POST to client with retry logic")

	client := getWebhookHTTPClient()

	// Retry settings
	maxRetries := 1
	if *webhookRetryEnabled {
		maxRetries = *webhookRetryCount
	}

	var lastError error

	var body interface{} = payload

	// Starts the retry loop.
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoffFactor := 1 << uint(attempt-1)

			// Calculate the final delay.
			delayDuration := time.Duration(*webhookRetryDelaySeconds) * time.Second * time.Duration(backoffFactor)

			log.Warn().
				Int("attempt", attempt+1).
				Str("url", myurl).
				Dur("delay", delayDuration).
				Msg("Retrying webhook request with exponential backoff...")

			time.Sleep(delayDuration)
		}

		var req *resty.Request
		var hmacSignature string
		var marshalErr error

		format := os.Getenv("WEBHOOK_FORMAT")

		if format == "json" {
			var jsonBody []byte

			if jsonStr, ok := payload["jsonData"]; ok {
				var postmap map[string]interface{}

				if err := json.Unmarshal([]byte(jsonStr), &postmap); err == nil {
					if instanceName, ok := payload["instanceName"]; ok {
						postmap["instanceName"] = instanceName
					}
					postmap["userID"] = userID
					body = postmap
				}
			}

			// Marshal body to JSON for HMAC signature
			jsonBody, marshalErr = json.Marshal(body)
			if marshalErr != nil {
				log.Error().Err(marshalErr).Msg("Failed to marshal body for HMAC")
			}

			// Generate HMAC signature if key exists
			if len(encryptedHmacKey) > 0 && len(jsonBody) > 0 {
				var err error
				hmacSignature, err = generateHmacSignature(jsonBody, encryptedHmacKey)
				if err != nil {
					log.Error().Err(err).Msg("Failed to generate HMAC signature")
				}
			}

			req = client.R().SetHeader("Content-Type", "application/json").SetBody(body)

		} else {

			if len(encryptedHmacKey) > 0 {
				formData := url.Values{}
				for k, v := range payload {
					formData.Add(k, v)
				}
				formString := formData.Encode()
				var err error
				hmacSignature, err = generateHmacSignature([]byte(formString), encryptedHmacKey)
				if err != nil {
					log.Error().Err(err).Msg("Failed to generate HMAC signature")
				}
			}
			req = client.R().SetFormData(payload)
			body = payload
		}

		if hmacSignature != "" {
			req.SetHeader("x-hmac-signature", hmacSignature)
		}

		resp, postErr := req.Post(myurl)

		lastError = postErr

		if postErr != nil {
			log.Error().Err(postErr).Int("attempt", attempt+1).Str("url", myurl).Msg("Webhook failed due to network/IO error")
			continue
		}

		if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
			lastError = fmt.Errorf("unexpected status code: %d. Body: %s", resp.StatusCode(), string(resp.Body()))
			log.Error().
				Int("status", resp.StatusCode()).
				Int("attempt", attempt+1).
				Str("url", myurl).
				Msg("Webhook failed due to non-2xx status code")

			if !*webhookRetryEnabled {
				break
			}
			continue
		}

		log.Info().Int("status", resp.StatusCode()).Str("url", myurl).Msg("Webhook call successful")
		return
	}

	if lastError != nil {
		log.Error().Str("url", myurl).Msg("Webhook permanently failed after all retries. Sending to error queue...")

		errorPayloadMap := make(map[string]interface{})
		if p, ok := body.(map[string]string); ok {

			for k, v := range p {
				errorPayloadMap[k] = v
			}
		} else if p, ok := body.(map[string]interface{}); ok {

			errorPayloadMap = p
		}

		errorPayload := WebhookErrorPayload{
			URL:              myurl,
			Payload:          errorPayloadMap,
			UserID:           userID,
			EncryptedHmacKey: hex.EncodeToString(encryptedHmacKey),
			AttemptTime:      time.Now(),
			ErrorMessage:     lastError.Error(),
		}

		PublishDataErrorToQueue(errorPayload)
	}
}

// webhook for messages with file attachments
func callHookFile(myurl string, payload map[string]string, userID string, file string) error {
	return callHookFileWithHmac(myurl, payload, userID, file, nil)
}

// webhook for messages with file attachments and HMAC
func callHookFileWithHmac(myurl string, payload map[string]string, userID string, file string, encryptedHmacKey []byte) error {
	log.Info().Str("file", file).Str("url", myurl).Msg("Sending POST with retry logic")

	client := getWebhookHTTPClient()

	maxRetries := 1
	if *webhookRetryEnabled {
		maxRetries = *webhookRetryCount
	}

	var lastError error

	finalPayload := make(map[string]string)
	for k, v := range payload {
		finalPayload[k] = v
	}
	finalPayload["file"] = file

	// 2. Loop Retry
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoffFactor := 1 << uint(attempt-1)

			delayDuration := time.Duration(*webhookRetryDelaySeconds) * time.Second * time.Duration(backoffFactor)

			log.Warn().
				Int("attempt", attempt+1).
				Str("url", myurl).
				Dur("delay", delayDuration).
				Msg("Retrying file webhook request with exponential backoff...")

			time.Sleep(delayDuration)
		}

		var hmacSignature string
		var jsonPayload []byte

		if len(encryptedHmacKey) > 0 {
			var err error
			jsonPayload, err = json.Marshal(finalPayload)
			if err != nil {
				log.Error().Err(err).Msg("Failed to marshal payload for HMAC")
			} else {
				hmacSignature, err = generateHmacSignature(jsonPayload, encryptedHmacKey)
				if err != nil {
					log.Error().Err(err).Msg("Failed to generate HMAC signature")
				}
			}
		}

		req := client.R().
			SetFiles(map[string]string{
				"file": file,
			}).
			SetFormData(finalPayload)

		if hmacSignature != "" {
			req.SetHeader("x-hmac-signature", hmacSignature)
		}

		resp, postErr := req.Post(myurl)

		lastError = postErr

		if postErr != nil {
			log.Error().Err(postErr).Int("attempt", attempt+1).Str("url", myurl).Msg("File webhook failed due to network/IO error")
			continue
		}

		if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
			lastError = fmt.Errorf("unexpected status code: %d. Body: %s", resp.StatusCode(), string(resp.Body()))
			log.Error().
				Int("status", resp.StatusCode()).
				Int("attempt", attempt+1).
				Str("url", myurl).
				Msg("File webhook failed due to non-2xx status code")

			if !*webhookRetryEnabled {
				break
			}
			continue
		}

		log.Info().Int("status", resp.StatusCode()).Str("url", myurl).Msg("File webhook call successful")
		return nil
	}

	if lastError != nil {
		log.Error().Str("url", myurl).Msg("File webhook permanently failed after all retries. Sending to error queue...")

		errorPayloadMap := make(map[string]interface{})
		for k, v := range finalPayload {
			errorPayloadMap[k] = v
		}

		errorPayload := WebhookFileErrorPayload{
			URL:              myurl,
			Payload:          errorPayloadMap,
			UserID:           userID,
			EncryptedHmacKey: hex.EncodeToString(encryptedHmacKey),
			FilePath:         file,
			AttemptTime:      time.Now(),
			ErrorMessage:     lastError.Error(),
		}

		PublishFileErrorToQueue(errorPayload)

		return fmt.Errorf("webhook failed permanently: %w", lastError)
	}

	return nil
}

func (s *server) respondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		log.Error().Err(err).Msg("Failed to encode JSON response")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statusCode)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Error().Err(err).Msg("Failed to write response body")
	}
}

// ProcessOutgoingMedia handles media processing for outgoing messages with S3 support
func ProcessOutgoingMedia(userID string, contactJID string, messageID string, data []byte, mimeType string, fileName string, db *sqlx.DB) (map[string]interface{}, error) {
	// Check if S3 is enabled for this user
	var s3Config struct {
		Enabled       bool   `db:"s3_enabled"`
		MediaDelivery string `db:"media_delivery"`
	}
	err := db.Get(&s3Config, "SELECT s3_enabled, media_delivery FROM users WHERE id = $1", userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get S3 config")
		s3Config.Enabled = false
		s3Config.MediaDelivery = "base64"
	}

	// Process S3 upload if enabled
	if s3Config.Enabled && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
		// Process S3 upload (outgoing messages are always in outbox)
		s3Data, err := GetS3Manager().ProcessMediaForS3(
			context.Background(),
			userID,
			contactJID,
			messageID,
			data,
			mimeType,
			fileName,
			false, // isIncoming = false for sent messages
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to upload media to S3")
			// Continue even if S3 upload fails
		} else {
			return s3Data, nil
		}
	}

	return nil, nil
}

// generateHmacSignature generates HMAC-SHA256 signature for webhook payload
func generateHmacSignature(payload []byte, encryptedHmacKey []byte) (string, error) {
	if len(encryptedHmacKey) == 0 {
		return "", nil
	}

	// Decrypt HMAC key
	hmacKey, err := decryptHMACKey(encryptedHmacKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt HMAC key: %w", err)
	}

	// Generate HMAC
	h := hmac.New(sha256.New, []byte(hmacKey))
	h.Write(payload)

	return hex.EncodeToString(h.Sum(nil)), nil
}

func encryptHMACKey(plainText string) ([]byte, error) {
	if *globalEncryptionKey == "" {
		return nil, fmt.Errorf("encryption key not configured")
	}

	block, err := aes.NewCipher([]byte(*globalEncryptionKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return ciphertext, nil
}

// decryptHMACKey decrypts HMAC key using AES-GCM
func decryptHMACKey(encryptedData []byte) (string, error) {
	if *globalEncryptionKey == "" {
		return "", fmt.Errorf("encryption key not configured")
	}

	block, err := aes.NewCipher([]byte(*globalEncryptionKey))
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

func extractFirstURL(text string) string {
	match := urlRegex.FindString(text)
	if match == "" {
		return ""
	}

	return match
}
func fetchOpenGraphData(ctx context.Context, urlStr string) (string, string, []byte) {
	pageData, _, err := fetchURLBytes(ctx, urlStr, openGraphPageMaxBytes)
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to fetch URL for Open Graph data")
		return "", "", nil
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(pageData))
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to parse HTML for Open Graph data")
		return "", "", nil
	}

	title := doc.Find(`meta[property="og:title"]`).AttrOr("content", "")
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").Text())
	}

	description := doc.Find(`meta[property="og:description"]`).AttrOr("content", "")
	if description == "" {
		description = doc.Find(`meta[name="description"]`).AttrOr("content", "")
	}

	var imageURLStr string
	selectors := []struct {
		selector string
		attr     string
	}{
		{`meta[property="og:image"]`, "content"},
		{`meta[property="twitter:image"]`, "content"},
		{`link[rel="apple-touch-icon"]`, "href"},
		{`link[rel="icon"]`, "href"},
	}

	for _, s := range selectors {
		imageURLStr, _ = doc.Find(s.selector).Attr(s.attr)
		if imageURLStr != "" {
			break
		}
	}

	pageURL, err := url.Parse(urlStr)
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to parse page URL for resolving image URL")
		return title, description, nil
	}

	imageData := fetchOpenGraphImage(ctx, pageURL, imageURLStr)
	return title, description, imageData
}

func fetchOpenGraphImage(ctx context.Context, pageURL *url.URL, imageURLStr string) []byte {
	imageURL, err := url.Parse(imageURLStr)
	if err != nil {
		log.Warn().Err(err).Str("imageURL", imageURLStr).Msg("Failed to parse Open Graph image URL")
		return nil
	}

	resolvedImageURL := pageURL.ResolveReference(imageURL).String()
	imgBytes, _, err := fetchURLBytes(ctx, resolvedImageURL, openGraphImageMaxBytes)
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to fetch Open Graph image")
		return nil
	}

	imgConfig, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to decode Open Graph image config")
		return nil
	}

	if imgConfig.Width > openGraphMaxImageDim || imgConfig.Height > openGraphMaxImageDim {
		log.Warn().
			Int("width", imgConfig.Width).
			Int("height", imgConfig.Height).
			Str("imageURL", resolvedImageURL).
			Msg("Open Graph image dimensions too large")
		return nil
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to decode Open Graph image")
		return nil
	}

	thumbnail := resize.Thumbnail(openGraphThumbnailWidth, openGraphThumbnailHeight, img, resize.Lanczos3)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumbnail, &jpeg.Options{Quality: openGraphJpegQuality}); err != nil {
		log.Warn().Err(err).Msg("Failed to encode thumbnail to JPEG")
		return nil
	}

	return buf.Bytes()
}

func convertVideoStickerToWebP(input []byte) ([]byte, error) {
	inFile, err := os.CreateTemp("", "sticker-input-*.mp4")
	if err != nil {
		return nil, err
	}
	defer os.Remove(inFile.Name())
	defer inFile.Close()

	if _, err := inFile.Write(input); err != nil {
		return nil, err
	}

	outFile, err := os.CreateTemp("", "sticker-output-*.webp")
	if err != nil {
		return nil, err
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	qValue := 10
	filter := "fps=15,scale=512:512:force_original_aspect_ratio=increase,crop=512:512"
	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-t", "10",
		"-i", inFile.Name(),
		"-vf", filter,
		"-loop", "0",
		"-an",
		"-vsync", "0",
		"-fs", "1000000",
		"-c:v", "libwebp",
		"-qscale:v", fmt.Sprintf("%d", qValue),
		outPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Error().Err(err).Str("stderr", stderr.String()).Msg("ffmpeg failed converting video sticker")
		return nil, err
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func processStickerData(stickerData string, mimeOverride string, packID, packName, packPublisher string, emojis []string) ([]byte, string, error) {
	if !strings.HasPrefix(stickerData, "data") {
		return nil, "", fmt.Errorf("data should start with \"data:mime/type;base64,\"")
	}

	dataURL, err := dataurl.DecodeString(stickerData)
	if err != nil {
		return nil, "", fmt.Errorf("could not decode base64 encoded data from payload")
	}

	filedata := dataURL.Data
	detectedMimeType := http.DetectContentType(filedata)

	if mimeOverride != "" {
		detectedMimeType = mimeOverride
	}

	// If this is a video sticker, convert to animated WebP
	if strings.HasPrefix(detectedMimeType, "video/") {
		converted, err := convertVideoStickerToWebP(filedata)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert video sticker to webp: %w", err)
		}
		filedata = converted
		detectedMimeType = "image/webp"
	}

	// If we have sticker metadata and the content is WebP, embed EXIF metadata
	if strings.HasPrefix(detectedMimeType, "image/webp") {
		filedata = embedStickerEXIF(filedata, packID, packName, packPublisher, emojis)
	}

	return filedata, detectedMimeType, nil
}

// embedStickerEXIF injects WhatsApp sticker metadata into a WebP image.
func embedStickerEXIF(inputWebP []byte, packID, packName, packPublisher string, emojis []string) []byte {
	if packID == "" && packName == "" && packPublisher == "" && len(emojis) == 0 {
		return inputWebP
	}

	meta := map[string]interface{}{}
	if packID != "" {
		meta["sticker-pack-id"] = packID
	}
	if packName != "" {
		meta["sticker-pack-name"] = packName
	}
	if packPublisher != "" {
		meta["sticker-pack-publisher"] = packPublisher
	}
	if len(emojis) > 0 {
		meta["emojis"] = emojis
	}

	jsonBytes, err := json.Marshal(meta)
	if err != nil {
		return inputWebP
	}

	starting := []byte{0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00, 0x01, 0x00, 0x41, 0x57, 0x07, 0x00}
	ending := []byte{0x16, 0x00, 0x00, 0x00}

	var exifBuf bytes.Buffer
	exifBuf.Write(starting)
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(jsonBytes)))
	exifBuf.Write(lenBuf)
	exifBuf.Write(ending)
	exifBuf.Write(jsonBytes)

	out, err := injectWebPExifChunk(inputWebP, exifBuf.Bytes())
	if err != nil {
		log.Warn().Err(err).Msg("failed to inject EXIF chunk; sending sticker without metadata")
		return inputWebP
	}
	return out
}

// injectWebPExifChunk adds/replaces EXIF chunk and sets EXIF bit in VP8X if present.
func injectWebPExifChunk(in []byte, exif []byte) ([]byte, error) {
	if len(in) < 12 || string(in[0:4]) != "RIFF" || string(in[8:12]) != "WEBP" {
		return nil, fmt.Errorf("not a RIFF WEBP file")
	}

	var out bytes.Buffer
	out.Grow(len(in) + len(exif) + 32)
	out.WriteString("RIFF")
	out.Write([]byte{0, 0, 0, 0})
	out.WriteString("WEBP")

	pos := 12
	vp8xIndex := -1
	var chunks [][]byte
	for pos+8 <= len(in) {
		tag := string(in[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(in[pos+4 : pos+8]))
		dataStart := pos + 8
		dataEnd := dataStart + size
		if dataEnd > len(in) {
			return nil, fmt.Errorf("truncated webp chunk: %s", tag)
		}
		pad := size & 1
		next := dataEnd + pad
		if tag == "VP8X" && size >= 10 {
			vp8xIndex = len(chunks)
		}
		if tag != "EXIF" {
			chunk := make([]byte, 8+size+pad)
			copy(chunk[0:4], in[pos:pos+4])
			binary.LittleEndian.PutUint32(chunk[4:8], uint32(size))
			copy(chunk[8:8+size], in[dataStart:dataEnd])
			if pad == 1 {
				chunk[8+size] = 0
			}
			chunks = append(chunks, chunk)
		}
		pos = next
	}

	if vp8xIndex >= 0 {
		c := chunks[vp8xIndex]
		if len(c) >= 18 {
			c[8] = c[8] | 0x04
			chunks[vp8xIndex] = c
		}
	}

	for _, c := range chunks {
		out.Write(c)
	}

	exifSize := len(exif)
	out.WriteString("EXIF")
	sz := make([]byte, 4)
	binary.LittleEndian.PutUint32(sz, uint32(exifSize))
	out.Write(sz)
	out.Write(exif)
	if exifSize%2 == 1 {
		out.WriteByte(0)
	}

	b := out.Bytes()
	riffSize := uint32(len(b) - 8)
	binary.LittleEndian.PutUint32(b[4:8], riffSize)
	return b, nil
}
