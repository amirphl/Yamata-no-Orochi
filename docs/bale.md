# Bale messaging integration

This document describes the Bale client implemented in
[`app/scheduler/bale_client.go`](../app/scheduler/bale_client.go), not a copy of
the provider’s external documentation. The client presents one internal API and
normalizes responses from the current Najva v2 service and legacy Safir v3
service.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `BALE_API_ACCESS_KEY` | empty | Required for every send, upload, and status request. |
| `BALE_PROVIDER` | `najva` | Provider selection: `najva_v2`, `safir_v3`, or `auto`. The aliases `najva`, `v2`, `safir`, `legacy`, and `v1` are accepted. Unknown values normalize to `auto`. |
| `BALE_NAJVA_DOMAIN` | `https://sms.najva.com` | Base URL for Najva endpoints. |
| `BALE_LEGACY_DOMAIN` | `https://safir.bale.ai` | Base URL for Safir endpoints. |

The checked-in `env.template` selects `najva`. Keep provider credentials in
`.env`/`.env.beta`; do not put them in this document or in command arguments.

`auto` tries Najva first and falls back to Safir only when the Najva endpoint is
reported as unsupported. Authentication, validation, rate-limit, server, and
network errors do not trigger a provider switch.

## Provider endpoints

| Operation | Najva v2 | Safir v3 |
|---|---|---|
| Same message to recipients | `POST /v2/sms/send` | One `POST /api/v3/send_message` per recipient |
| Different text per recipient | `POST /v2/sms/send-p2p` | One send request per recipient |
| Delivery status | `POST /v2/sms/status` | Not supported by this client |
| File upload | `POST /upload-file/bale` | `POST /api/v3/upload_file` |

The client supplies the configured key through the legacy and common proxy
header forms (`api-access-key`, `x-api-key`, `apikey`, and `Authorization`). A
missing key fails before an HTTP request is made.

## Internal request model

The scheduler uses this stable structure:

```json
{
  "request_id": "campaign-recipient-id",
  "bot_id": 123456,
  "phone_number": "989121234567",
  "message_data": {
    "message": {
      "text": "Campaign content",
      "file_id": "optional-provider-file-id",
      "copy_text": "optional-legacy-copy-text"
    }
  }
}
```

`message_data.otp_message.otp` is also accepted by the internal type. For
Najva, regular and OTP content are normalized to plain message text. Najva uses
`bot_id` as its positive `sender` value and normalizes recipient phone numbers
before sending.

`request_id` is preserved in normalized responses so scheduler results can be
matched to input rows. The legacy Safir payload also sends it to the provider.

## Batching

`SendBatch` removes entries with empty phone numbers and then chooses the most
specific supported operation:

1. For Najva/auto, two or more requests with the same positive bot ID, text,
   and optional file ID use `/v2/sms/send`.
2. Requests with the same positive bot ID/file ID but different non-empty text
   use `/v2/sms/send-p2p`.
3. Unsupported batch endpoints fall back to one-by-one sends.
4. `safir_v3` always sends one request per recipient.

Najva calls are capped at 9,000 recipients, intentionally below the provider’s
10,000-item boundary. A response is normalized per recipient and includes the
provider message ID or structured error data. Incomplete batch responses create
an `INCOMPLETE_RESPONSE` item instead of silently dropping a recipient.

## Uploads

Najva uploads accept `.jpeg`, `.jpg`, `.png`, `.gif`, `.opus`, `.ogg`, and
`.mp4`. The client enforces a 10 MiB limit, intentionally below the provider
boundary. If the source file has no extension, the client detects a supported
media type and creates a temporary file with the inferred extension.

Safir uploads use multipart form data and return its `file_id` directly. In
`auto`, an unsupported Najva upload endpoint can fall back to Safir.

The scheduler uploads campaign media through this client and then supplies the
returned file ID with message sends. Upload paths must refer to readable local
files in the worker container.

## Status tracking

Status tracking is available only for Najva/auto. Message IDs must parse as
integers. The client splits lookups into chunks of 900 IDs, intentionally below
the provider’s 1,000-item boundary, and calls `/v2/sms/status` for each chunk.

Known status codes are normalized as follows:

| Code | Meaning |
|---:|---|
| 1 | in queue |
| 2 | scheduled |
| 4 | sent to operator |
| 6 | failed to send to recipient |
| 10 | delivered |
| 11 | delivery problem |
| 13 | canceled |
| 14 | recipient blocked |
| 100 | invalid or missing message ID |

If a requested ID is absent from a successful provider response, the client
adds a synthetic status `100` result. Raw response bodies are retained for
scheduler persistence/diagnostics; access to those records should follow the
same sensitivity policy as provider logs.

## Retry behavior

Send and status operations retry retryable provider/network failures with
exponential backoff starting at one second and capped at 15 minutes. The client
has no fixed attempt limit; the caller’s context cancellation/deadline stops
the loop. Validation errors, permanent provider errors, and successful
responses with non-retryable recipient failures return immediately.

This makes context ownership important: worker shutdown and campaign
cancellation must cancel in-flight contexts so a persistent provider outage
does not hold work indefinitely.

## Testing

The client tests use local HTTP test servers and do not require provider
credentials:

```bash
go test ./app/scheduler -run 'Test.*Bale'
```

When changing provider payloads, cover at least single send, same-message batch,
P2P batch, upload validation, status chunking, response normalization,
unsupported-endpoint fallback, retry classification, and context cancellation.
