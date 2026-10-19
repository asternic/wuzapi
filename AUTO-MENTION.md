# Automatic mentions

Write a standalone `@phone` token in a message body or supported media caption to
add a WhatsApp mention automatically. Use the phone number in international
format without the `+` prefix. No additional mention field is required.

The message text or caption stays unchanged. The detected number is added to the
message's mention metadata as `phone@s.whatsapp.net`.

## Text messages

Send a request to `POST /chat/send/text`:

```json
{
  "Phone": "120363407915251919@g.us",
  "Body": "Hello @6281234567890, please read this."
}
```

Multiple mentions are supported. Repeated mentions of the same number produce
only one metadata entry:

```json
{
  "Phone": "120363407915251919@g.us",
  "Body": "Hello @6281234567890 and @6289876543210."
}
```

## Supported fields and captions

| Endpoint | Field scanned for automatic mentions |
|---|---|
| `POST /chat/send/text` | `Body` |
| `POST /chat/send/edit` | `Body` |
| `POST /chat/send/image` | `Caption` |
| `POST /chat/send/video` | `Caption` |
| `POST /chat/send/document` | `Caption` |

For image, video, and document requests, put `@phone` tokens in the `Caption`
field alongside the endpoint's required media fields.

Audio messages do not include the request's `Caption` in the outgoing message.
Consequently, `POST /chat/send/audio` does not scan captions for automatic
mentions. It continues to accept explicitly supplied `ContextInfo.MentionedJID`
values.

## Detection rules

Mentions must be standalone tokens. Spaces, line breaks, parentheses, quotes,
commas, and sentence-ending punctuation can surround a mention. For example,
`Hello (@6281234567890)!` and `Hello @6281234567890.` are supported.

Email addresses, URLs, and longer tokens such as `@6281234567890abc` or
`@6281234567890_name` are ignored.

## Explicit mentions

For the endpoints with automatic detection, explicitly supplied
`ContextInfo.MentionedJID` entries are combined with detected mentions and
deduplicated. Explicit entries are preserved even when they do not appear in the
message body or caption.

```json
{
  "Phone": "120363407915251919@g.us",
  "Body": "Hello @6281234567890.",
  "ContextInfo": {
    "MentionedJID": ["6289876543210@s.whatsapp.net"]
  }
}
```

This request includes both numbers in the mention metadata.

## Mention all group members

Set `"MentionAll": true` to mention all current participants without adding visible
phone numbers. A standalone `@everyone` in the body or caption also enables this;
the visible text is preserved. Supported on text, edit, image, video, and document
endpoints. Direct chats are rejected. Participant lookup failures abort the send.
Explicit mentions and group participants are combined and deduplicated.

```json
{"Phone":"120363407915251919@g.us","Body":"Please read this.","MentionAll":true}
```
