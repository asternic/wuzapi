package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

const chatwootAttachmentRedirectLimit = 3

type chatwootAttachment struct {
	DataURL  string `json:"data_url"`
	FileType string `json:"file_type"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
}

func validateChatwootAttachmentURL(value string, allowedHosts map[string]struct{}) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("Chatwoot attachment URL must use HTTPS")
	}
	if _, ok := allowedHosts[strings.ToLower(parsed.Hostname())]; !ok {
		return nil, errors.New("Chatwoot attachment URL host is not allowlisted")
	}
	return parsed, nil
}

func chatwootAttachmentLimit(fileType string) int64 {
	switch fileType {
	case "image", "sticker":
		return fetchImageMaxBytes
	case "audio":
		return fetchAudioMaxBytes
	case "video":
		return fetchVideoMaxBytes
	default:
		return fetchDocumentMaxBytes
	}
}

func chatwootAttachmentHostSet(config chatwootConfigRecord) map[string]struct{} {
	result := make(map[string]struct{})
	for _, host := range chatwootAttachmentHosts(config) {
		result[strings.ToLower(host)] = struct{}{}
	}
	return result
}

func downloadChatwootAttachment(ctx context.Context, attachment chatwootAttachment, config chatwootConfigRecord) (*mediaFile, error) {
	allowedHosts := chatwootAttachmentHostSet(config)
	currentURL, err := validateChatwootAttachmentURL(attachment.DataURL, allowedHosts)
	if err != nil {
		return nil, err
	}
	client := newChatwootAttachmentHTTPClient(allowedHosts)
	limit := chatwootAttachmentLimit(attachment.FileType)
	for redirects := 0; redirects <= chatwootAttachmentRedirectLimit; redirects++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL.String(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
			location, locationErr := resp.Location()
			resp.Body.Close()
			if locationErr != nil {
				return nil, locationErr
			}
			currentURL = currentURL.ResolveReference(location)
			if _, err := validateChatwootAttachmentURL(currentURL.String(), allowedHosts); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			resp.Body.Close()
			return nil, fmt.Errorf("Chatwoot attachment returned HTTP %d", resp.StatusCode)
		}
		store, _ := getMediaStore()
		release, err := store.acquire(ctx)
		if err != nil {
			resp.Body.Close()
			return nil, err
		}
		name := filepath.Base(attachment.FileName)
		if name == "." || name == "" {
			name = "chatwoot-attachment"
		}
		media, file, err := store.create(name, strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
		if err != nil {
			release()
			resp.Body.Close()
			return nil, err
		}
		media.Size, err = io.Copy(file, io.LimitReader(resp.Body, limit+1))
		release()
		closeErr := file.Close()
		resp.Body.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil && media.Size > limit {
			err = fmt.Errorf("Chatwoot attachment exceeds the allowed size")
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
	return nil, errors.New("Chatwoot attachment redirect limit exceeded")
}

func newChatwootAttachmentHTTPClient(allowedHosts map[string]struct{}) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if _, ok := allowedHosts[strings.ToLower(host)]; !ok {
			return nil, errors.New("attachment request attempted an unexpected host")
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, err
		}
		allowInternal := chatwootInternalHostAllowed(host)
		for _, ip := range ips {
			if isPrivateOrLoopback(ip) && !allowInternal {
				continue
			}
			connection, dialErr := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return connection, nil
			}
		}
		return nil, errors.New("attachment host has no allowed reachable address")
	}
	return &http.Client{Timeout: chatwootRequestTimeout, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}
