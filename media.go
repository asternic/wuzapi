package main

import (
	"context"
	"mime"
	"os"
	"path/filepath"
	"strings"
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

// resolveMediaFileName picks the filename surfaced in the webhook and S3
// payloads. It prefers the sender's original filename (e.g. a document's
// FileName) and falls back to the temp file's base name when none was
// provided. It strips any directory component a sender may have embedded so a
// crafted name can't escape into a path. Backslashes are normalised to forward
// slashes first: filepath.Base only treats the OS separator as a divider, so on
// Linux (where this typically runs) a Windows-style "..\\..\\name" would not be
// stripped otherwise.
func resolveMediaFileName(originalFileName, fallbackPath string) string {
	if originalFileName != "" {
		return filepath.Base(strings.ReplaceAll(originalFileName, "\\", "/"))
	}
	return filepath.Base(fallbackPath)
}

func (mycli *MyClient) processMedia(
	msg whatsmeow.DownloadableMessage,
	mimeType string,
	fallbackExt string,
	timeout time.Duration,
	isIncoming bool,
	chatJID string,
	messageID string,
	originalFileName string,
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
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		ext = exts[0]
	}
	tmpPath := filepath.Join(tmpDir, messageID+ext)
	displayName := resolveMediaFileName(originalFileName, tmpPath)

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
			displayName,
			isIncoming,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to upload media to S3")
		} else {
			postmap["s3"] = s3Data
		}
	}

	if s3cfg.MediaDelivery == "base64" || s3cfg.MediaDelivery == "both" {
		b64, mime_, err := fileToBase64(tmpPath)
		if err != nil {
			log.Error().Err(err).Msg("Failed to convert media to base64")
			return
		}
		postmap["base64"] = b64
		postmap["mimeType"] = mime_
		postmap["fileName"] = displayName
	}

	for k, v := range extraKeys {
		postmap[k] = v
	}

	log.Info().Str("path", tmpPath).Msg("Media processed")
}
