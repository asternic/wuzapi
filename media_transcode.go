package main

// Outgoing audio/video normalization.
//
// Browsers record voice as WebM/Opus or MP4/AAC and video as WebM/VP8-9.
// WhatsApp only renders voice notes as Ogg/Opus and videos as MP4 (H.264/AAC);
// anything else arrives as a file that does not play (or does not arrive) on
// phones. When FFmpeg is available, /chat/send/audio and /chat/send/video now
// convert such inputs, and voice notes get their duration and waveform so they
// look like native recordings. Inputs that are already compatible are sent
// untouched. Without FFmpeg (or when probing fails) the legacy behavior is kept.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	voiceMime           = "audio/ogg; codecs=opus"
	videoMime           = "video/mp4"
	transcodeDemuxers   = "matroska,webm,ogg,mov,mp4,m4a,3gp,3g2,mj2,wav,mp3,flac,aac,amr,avi,mpeg,mpegts,caf,aiff"
	audioTranscodeLimit = 90 * time.Second
	videoTranscodeLimit = 5 * time.Minute
)

type mediaProbe struct {
	Format   string
	Video    string
	Audio    string
	Duration float64
	Width    int
	Height   int
}

var (
	probeOutgoingMedia     = ffprobeMedia
	transcodeOutgoingMedia = ffmpegTranscode
	voiceWaveform          = ffmpegWaveform
)

func ffprobeMedia(ctx context.Context, path string) (mediaProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file",
		"-show_entries", "format=format_name,duration:stream=codec_type,codec_name,width,height",
		"-of", "json", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	var diagnostics cappedMediaLog
	cmd.Stderr = &diagnostics
	if err := cmd.Run(); err != nil {
		return mediaProbe{}, fmt.Errorf("ffprobe: %w", err)
	}
	var raw struct {
		Format struct {
			Name     string `json:"format_name"`
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		return mediaProbe{}, err
	}
	p := mediaProbe{Format: raw.Format.Name}
	p.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for _, s := range raw.Streams {
		switch {
		case s.Type == "video" && p.Video == "" && s.Codec != "mjpeg" && s.Codec != "png":
			p.Video, p.Width, p.Height = s.Codec, s.Width, s.Height
		case s.Type == "audio" && p.Audio == "":
			p.Audio = s.Codec
		}
	}
	return p, nil
}

func isOggOpus(p mediaProbe) bool {
	return strings.Contains(p.Format, "ogg") && p.Audio == "opus" && p.Video == ""
}

// WhatsApp plays these as regular (non-voice) audio files.
func isWhatsAppAudio(p mediaProbe) bool {
	if p.Video != "" {
		return false
	}
	switch {
	case isOggOpus(p):
		return true
	case p.Format == "mp3" && p.Audio == "mp3":
		return true
	case strings.Contains(p.Format, "mp4") && p.Audio == "aac":
		return true
	case p.Format == "aac" || p.Format == "amr":
		return true
	}
	return false
}

func isWhatsAppVideo(p mediaProbe) bool {
	return strings.Contains(p.Format, "mp4") && p.Video == "h264" && (p.Audio == "" || p.Audio == "aac")
}

func roundSeconds(d float64) uint32 {
	if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return 0
	}
	// Opus/AAC encoders pad a few milliseconds; round instead of ceil.
	return uint32(math.Max(1, math.Round(d)))
}

func ffmpegTranscode(ctx context.Context, src *mediaFile, kind string) (*mediaFile, error) {
	store, err := getMediaStore()
	if err != nil {
		return nil, err
	}
	release, err := store.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	limit, mime, name := audioTranscodeLimit, voiceMime, "audio.ogg"
	args := []string{"-nostdin", "-v", "error", "-y", "-protocol_whitelist", "file", "-format_whitelist", transcodeDemuxers, "-i", src.Path}
	switch kind {
	case "voice", "audio":
		channels, bitrate, application := "1", "32k", "voip"
		if kind == "audio" { // regular audio files keep stereo and more bitrate
			channels, bitrate, application = "2", "64k", "audio"
		}
		args = append(args, "-map", "0:a:0", "-vn", "-ac", channels, "-ar", "48000", "-c:a", "libopus",
			"-b:a", bitrate, "-application", application, "-t", "3600", "-f", "ogg")
	case "video":
		limit, mime, name = videoTranscodeLimit, videoMime, "video.mp4"
		args = append(args, "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "libx264", "-preset", "veryfast", "-crf", "26",
			"-pix_fmt", "yuv420p", "-vf", "scale='min(1280,iw)':-2", "-c:a", "aac", "-b:a", "96k", "-ac", "2",
			"-movflags", "+faststart", "-fs", strconv.FormatInt(fetchVideoMaxBytes, 10), "-f", "mp4")
	default:
		return nil, fmt.Errorf("unsupported transcode kind %q", kind)
	}
	out, file, err := store.create(name, mime)
	if err != nil {
		return nil, err
	}
	file.Close()
	args = append(args, out.Path)
	tctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(tctx, "ffmpeg", args...)
	var diagnostics cappedMediaLog
	cmd.Stderr = &diagnostics
	if err := cmd.Run(); err != nil {
		out.Close()
		return nil, fmt.Errorf("could not convert %s: %v", kind, strings.TrimSpace(diagnostics.String()))
	}
	if err := out.refresh(); err != nil || out.Size == 0 {
		out.Close()
		return nil, errors.New("conversion produced an empty file")
	}
	return out, nil
}

// ffmpegWaveform returns 64 amplitude samples (0-100), the format WhatsApp
// clients use to draw a voice note.
func ffmpegWaveform(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-protocol_whitelist", "file",
		"-i", path, "-t", "600", "-ac", "1", "-ar", "4000", "-f", "s16le", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pcm, readErr := io.ReadAll(io.LimitReader(stdout, 4000*2*600))
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		return nil, waitErr
	}
	return waveformFromPCM(pcm), nil
}

func waveformFromPCM(pcm []byte) []byte {
	const buckets = 64
	samples := len(pcm) / 2
	wave := make([]byte, buckets)
	if samples == 0 {
		return wave
	}
	sums := make([]float64, buckets)
	counts := make([]int, buckets)
	for i := 0; i < samples; i++ {
		v := int16(uint16(pcm[2*i]) | uint16(pcm[2*i+1])<<8)
		b := i * buckets / samples
		sums[b] += math.Abs(float64(v))
		counts[b]++
	}
	peak := 0.0
	for i := range sums {
		if counts[i] > 0 {
			sums[i] /= float64(counts[i])
		}
		peak = math.Max(peak, sums[i])
	}
	if peak == 0 {
		return wave
	}
	for i := range sums {
		wave[i] = byte(math.Round(sums[i] / peak * 100))
	}
	return wave
}

type preparedAudio struct {
	Media     *mediaFile // nil when the original is sent unchanged
	Mime      string
	Seconds   uint32
	Waveform  []byte
	Converted bool
}

// prepareOutgoingAudio converts audio WhatsApp would not play and fills in the
// duration and, for voice notes, the waveform. Errors mean the input could not
// be converted; probe failures fall back to the legacy behavior (zero value).
func prepareOutgoingAudio(ctx context.Context, media *mediaFile, ptt, convert bool) (preparedAudio, error) {
	probe, err := probeOutgoingMedia(ctx, media.Path)
	if err != nil {
		return preparedAudio{}, nil
	}
	result := preparedAudio{Seconds: roundSeconds(probe.Duration)}
	source := media
	if convert && ((ptt && !isOggOpus(probe)) || (!ptt && !isWhatsAppAudio(probe))) {
		if probe.Audio == "" {
			return preparedAudio{}, errors.New("audio has no audio stream")
		}
		kind := "audio"
		if ptt {
			kind = "voice"
		}
		out, err := transcodeOutgoingMedia(ctx, media, kind)
		if err != nil {
			return preparedAudio{}, err
		}
		result.Media, result.Mime, result.Converted, source = out, voiceMime, true, out
		if p, err := probeOutgoingMedia(ctx, out.Path); err == nil && p.Duration > 0 {
			result.Seconds = roundSeconds(p.Duration)
		}
	}
	if ptt {
		if wave, err := voiceWaveform(ctx, source.Path); err == nil {
			result.Waveform = wave
		}
	}
	return result, nil
}

type preparedVideo struct {
	Media         *mediaFile
	Seconds       uint32
	Width, Height uint32
	Converted     bool
}

func prepareOutgoingVideo(ctx context.Context, media *mediaFile, convert bool) (preparedVideo, error) {
	probe, err := probeOutgoingMedia(ctx, media.Path)
	if err != nil {
		return preparedVideo{}, nil
	}
	result := preparedVideo{Seconds: roundSeconds(probe.Duration), Width: uint32(probe.Width), Height: uint32(probe.Height)}
	if !convert || isWhatsAppVideo(probe) {
		return result, nil
	}
	if probe.Video == "" {
		return preparedVideo{}, errors.New("video has no video stream")
	}
	out, err := transcodeOutgoingMedia(ctx, media, "video")
	if err != nil {
		return preparedVideo{}, err
	}
	result.Media, result.Converted = out, true
	if p, err := probeOutgoingMedia(ctx, out.Path); err == nil {
		result.Seconds, result.Width, result.Height = roundSeconds(p.Duration), uint32(p.Width), uint32(p.Height)
	}
	return result, nil
}

// ffmpegVideoThumbnail grabs an early frame so videos sent without a
// JPEGThumbnail do not show a black preview.
func ffmpegVideoThumbnail(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-protocol_whitelist", "file",
		"-i", path, "-frames:v", "1", "-vf", "thumbnail=25,scale=320:-2", "-f", "image2pipe", "-c:v", "mjpeg", "-")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(&out)
	if err != nil {
		return nil, err
	}
	return jpegThumbnail(img, 72, 72)
}

var videoThumbnail = ffmpegVideoThumbnail
