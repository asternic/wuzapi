package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func benchmarkMediaFile(b *testing.B, mib int) *mediaFile {
	b.Helper()
	s, err := getMediaStore()
	if err != nil {
		b.Fatal(err)
	}
	file, out, err := s.create("large.bin", "application/octet-stream")
	if err != nil {
		b.Fatal(err)
	}
	file.Size = int64(mib) * 1024 * 1024
	if err = out.Truncate(file.Size); err != nil {
		b.Fatal(err)
	}
	out.Close()
	b.Cleanup(file.Close)
	return file
}
func BenchmarkMediaWebhook(b *testing.B) {
	for _, mib := range []int{1, 25, 100} {
		b.Run(fmt.Sprintf("%dMiB", mib), func(b *testing.B) {
			source := benchmarkMediaFile(b, mib)
			b.Run("baselineJSON", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(source.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// The baseline's fileToBase64 + event JSON step, excluding earlier Download
					// and subsequent Resty copies, makes this a conservative comparison.
					data, err := os.ReadFile(source.Path)
					if err != nil {
						b.Fatal(err)
					}
					payload, err := json.Marshal(map[string]interface{}{"base64": base64.StdEncoding.EncodeToString(data)})
					if err != nil {
						b.Fatal(err)
					}
					if len(payload) == 0 {
						b.Fatal("empty")
					}
				}
			})
			b.Run("fileJSON", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(source.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					file, err := prepareMediaJSON(map[string]interface{}{"base64": source})
					if err != nil {
						b.Fatal(err)
					}
					file.Close()
				}
			})
			event, err := prepareMediaJSON(map[string]interface{}{"base64": source})
			if err != nil {
				b.Fatal(err)
			}
			defer event.Close()
			b.Run("fileForm", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(source.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					file, err := prepareMediaForm(event, "instance", "user")
					if err != nil {
						b.Fatal(err)
					}
					file.Close()
				}
			})
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				buffer := make([]byte, 32*1024)
				n := 0
				for {
					count, err := r.Body.Read(buffer)
					n += count
					if n >= 1024*1024 {
						time.Sleep(time.Millisecond)
						n = 0
					}
					if err != nil {
						break
					}
				}
			}))
			defer receiver.Close()
			b.Run("slowHTTP", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(event.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := postMediaBody(context.Background(), receiver.Client(), receiver.URL, "application/json", "", event); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("twoConcurrent", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(2 * source.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var wg sync.WaitGroup
					for j := 0; j < 2; j++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							file, err := prepareMediaJSON(map[string]interface{}{"base64": source})
							if err != nil {
								b.Error(err)
								return
							}
							defer file.Close()
							if _, err = postMediaBody(context.Background(), receiver.Client(), receiver.URL, "application/json", "", file); err != nil {
								b.Error(err)
							}
						}()
					}
					wg.Wait()
				}
			})
			b.Run("rabbitRequiredBuffer", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(event.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					rabbitMu.Lock()
					data, err := os.ReadFile(event.Path)
					if err == nil {
						_, err = io.Discard.Write(data)
					}
					rabbitMu.Unlock()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func BenchmarkMediaTransfers(b *testing.B) {
	for _, mib := range []int{1, 25, 100} {
		b.Run(fmt.Sprintf("%dMiB", mib), func(b *testing.B) {
			source := benchmarkMediaFile(b, mib)
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/octet-stream")
					file, err := source.Open()
					if err != nil {
						w.WriteHeader(500)
						return
					}
					defer file.Close()
					io.Copy(w, file)
				} else {
					io.Copy(io.Discard, r.Body)
					w.Header().Set("ETag", `"fixture"`)
				}
			}))
			defer endpoint.Close()
			previous := globalHTTPClient
			globalHTTPClient = endpoint.Client()
			defer func() { globalHTTPClient = previous }()
			b.Run("baselineURL", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(source.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, _, err := fetchURLBytes(context.Background(), endpoint.URL, source.Size); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("fileURL", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(source.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					file, err := readOutgoingMedia(context.Background(), endpoint.URL, source.Size)
					if err != nil {
						b.Fatal(err)
					}
					file.Close()
				}
			})
			manager := GetS3Manager()
			config := &S3Config{Enabled: true, Endpoint: endpoint.URL, Region: "us-east-1", Bucket: "fixtures", AccessKey: "key", SecretKey: "secret", PathStyle: true}
			if err := manager.InitializeS3Client("benchmark", config); err != nil {
				b.Fatal(err)
			}
			defer manager.RemoveClient("benchmark")
			b.Run("fileS3", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(source.Size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r, err := source.Open()
					if err != nil {
						b.Fatal(err)
					}
					err = manager.UploadReaderToS3(context.Background(), "benchmark", "fixture", r, source.Size, "application/octet-stream")
					r.Close()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// These costs are intentionally retained by the API contract and image codec.
func BenchmarkMediaResidualJSONDecode(b *testing.B) {
	for _, mib := range []int{1, 25, 100} {
		b.Run(fmt.Sprintf("%dMiB", mib), func(b *testing.B) {
			request := []byte(`{"Document":"data:application/octet-stream;base64,` + strings.Repeat("A", mib*1024*1024*4/3) + `"}`)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var payload struct{ Document string }
				if err := json.NewDecoder(bytes.NewReader(request)).Decode(&payload); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkMediaResidualPixels(b *testing.B) {
	for _, side := range []int{1024, 4096} {
		b.Run(fmt.Sprintf("%dx%d", side, side), func(b *testing.B) {
			var encoded bytes.Buffer
			png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, side, side)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := image.Decode(bytes.NewReader(encoded.Bytes())); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
