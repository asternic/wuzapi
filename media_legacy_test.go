package main

// Baseline implementations retained only as compatibility fixtures. Production
// uses the file-backed pipeline in media_sources.go and media_sticker.go.
import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
	"image"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

func runFFmpegConversion(input []byte, inputExt string, ffmpegArgs func(inPath, outPath string) []string, errMsg string) ([]byte, error) {
	inFile, err := os.CreateTemp("", "sticker-input-*"+inputExt)
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

	args := ffmpegArgs(inFile.Name(), outPath)
	cmd := exec.Command("ffmpeg", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Error().Err(err).Str("stderr", stderr.String()).Msg(errMsg)
		return nil, err
	}

	return os.ReadFile(outPath)
}

func convertVideoStickerToWebP(input []byte) ([]byte, error) {
	return runFFmpegConversion(input, ".mp4", func(inPath, outPath string) []string {
		return []string{
			"-y",
			"-t", "10",
			"-i", inPath,
			"-vf", "fps=15,scale=512:512",
			"-loop", "0",
			"-an",
			"-vsync", "0",
			"-fs", "1000000",
			"-c:v", "libwebp",
			"-qscale:v", "10",
			outPath,
		}
	}, "ffmpeg failed converting video sticker")
}

func convertImageToWebP(input []byte) ([]byte, error) {
	return runFFmpegConversion(input, ".img", func(inPath, outPath string) []string {
		return []string{
			"-y",
			"-i", inPath,
			"-vf", "scale=512:512",
			"-c:v", "libwebp",
			"-lossless", "1",
			outPath,
		}
	}, "ffmpeg failed converting image sticker")
}

func processStickerData(stickerData string, mimeOverride string, packID, packName, packPublisher string, emojis []string) ([]byte, string, error) {
	if !strings.HasPrefix(stickerData, "data") {
		return nil, "", fmt.Errorf("data should start with \"data:mime/type;base64,\"")
	}

	dataURL, err := dataurl.DecodeString(stickerData)
	if err != nil {
		return nil, "", fmt.Errorf("could not decode base64 encoded data from payload")
	}

	filedata, mimeType, err := convertToWebPSticker(dataURL.Data, mimeOverride)
	if err != nil {
		return nil, "", err
	}

	if mimeType == "image/webp" {
		filedata = embedStickerEXIF(filedata, packID, packName, packPublisher, emojis)
	}

	return filedata, mimeType, nil
}

func convertToWebPSticker(data []byte, mimeOverride string) ([]byte, string, error) {
	mimeType := http.DetectContentType(data)
	if mimeOverride != "" {
		mimeType = mimeOverride
	}

	switch {
	case strings.HasPrefix(mimeType, "video/"), mimeType == "image/gif":
		converted, err := convertVideoStickerToWebP(data)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert video/gif sticker to webp: %w", err)
		}
		return converted, "image/webp", nil

	case mimeType == "image/jpeg", mimeType == "image/png", mimeType == "image/jpg":
		converted, err := convertImageToWebP(data)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert image sticker to webp: %w", err)
		}
		return converted, "image/webp", nil

	default:
		return data, mimeType, nil
	}
}

func embedStickerEXIF(inputWebP []byte, packID, packName, packPublisher string, emojis []string) []byte {
	meta := buildStickerMetadata(packID, packName, packPublisher, emojis)
	if meta == nil {
		return inputWebP
	}

	exifData := buildWhatsAppEXIF(meta)
	out, err := injectWebPEXIF(inputWebP, exifData)
	if err != nil {
		log.Warn().Err(err).Msg("failed to inject EXIF chunk; sending sticker without metadata")
		return inputWebP
	}
	return out
}

func injectWebPEXIF(in []byte, exif []byte) ([]byte, error) {
	if !isValidWebP(in) {
		return nil, fmt.Errorf("not a RIFF WEBP file")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(in))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image config: %w", err)
	}

	chunks, vp8xIndex, err := parseWebPChunks(in)
	if err != nil {
		return nil, err
	}

	chunks = ensureVP8XWithEXIF(chunks, vp8xIndex, cfg.Width, cfg.Height)

	return assembleWebP(chunks, exif), nil
}

func parseWebPChunks(in []byte) (chunks [][]byte, vp8xIndex int, err error) {
	vp8xIndex = -1
	pos := riffHeaderSize

	for pos+chunkHeaderSize <= len(in) {
		tag := string(in[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(in[pos+4 : pos+8]))
		dataEnd := pos + chunkHeaderSize + size

		if dataEnd > len(in) {
			return nil, -1, fmt.Errorf("truncated webp chunk: %s", tag)
		}

		pad := size & 1
		if tag == "VP8X" && size >= vp8xPayloadSize {
			vp8xIndex = len(chunks)
		}
		if tag != "EXIF" {
			chunk := make([]byte, chunkHeaderSize+size+pad)
			copy(chunk, in[pos:dataEnd])
			if pad == 1 {
				chunk[chunkHeaderSize+size] = 0
			}
			chunks = append(chunks, chunk)
		}
		pos = dataEnd + pad
	}
	return chunks, vp8xIndex, nil
}

func ensureVP8XWithEXIF(chunks [][]byte, vp8xIndex, width, height int) [][]byte {
	if vp8xIndex >= 0 {
		chunks[vp8xIndex][vp8xFlagsOffset] |= vp8xFlagEXIF
		return chunks
	}
	return append([][]byte{createVP8XChunk(width, height)}, chunks...)
}

func assembleWebP(chunks [][]byte, exif []byte) []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")
	out.Write([]byte{0, 0, 0, 0})
	out.WriteString("WEBP")

	for _, c := range chunks {
		out.Write(c)
	}

	writeChunk(&out, "EXIF", exif)

	b := out.Bytes()
	binary.LittleEndian.PutUint32(b[riffSizeOffset:], uint32(len(b)-8))
	return b
}
