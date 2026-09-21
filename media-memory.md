# Attachment memory and validation

The event-media pipeline uses Whatsmeow `DownloadToFile`, seekable S3 uploads,
streamed base64 JSON/form preparation, and replayable HTTP bodies. Outgoing
attachments use file-backed data-URL decoding/URL fetching, `UploadReader` with
separate encryption scratch, and file-based sticker conversion and EXIF updates.
No Whatsmeow dependency fork or PR #356 code is used.

## Configuration and lifecycle

- `WUZAPI_MEDIA_TMPDIR`: defaults to `os.TempDir()`. Choose a disk-backed mount;
  tmpfs still counts toward container memory. The directory must be writable and
  support advisory locks. Docker Compose forwards this variable; mount any custom
  storage at the configured container path.
- `WUZAPI_MEDIA_CONCURRENCY`: positive integer, default `2`. Download, decode,
  upload, conversion, and request-body preparation wait for capacity. Cancellation
  interrupts waiting. Incoming download timeouts start after acquiring capacity.
  HTTP retry backoff does not hold a permit.
- Internal filenames are unique and files have mode `0600` inside process-owned
  `0700` directories. External names and MIME-derived extension selection stay
  unchanged. The ownership lock remains held while the process is alive; a root
  lock serializes startup/reclamation. Another instance cannot reclaim live files.
- Event handlers own incoming files until delivery bodies have been prepared.
  Delivery workers own those bodies until user/global webhooks and RabbitMQ
  finish. Each consumer opens its own cursor. HTTP retries and 307/308 redirects
  reopen the same bytes with `Content-Length` and `GetBody`; HMAC hashes that file.
- Shutdown cancels media operations and waits within the existing shutdown grace
  period. Work that cannot finish before forced exit is reclaimed on the next
  start. Files are temporary storage, not a durable delivery queue.

## Measurements

Recorded on Apple M2, macOS arm64, Go 1.26.0. Values below are **total Go heap
bytes allocated per operation**, not peak RSS or disk/page-cache usage. Each
benchmark used one measured iteration, so small differences include cold caches
and HTTP setup; timing is not a production throughput guarantee.

| Path | 1 MiB attachment | 25 MiB attachment | 100 MiB attachment |
|---|---:|---:|---:|
| Baseline file read + base64 + JSON | 8,064,192 | 201,002,376 | 803,933,480 |
| File-backed JSON preparation | 69,832 | 69,592 | 69,528 |
| File-backed form preparation | 80,480 | 80,480 | 80,480 |
| Prepared HTTP body, slow receiver | 127,024 | 123,432 | 116,600 |
| Two simultaneous JSON preparations + HTTP sends, combined | 330,160 | 334,456 | 302,600 |
| RabbitMQ required final JSON buffer | 1,401,256 | 34,955,688 | 139,813,288 |
| Baseline URL download | 2,305,704 | 55,449,696 | 252,291,904 |
| File-backed URL download | 48,896 | 89,192 | 109,384 |
| Seekable S3 upload | 171,120 | 205,536 | 201,232 |

The JSON baseline is conservative: it excludes the old initial Whatsmeow download
buffer and subsequent Resty copies. Slow receivers sleep during consumption;
concurrent measurements send two attachments. URL and S3 measurements use local
HTTP fixtures. WhatsApp download/decryption itself was not benchmarked against a
live account. RabbitMQ measurements isolate its required allocation under the
shared publisher lock; they do not measure a live broker.

Reproduce:

```sh
go test -run '^$' -bench 'BenchmarkMedia(Webhook|Transfers|Residual)' -benchtime=1x -benchmem ./...
go test -race ./...
go vet ./...
WUZAPI_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 dbname=postgres user=... sslmode=disable' go test -race ./...
```

## Remaining size-dependent costs

The existing JSON decoder still buffers request JSON and materializes the encoded
attachment string. Measured decoder allocation was 5,594,976 / 169,171,536 /
676,682,208 bytes for 1 / 25 / 100 MiB-equivalent base64 requests. Streaming the
subsequent decode avoids another plaintext byte slice but cannot eliminate that
original request cost without changing JSON ingestion.

Image thumbnails still decode pixels: measured PNG decoding allocated about
4.25 MB at 1024×1024 and 67.2 MB at 4096×4096. RabbitMQ's API requires a `[]byte`:
only the publisher holding the shared channel lock reads its final JSON file into
memory. Queued publishers retain files. Existing non-2xx webhook response text is
still retained for the error-queue contract, so an unusually large receiver error
response also consumes memory. Link-preview processing and the explicit media
Download API response builders are unchanged by this event/upload patch.

Disk use includes plaintext, encryption/conversion scratch, the original event
JSON, enhanced delivery JSON, and any form/error envelopes. Base64 expands media
by roughly 4/3; form escaping can add more. Slow receivers and retries retain
prepared files until completion. Budget disk accordingly.

## Compatibility coverage and limits

Tests cover JSON/form bytes and field placement, global/user delivery, HMAC over
sent bytes, invalid keys, application retries, 307/308 redirects, configured proxy
routing, URL SSRF protection, cancellation, preparation failure cleanup, independent
readers, live/stale directory ownership, shutdown, serialized stdout lines, S3
base64/s3/both modes, S3 size/headers/retention/metadata, PNG/JPEG thumbnails and sticker conversions, animated GIF sticker conversion,
MP3/OGG MIME values, data URLs, URL size limits, endpoint invalid inputs, optional
button-image behavior, encryption scratch ownership, and WebP EXIF byte parity.
The existing RabbitMQ synchronization tests also exercise the file publisher when
disabled during connection replacement.

The full suite passes with race detection on SQLite and PostgreSQL; `go vet`
passes. No live WhatsApp session or live RabbitMQ broker was exercised. Actual
WhatsApp encrypted downloads, server uploads, final message delivery, and receiver
rendering still need a live smoke test. Sticker conversion parity passed for PNG, JPEG, and animated GIF using a temporary
FFmpeg 8.1.1 build with `libwebp` under `/tmp/ffmpeg-8.1.1`. The system FFmpeg was
not changed. Tests skip conversion parity when the selected FFmpeg lacks
`libwebp`; select a suitable build through `PATH` to run them. Existing WebP EXIF
processing is tested without FFmpeg. Production still requires a suitable FFmpeg
installation, as before. Linux amd64 builds and Windows amd64 test compilation
also passed; the Windows lock implementation was compiled, not run on Windows.
