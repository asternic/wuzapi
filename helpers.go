package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

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

func fetchURLBytes(resourceURL string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", resourceURL, nil)
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

	limitedBody := http.MaxBytesReader(nil, resp.Body, 10*1024*1024)
	data, err := io.ReadAll(limitedBody)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	return data, contentType, nil
}

// Update entry in User map
func updateUserInfo(values interface{}, field string, value string) interface{} {
	log.Debug().Str("field", field).Str("value", value).Msg("User info updated")
	values.(Values).m[field] = value
	return values
}

// webhook for regular messages
func callHook(myurl string, payload map[string]string, id string) {
	callHookWithHmac(myurl, payload, id, nil)
}

// webhook for regular messages with HMAC
func callHookWithHmac(myurl string, payload map[string]string, id string, encryptedHmacKey []byte) {
	log.Info().Str("url", myurl).Msg("Sending POST to client " + id)

	// Log the payload map
	log.Debug().Msg("Payload:")
	for key, value := range payload {
		log.Debug().Str(key, value).Msg("")
	}

	client := clientManager.GetHTTPClient(id)

	format := os.Getenv("WEBHOOK_FORMAT")
	if format == "json" {
		// Send as pure JSON
		// The original payload is a map[string]string, but we want to send the postmap (map[string]interface{})
		// So we try to decode the jsonData field if it exists, otherwise we send the original payload
		var body interface{} = payload
		var jsonBody []byte

		if jsonStr, ok := payload["jsonData"]; ok {
			var postmap map[string]interface{}
			err := json.Unmarshal([]byte(jsonStr), &postmap)
			if err == nil {
				body = postmap
			}
		}

		// Marshal body to JSON for HMAC signature
		jsonBody, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			log.Error().Err(marshalErr).Msg("Failed to marshal body for HMAC")
		}

		// Generate HMAC signature if key exists
		var hmacSignature string
		var err error
		if len(encryptedHmacKey) > 0 && len(jsonBody) > 0 {
			hmacSignature, err = generateHmacSignature(jsonBody, encryptedHmacKey)
			if err != nil {
				log.Error().Err(err).Msg("Failed to generate HMAC signature")
			} else {
				log.Debug().Str("hmacSignature", hmacSignature).Msg("Generated HMAC signature")
			}
		}

		req := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(body)

		// Add HMAC signature header if available
		if hmacSignature != "" {
			req.SetHeader("x-hmac-signature", hmacSignature)
		}

		_, postErr := req.Post(myurl)
		if postErr != nil {
			log.Debug().Str("error", postErr.Error())
		}
	} else {
		/// Default: send as form-urlencoded
		// Generate HMAC signature if encrypted key exists
		var hmacSignature string
		var err error
		if len(encryptedHmacKey) > 0 {
			formData := url.Values{}
			for k, v := range payload {
				formData.Add(k, v)
			}
			formString := formData.Encode() // "token=abc&message=hello"

			hmacSignature, err = generateHmacSignature([]byte(formString), encryptedHmacKey)
			if err != nil {
				log.Error().Err(err).Msg("Failed to generate HMAC signature")
			} else {
				log.Debug().Str("hmacSignature", hmacSignature).Msg("Generated HMAC signature for form-data")
			}
		}

		req := client.R().SetFormData(payload)
		// Add HMAC signature header if available
		if hmacSignature != "" {
			req.SetHeader("x-hmac-signature", hmacSignature)
		}

		_, postErr := req.Post(myurl)
		if postErr != nil {
			log.Debug().Str("error", postErr.Error())
		}
	}
}

// webhook for messages with file attachments
func callHookFile(myurl string, payload map[string]string, id string, file string) error {
	return callHookFileWithHmac(myurl, payload, id, file, nil)
}

// webhook for messages with file attachments and HMAC
func callHookFileWithHmac(myurl string, payload map[string]string, id string, file string, encryptedHmacKey []byte) error {
	log.Info().Str("file", file).Str("url", myurl).Msg("Sending POST")

	client := clientManager.GetHTTPClient(id)

	// Create final payload map
	finalPayload := make(map[string]string)
	for k, v := range payload {
		finalPayload[k] = v
	}

	finalPayload["file"] = file

	log.Debug().Interface("finalPayload", finalPayload).Msg("Final payload to be sent")

	// Generate HMAC signature if key exists
	var hmacSignature string
	var jsonPayload []byte
	var err error

	if len(encryptedHmacKey) > 0 {
		// Para multipart/form-data, assinar a representação JSON do payload final
		jsonPayload, err = json.Marshal(finalPayload)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal payload for HMAC")
		} else {
			hmacSignature, err = generateHmacSignature(jsonPayload, encryptedHmacKey)
			if err != nil {
				log.Error().Err(err).Msg("Failed to generate HMAC signature")
			} else {
				log.Debug().Str("hmacSignature", hmacSignature).Msg("Generated HMAC signature for file webhook")
			}
		}
	}

	req := client.R().
		SetFiles(map[string]string{
			"file": file,
		}).
		SetFormData(finalPayload)

	// Add HMAC signature header if available
	if hmacSignature != "" {
		req.SetHeader("x-hmac-signature", hmacSignature)
	}

	resp, err := req.Post(myurl)

	if err != nil {
		log.Error().Err(err).Str("url", myurl).Msg("Failed to send POST request")
		return fmt.Errorf("failed to send POST request: %w", err)
	}

	log.Debug().Interface("payload", finalPayload).Msg("Payload sent to webhook")
	log.Info().Int("status", resp.StatusCode()).Str("body", string(resp.Body())).Msg("POST request completed")

	return nil
}

func (s *server) respondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Error().Err(err).Msg("Failed to encode JSON response")
		w.WriteHeader(http.StatusInternalServerError)
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
