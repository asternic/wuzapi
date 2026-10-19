package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
)

// Office and OpenDocument files are ZIP or OLE containers, so sniffing reports
// application/zip or application/octet-stream. Listed here so the lookup does
// not depend on the host MIME database, which slim images do not ship.
var documentExtensionMimeTypes = map[string]string{
	".csv":  "text/csv",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".odp":  "application/vnd.oasis.opendocument.presentation",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".odt":  "application/vnd.oasis.opendocument.text",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// documentMimeType returns the declared MIME type unless it is missing or
// generic, then the type for the file name extension, then the sniffed type.
func documentMimeType(declared, fileName, sniffed string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	byExtension, ok := documentExtensionMimeTypes[ext]
	if !ok {
		byExtension = mime.TypeByExtension(ext)
	}
	for _, candidate := range []string{declared, byExtension, sniffed} {
		mediaType, _, err := mime.ParseMediaType(candidate)
		if err == nil && mediaType != "application/octet-stream" && mediaType != "binary/octet-stream" {
			return mediaType
		}
	}
	return "application/octet-stream"
}

// Decode the small data-URL header with the existing parser, and its payload
// through a reader. Legacy JSON decoding still owns the original encoded string.
func dataURLReader(value string) (io.Reader, string, error) {
	quoted, escaped := false, false
	comma := -1
	for i := 0; i < len(value); i++ {
		c := value[i]
		if escaped {
			escaped = false
			continue
		}
		if quoted && c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			quoted = !quoted
		}
		if c == ',' && !quoted {
			comma = i
			break
		}
	}
	if comma < 0 {
		return nil, "", fmt.Errorf("missing comma before data")
	}
	header, err := dataurl.DecodeString(value[:comma+1])
	if err != nil {
		return nil, "", err
	}
	payload := value[comma+1:]
	if header.Encoding == dataurl.EncodingBase64 {
		// The legacy parser accepts LF but rejects CR, spaces and URL-escaped base64.
		for i := 0; i < len(payload); i++ {
			c := payload[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '/' || c == '=' || c == '\n') {
				return nil, "", fmt.Errorf("invalid data character")
			}
		}
		return base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload)), header.ContentType(), nil
	}
	return &dataURLASCIIReader{value: payload}, header.ContentType(), nil
}

type dataURLASCIIReader struct {
	value string
	pos   int
}

func (r *dataURLASCIIReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) && r.pos < len(r.value) {
		c := r.value[r.pos]
		r.pos++
		if c < 32 || c >= 127 || strings.ContainsRune(" <>#\"{}|\\^[]`", rune(c)) {
			return n, fmt.Errorf("invalid data character")
		}
		if c == '%' {
			if r.pos+2 > len(r.value) {
				return n, fmt.Errorf("incomplete escape")
			}
			hi, okHi := mediaHexDigit(r.value[r.pos])
			lo, okLo := mediaHexDigit(r.value[r.pos+1])
			if !okHi || !okLo {
				return n, fmt.Errorf("invalid escape")
			}
			c = hi<<4 | lo
			r.pos += 2
		}
		p[n] = c
		n++
	}
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func readOutgoingMedia(parent context.Context, value string, limit int64) (*mediaFile, error) {
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
	var reader io.Reader
	var mimeType, declaredMIME string
	if isHTTPURL(value) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "WhatsApp/2.23.20.0")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
		resp, err := globalHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}
		reader = io.LimitReader(resp.Body, limit+1)
		mimeType = resp.Header.Get("Content-Type")
		declaredMIME = mimeType
	} else {
		reader, mimeType, err = dataURLReader(value)
		if err != nil {
			return nil, err
		}
		// The parser supplies text/plain for data:;base64,... and data:,... .
		// Keep that legacy MIME for other consumers, but do not treat the
		// implicit default as a declaration when resolving document types.
		if strings.IndexAny(strings.TrimPrefix(value, "data:"), ";,") > 0 {
			declaredMIME = mimeType
		}
	}
	media, file, err := store.create("", mimeType)
	if err != nil {
		return nil, err
	}
	media.DeclaredMIME = declaredMIME
	media.Size, err = io.Copy(file, contextReader{ctx, reader})
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil && isHTTPURL(value) && media.Size > limit {
		err = fmt.Errorf("response exceeds allowed size (%d bytes)", limit)
	}
	if err == nil && media.MIME == "" {
		media.MIME, err = media.sniff()
	}
	if err != nil {
		media.Close()
		return nil, err
	}
	return media, nil
}

type mediaUploader interface {
	UploadReader(context.Context, io.Reader, io.ReadWriteSeeker, whatsmeow.MediaType) (whatsmeow.UploadResponse, error)
}

func uploadMedia(parent context.Context, client mediaUploader, media *mediaFile, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	ctx, cancel, err := mediaContext(parent)
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	defer cancel()
	store, _ := getMediaStore()
	release, err := store.acquire(ctx)
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	defer release()
	r, err := media.Open()
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	defer r.Close()
	scratch, out, err := store.create("", "")
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	defer scratch.Close()
	defer out.Close()
	return client.UploadReader(ctx, contextReader{ctx, r}, out, kind)
}
func mediaThumbnail(parent context.Context, media *mediaFile) ([]byte, error) {
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
	r, err := media.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(contextReader{ctx, r})
	if err != nil {
		return nil, fmt.Errorf("could not decode image for thumbnail preparation: %w", err)
	}
	thumb, err := jpegThumbnail(img, 72, 72)
	if err != nil {
		return nil, fmt.Errorf("Failed to encode jpeg thumbnail: %w", err)
	}
	return thumb, nil
}

func mediaHexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
