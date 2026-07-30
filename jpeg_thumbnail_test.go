package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/nfnt/resize"
)

// TestJpegThumbnail covers the in-memory thumbnail encoding that replaced the
// leaking temp-file path in SendImage: it must return decodable JPEG bytes
// bounded by the requested size, preserving aspect ratio.
func TestJpegThumbnail(t *testing.T) {
	// A 200x100 source image (2:1 aspect).
	src := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for x := 0; x < 200; x++ {
		for y := 0; y < 100; y++ {
			src.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}

	out, err := jpegThumbnail(src, 72, 72)
	if err != nil {
		t.Fatalf("jpegThumbnail: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("jpegThumbnail returned no bytes")
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not a decodable image: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("format = %q; want jpeg", format)
	}
	if cfg.Width == 0 || cfg.Height == 0 {
		t.Errorf("thumbnail has a zero dimension: %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width > 72 || cfg.Height > 72 {
		t.Errorf("thumbnail %dx%d exceeds the 72x72 bound", cfg.Width, cfg.Height)
	}
}

// TestJpegThumbnailNil verifies the nil-image guard returns an error instead of
// panicking (resize.Thumbnail dereferences the image's bounds).
func TestJpegThumbnailNil(t *testing.T) {
	if _, err := jpegThumbnail(nil, 72, 72); err == nil {
		t.Error("expected an error for a nil image, got nil")
	}
}

// TestJpegThumbnailRealImage runs the fix end-to-end against a real image file
// shipped with the project. It decodes the image exactly like SendImage does
// (image.Decode), produces the thumbnail, and asserts the output is byte-for-byte
// identical to the previous resize+encode — so removing the temp file changed
// nothing but the mechanism — and is a valid, bounded JPEG.
func TestJpegThumbnailRealImage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("static", "images", "background_image.png"))
	if err != nil {
		t.Skipf("real image not available: %v", err)
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode real image: %v", err)
	}
	t.Logf("decoded a real %s image: %dx%d (%T)", format, img.Bounds().Dx(), img.Bounds().Dy(), img)

	got, err := jpegThumbnail(img, 72, 72)
	if err != nil {
		t.Fatalf("jpegThumbnail on real image: %v", err)
	}

	// Reference: the exact resize + JPEG encode the old temp-file path performed.
	var ref bytes.Buffer
	if err := jpeg.Encode(&ref, resize.Thumbnail(72, 72, img, resize.Lanczos3), nil); err != nil {
		t.Fatalf("reference encode: %v", err)
	}
	if !bytes.Equal(got, ref.Bytes()) {
		t.Errorf("thumbnail differs from the resize+encode reference (%d vs %d bytes)", len(got), ref.Len())
	}

	cfg, f, err := image.DecodeConfig(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("thumbnail is not a decodable image: %v", err)
	}
	t.Logf("thumbnail produced: %s %dx%d, %d bytes", f, cfg.Width, cfg.Height, len(got))
	if f != "jpeg" || cfg.Width == 0 || cfg.Width > 72 || cfg.Height > 72 {
		t.Errorf("unexpected thumbnail: format=%s size=%dx%d", f, cfg.Width, cfg.Height)
	}
}
