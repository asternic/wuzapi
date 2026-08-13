package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
)

const (
	downloadTimeoutImage    = 2 * time.Minute
	downloadTimeoutAudio    = 5 * time.Minute
	downloadTimeoutDocument = 10 * time.Minute
	downloadTimeoutVideo    = 10 * time.Minute
	downloadTimeoutSticker  = 1 * time.Minute
)

type mediaS3Config struct {
	Enabled       string
	MediaDelivery string
}

// lazyBase64 carries raw media bytes through postmap without eagerly
// encoding them
type lazyBase64 struct {
	data []byte
}

func (b lazyBase64) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.StdEncoding.EncodeToString(b.data))
}

// resolves MIME Type based on WhatsApp declared preferably
func resolveMimeType(declaredMimeType string, data []byte) string {
	if declaredMimeType != "" {
		return declaredMimeType
	}
	return http.DetectContentType(data)
}

func (mycli *MyClient) processMedia(
	msg whatsmeow.DownloadableMessage,
	mimeType string,
	fallbackExt string,
	timeout time.Duration,
	isIncoming bool,
	chatJID string,
	messageID string,
	s3cfg mediaS3Config,
	postmap map[string]interface{},
	extraKeys map[string]interface{},
) {
	tmpDir := filepath.Join("/tmp", "user_"+mycli.userID)
	if err := os.MkdirAll(tmpDir, 0751); err != nil {
		log.Error().Err(err).Msg("Could not create temporary directory")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	data, err := mycli.WAClient.Download(ctx, msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to download media")
		return
	}

	ext := fallbackExt
	//prefer fallbackExt when available
	if len(ext) == 0 {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	tmpPath := filepath.Join(tmpDir, messageID+ext)

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.Error().Err(err).Msg("Failed to save media to temporary file")
		return
	}
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			log.Error().Err(err).Msg("Failed to delete temporary file")
		} else {
			log.Info().Str("path", tmpPath).Msg("Temporary file deleted")
		}
	}()

	if s3cfg.Enabled == "true" && (s3cfg.MediaDelivery == "s3" || s3cfg.MediaDelivery == "both") {
		s3Data, err := GetS3Manager().ProcessMediaForS3(
			ctx,
			mycli.userID,
			chatJID,
			messageID,
			data,
			mimeType,
			filepath.Base(tmpPath),
			isIncoming,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to upload media to S3")
		} else {
			postmap["s3"] = s3Data
		}
	}

	if s3cfg.MediaDelivery == "base64" || s3cfg.MediaDelivery == "both" {
		postmap["base64"] = lazyBase64{data: data}
		postmap["mimeType"] = resolveMimeType(mimeType, data)
		postmap["fileName"] = filepath.Base(tmpPath)
	}

	for k, v := range extraKeys {
		postmap[k] = v
	}

	log.Info().Str("path", tmpPath).Msg("Media processed")
}
