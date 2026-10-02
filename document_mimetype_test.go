package main

import (
	"net/http"
	"testing"
)

func TestResolveDocumentMimeType(t *testing.T) {
	zipData := []byte("PK\x03\x04\x14\x00\x00\x00")

	tests := []struct {
		name             string
		explicitMimeType string
		dataURLMimeType  string
		httpContentType  string
		fileName         string
		fileData         []byte
		want             string
	}{
		{
			name:             "explicit MIME type wins over every other source",
			explicitMimeType: " application/custom ",
			dataURLMimeType:  "application/octet-stream",
			httpContentType:  "application/pdf",
			fileName:         "report.xlsx",
			fileData:         zipData,
			want:             "application/custom",
		},
		{
			name:            "Excel data URL MIME type is preserved",
			dataURLMimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			fileName:        "report.xlsx",
			fileData:        zipData,
			want:            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:            "generic data URL MIME type falls through to filename extension",
			dataURLMimeType: "application/octet-stream",
			fileName:        "report.xlsx",
			fileData:        zipData,
			want:            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:     "docx resolves from filename extension",
			fileName: "report.DOCX",
			fileData: zipData,
			want:     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:     "pptx resolves from filename extension",
			fileName: "slides.pptx",
			fileData: zipData,
			want:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		},
		{
			name:     "PDF resolves from filename extension",
			fileName: "document.pdf",
			fileData: []byte("%PDF-1.7"),
			want:     "application/pdf",
		},
		{
			name:     "unknown extension falls back to detected content type",
			fileName: "document.unknown",
			fileData: []byte("%PDF-1.7"),
			want:     "application/pdf",
		},
		{
			name:     "unknown extension and generic detection fall back safely",
			fileName: "document.unknown",
			fileData: []byte{0x00, 0x01, 0x02, 0x03},
			want:     "application/octet-stream",
		},
		{
			name:             "explicit MIME type parameters are removed",
			explicitMimeType: " text/csv; charset=utf-8 ",
			fileName:         "report.xlsx",
			fileData:         zipData,
			want:             "text/csv",
		},
		{
			name:             "invalid explicit MIME type falls through",
			explicitMimeType: "not-a-media-type",
			fileName:         "report.xlsx",
			fileData:         zipData,
			want:             "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:            "HTTP MIME type parameters are removed",
			httpContentType: " application/pdf; charset=binary ",
			fileName:        "document.unknown",
			fileData:        zipData,
			want:            "application/pdf",
		},
		{
			name:            "generic HTTP MIME type falls through to filename extension",
			httpContentType: "binary/octet-stream",
			fileName:        "report.xlsx",
			fileData:        zipData,
			want:            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDocumentMimeType(
				tt.explicitMimeType,
				tt.dataURLMimeType,
				tt.httpContentType,
				tt.fileName,
				http.DetectContentType(tt.fileData),
			)
			if got != tt.want {
				t.Fatalf("resolveDocumentMimeType() = %q, want %q", got, tt.want)
			}
		})
	}
}
