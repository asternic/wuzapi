package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

func processStickerFile(parent context.Context, input *mediaFile, mimeOverride, packID, packName, packPublisher string, emojis []string) (*mediaFile, error) {
	ctx, cancel, err := mediaContext(parent)
	if err != nil {
		return nil, err
	}
	defer cancel()
	store, _ := getMediaStore()
	release, err := store.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	mimeType, err := input.sniff()
	if err != nil {
		return nil, err
	}
	if mimeOverride != "" {
		mimeType = mimeOverride
	}
	result := input
	video := strings.HasPrefix(mimeType, "video/") || mimeType == "image/gif"
	still := mimeType == "image/jpeg" || mimeType == "image/png" || mimeType == "image/jpg"
	if video || still {
		converted, out, err := store.create("", "image/webp")
		if err != nil {
			return nil, err
		}
		out.Close()
		args := []string{"-y", "-i", input.Path, "-vf", "scale=512:512", "-c:v", "libwebp", "-lossless", "1"}
		kind := "image"
		if video {
			kind = "video/gif"
			args = []string{"-y", "-t", "10", "-i", input.Path, "-vf", "fps=15,scale=512:512", "-loop", "0", "-an", "-vsync", "0", "-fs", "1000000", "-c:v", "libwebp", "-qscale:v", "10"}
		}
		args = append(args, "-f", "webp", converted.Path)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		var diagnostics cappedMediaLog
		cmd.Stdout = &diagnostics
		cmd.Stderr = &diagnostics
		if err = cmd.Run(); err != nil {
			converted.Close()
			return nil, fmt.Errorf("failed to convert %s sticker to webp: %w: %s", kind, err, diagnostics.String())
		}
		if err = converted.refresh(); err != nil {
			converted.Close()
			return nil, err
		}
		result = converted
		mimeType = "image/webp"
	}
	result.MIME = mimeType
	if mimeType == "image/webp" {
		if meta := buildStickerMetadata(packID, packName, packPublisher, emojis); meta != nil {
			updated, err := injectMediaEXIF(ctx, result, buildWhatsAppEXIF(meta))
			if err != nil {
				log.Warn().Err(err).Msg("failed to inject EXIF chunk; sending sticker without metadata")
			} else {
				if result != input {
					result.Close()
				}
				result = updated
			}
		}
	}
	return result, nil
}

type cappedMediaLog struct{ bytes.Buffer }

func (b *cappedMediaLog) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 8192 - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}

// Copy RIFF chunks directly, replacing EXIF and setting the VP8X flag. Chunk
// payloads (including animated frames) are never loaded into a byte slice.
func injectMediaEXIF(ctx context.Context, input *mediaFile, exif []byte) (*mediaFile, error) {
	r, err := input.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var header [12]byte
	if _, err = io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	if !isValidWebP(header[:]) {
		return nil, fmt.Errorf("not a RIFF WEBP file")
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil, err
	}
	// Inspect headers only, as VP8X need not be the first chunk in legacy inputs.
	hasVP8X := false
	for pos := int64(12); pos+8 <= input.Size; {
		var chunk [8]byte
		if _, err = r.ReadAt(chunk[:], pos); err != nil {
			return nil, err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		if pos+8+size > input.Size {
			return nil, fmt.Errorf("truncated webp chunk: %s", chunk[:4])
		}
		if string(chunk[:4]) == "VP8X" && size >= 10 {
			hasVP8X = true
		}
		pos += 8 + size + (size & 1)
	}
	store, _ := getMediaStore()
	result, out, err := store.create(input.Name, "image/webp")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		out.Close()
		if !success {
			result.Close()
		}
	}()
	if _, err = out.Write(header[:]); err != nil {
		return nil, err
	}
	if !hasVP8X {
		if _, err = out.Write(createVP8XChunk(cfg.Width, cfg.Height)); err != nil {
			return nil, err
		}
	}
	for pos := int64(12); pos+8 <= input.Size; {
		var chunk [8]byte
		if _, err = r.ReadAt(chunk[:], pos); err != nil {
			return nil, err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		tag := string(chunk[:4])
		if tag != "EXIF" {
			if _, err = out.Write(chunk[:]); err != nil {
				return nil, err
			}
			offset, length := pos+8, size
			if tag == "VP8X" && size >= 10 {
				var flag [1]byte
				if _, err = r.ReadAt(flag[:], offset); err != nil {
					return nil, err
				}
				flag[0] |= vp8xFlagEXIF
				if _, err = out.Write(flag[:]); err != nil {
					return nil, err
				}
				offset++
				length--
			}
			if _, err = io.Copy(out, contextReader{ctx, io.NewSectionReader(r, offset, length)}); err != nil {
				return nil, err
			}
			if size&1 == 1 {
				if _, err = out.Write([]byte{0}); err != nil {
					return nil, err
				}
			}
		}
		pos += 8 + size + (size & 1)
	}
	var chunk bytes.Buffer
	writeChunk(&chunk, "EXIF", exif)
	if _, err = out.Write(chunk.Bytes()); err != nil {
		return nil, err
	}
	size, err := out.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(size-8))
	if _, err = out.WriteAt(length[:], 4); err != nil {
		return nil, err
	}
	if err = out.Close(); err != nil {
		return nil, err
	}
	result.Size = size
	success = true
	return result, nil
}
