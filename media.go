package main

import (
	"context"
	"mime"
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
	ctx, cancel, err := mediaContext(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not initialize media storage")
		return
	}
	defer cancel()
	store, _ := getMediaStore()
	release, err := store.acquire(ctx)
	if err != nil {
		return
	}
	defer release()
	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()
	media, file, err := store.create(incomingMediaName(messageID, mimeType, fallbackExt), mimeType)
	if err != nil {
		log.Error().Err(err).Msg("Could not create media file")
		return
	}
	retained := false
	defer func() {
		if !retained {
			media.Close()
		}
	}()
	err = mycli.WAClient.DownloadToFile(ctx, msg, file)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = media.refresh()
	}
	if err != nil {
		log.Error().Err(err).Msg("Failed to download media")
		return
	}
	retained = mycli.attachDownloadedMedia(ctx, media, mimeType, chatJID, messageID, isIncoming, s3cfg, postmap)

	for k, v := range extraKeys {
		postmap[k] = v
	}
	log.Info().Int64("size", media.Size).Msg("Media processed")
}

// Returns whether the event now owns media; S3-only consumers finish inline.
func (mycli *MyClient) attachDownloadedMedia(ctx context.Context, media *mediaFile, mimeType, chatJID, messageID string, isIncoming bool, s3cfg mediaS3Config, postmap map[string]interface{}) bool {
	if s3cfg.Enabled == "true" && (s3cfg.MediaDelivery == "s3" || s3cfg.MediaDelivery == "both") {
		reader, openErr := media.Open()
		if openErr == nil {
			data, uploadErr := GetS3Manager().ProcessMediaReaderForS3(ctx, mycli.userID, chatJID, messageID, reader, media.Size, mimeType, media.Name, isIncoming)
			reader.Close()
			if uploadErr != nil {
				log.Error().Err(uploadErr).Msg("Failed to upload media to S3")
			} else {
				postmap["s3"] = data
			}
		} else {
			log.Error().Err(openErr).Msg("Failed to open media for S3")
		}
	}
	if s3cfg.MediaDelivery == "base64" || s3cfg.MediaDelivery == "both" {
		sniffed, err := media.sniff()
		if err != nil {
			log.Error().Err(err).Msg("Failed to sniff media")
			return false
		}
		if previous, ok := postmap["base64"].(*mediaFile); ok {
			previous.Close()
		}
		postmap["base64"] = media
		postmap["mimeType"] = sniffed
		postmap["fileName"] = media.Name
		return true // Event handler owns the reference until delivery preparation ends.
	}
	return false
}

func incomingMediaName(messageID, mimeType, fallbackExt string) string {
	ext := fallbackExt
	if extensions, _ := mime.ExtensionsByType(mimeType); len(extensions) > 0 {
		ext = extensions[0]
	}
	return filepath.Base(messageID + ext)
}
