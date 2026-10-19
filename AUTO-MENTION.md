# Auto-Mention untuk Wuzapi (fork)

Fitur tambahan di fork ini: **auto-detect `@nomor` di teks pesan** → otomatis jadi mention/tag WhatsApp.
Terinspirasi dari [aldinokemal/go-whatsapp-web-multidevice](https://github.com/aldinokemal/go-whatsapp-web-multidevice).

## Cara pakai

Cukup tulis `@nomor` (format internasional tanpa `+`) di teks/caption. Tidak perlu field tambahan.

```json
POST /chat/send/text
{
  "Phone": "120363407915251919@g.us",
  "Body": "Halo @6281234567890, tolong dibaca ya"
}
```

Beberapa nomor sekaligus juga bisa:

```json
{
  "Phone": "120363407915251919@g.us",
  "Body": "Halo @6281234567890 dan @6281234567890"
}
```

## Endpoint yang didukung

| Endpoint | Teks yang dipindai |
|---|---|
| `POST /chat/send/text` | `Body` |
| `POST /chat/send/image` | `Caption` |
| `POST /chat/send/video` | `Caption` |
| `POST /chat/send/audio` | `Caption` |
| `POST /chat/send/document` | `Caption` |

## Kompatibilitas

Cara lama tetap jalan: `ContextInfo.MentionedJID` yang diisi manual digabung
(dedupe) dengan hasil auto-detect. Jadi tidak ada breaking change.

## Perubahan kode

- `helpers.go`: fungsi `extractMentionJIDs(text)` (regex `@(\d+)` → `nomor@s.whatsapp.net`)
  dan `mergeMentionJIDs(explicit, text)` (gabung + dedupe).
- `handlers.go`: 6 titik `MentionedJID` (text ×2, image, video, audio, document)
  sekarang memanggil `mergeMentionJIDs` dengan teks pesan sebagai sumber auto-detect.

## Build

```sh
go build -o wuzapi .
```

## Catatan

Base: [asternic/wuzapi](https://github.com/asternic/wuzapi) commit `a33e10f`
(Merge PR #387). Lisensi tetap MIT mengikuti upstream.
