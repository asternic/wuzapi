package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
}

// generated builds a fixture with FFmpeg's test sources (no external files).
func generated(t *testing.T, name string, args ...string) *mediaFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("ffmpeg", append(append([]string{"-nostdin", "-v", "error", "-y"}, args...), path)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot build %s fixture: %v %s", name, err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return testMedia(t, data)
}

func TestPrepareAudioConvertsBrowserRecording(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()
	// Chrome MediaRecorder: WebM with Opus.
	webm := generated(t, "rec.webm", "-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-c:a", "libopus", "-f", "webm")
	got, err := prepareOutgoingAudio(ctx, webm, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Media == nil || !got.Converted || got.Mime != voiceMime {
		t.Fatalf("webm voice note not converted: %+v", got)
	}
	defer got.Media.Close()
	p, err := probeOutgoingMedia(ctx, got.Media.Path)
	if err != nil || !isOggOpus(p) {
		t.Fatalf("output is not ogg/opus: %+v %v", p, err)
	}
	if got.Seconds != 3 {
		t.Fatalf("seconds = %d, want 3", got.Seconds)
	}
	if len(got.Waveform) != 64 || maxByte(got.Waveform) != 100 {
		t.Fatalf("waveform not filled: %v", got.Waveform)
	}

	// Safari MediaRecorder: MP4 with AAC.
	m4a := generated(t, "rec.m4a", "-f", "lavfi", "-i", "sine=frequency=300:duration=2", "-c:a", "aac", "-f", "mp4")
	got, err = prepareOutgoingAudio(ctx, m4a, true, true)
	if err != nil || !got.Converted {
		t.Fatalf("mp4/aac voice note not converted: %+v %v", got, err)
	}
	got.Media.Close()
}

func TestPrepareAudioKeepsCompatibleInput(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()
	ogg := generated(t, "voice.ogg", "-f", "lavfi", "-i", "sine=duration=2", "-c:a", "libopus", "-f", "ogg")
	got, err := prepareOutgoingAudio(ctx, ogg, true, true)
	if err != nil || got.Media != nil || got.Converted {
		t.Fatalf("ogg/opus voice note must be sent unchanged: %+v %v", got, err)
	}
	if got.Seconds != 2 || len(got.Waveform) != 64 {
		t.Fatalf("duration/waveform missing for unchanged voice note: %+v", got)
	}
	mp3 := generated(t, "song.mp3", "-f", "lavfi", "-i", "sine=duration=1", "-c:a", "libmp3lame", "-f", "mp3")
	got, err = prepareOutgoingAudio(ctx, mp3, false, true)
	if err != nil || got.Media != nil || got.Waveform != nil {
		t.Fatalf("mp3 file audio must be sent unchanged: %+v %v", got, err)
	}
	webm := generated(t, "rec.webm", "-f", "lavfi", "-i", "sine=duration=1", "-c:a", "libopus", "-f", "webm")
	got, err = prepareOutgoingAudio(ctx, webm, true, false)
	if err != nil || got.Media != nil {
		t.Fatalf("convert=false must keep the original: %+v %v", got, err)
	}
}

func TestPrepareAudioRejectsNonAudio(t *testing.T) {
	requireFFmpeg(t)
	video := generated(t, "silent.webm", "-f", "lavfi", "-i", "testsrc=duration=1:size=64x64", "-c:v", "libvpx", "-f", "webm")
	if _, err := prepareOutgoingAudio(context.Background(), video, true, true); err == nil {
		t.Fatal("video without audio accepted as voice note")
	}
	// Garbage keeps the legacy behavior (sent as is, no error).
	junk := testMedia(t, []byte("not media at all"))
	if got, err := prepareOutgoingAudio(context.Background(), junk, true, true); err != nil || got.Media != nil {
		t.Fatalf("unprobeable input must fall back to legacy: %+v %v", got, err)
	}
}

func TestPrepareVideo(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()
	webm := generated(t, "clip.webm", "-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=duration=2", "-shortest", "-c:v", "libvpx", "-c:a", "libopus", "-f", "webm")
	got, err := prepareOutgoingVideo(ctx, webm, true)
	if err != nil || got.Media == nil || !got.Converted {
		t.Fatalf("webm video not converted: %+v %v", got, err)
	}
	defer got.Media.Close()
	p, err := probeOutgoingMedia(ctx, got.Media.Path)
	if err != nil || !isWhatsAppVideo(p) {
		t.Fatalf("output is not mp4/h264/aac: %+v %v", p, err)
	}
	if got.Seconds != 2 || got.Width != 320 || got.Height != 240 {
		t.Fatalf("metadata = %+v", got)
	}
	if thumb, err := videoThumbnail(ctx, got.Media.Path); err != nil || len(thumb) < 100 {
		t.Fatalf("thumbnail: %d bytes, %v", len(thumb), err)
	}

	mp4 := generated(t, "clip.mp4", "-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=15",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-f", "mp4")
	got, err = prepareOutgoingVideo(ctx, mp4, true)
	if err != nil || got.Media != nil || got.Seconds != 1 {
		t.Fatalf("h264 mp4 must be sent unchanged with metadata: %+v %v", got, err)
	}
}

func TestWaveformFromPCM(t *testing.T) {
	if w := waveformFromPCM(nil); len(w) != 64 || maxByte(w) != 0 {
		t.Fatalf("empty input: %v", w)
	}
	pcm := make([]byte, 64*2*10)
	for i := 32 * 10; i < 64*10; i++ { // second half loud
		pcm[2*i], pcm[2*i+1] = 0x00, 0x40
	}
	w := waveformFromPCM(pcm)
	if w[0] != 0 || w[63] != 100 {
		t.Fatalf("waveform shape: %v", w)
	}
}

func maxByte(b []byte) byte {
	var m byte
	for _, v := range b {
		if v > m {
			m = v
		}
	}
	return m
}
