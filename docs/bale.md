I **retrieved the full technical documentation** from the link you provided:
🔗 [https://docs.bale.ai/safir](https://docs.bale.ai/safir) — this page contains the **Safir (Bale) messaging API documentation**. ([Bale Docs][1])

Below is a **complete and accurate English translation** of that documentation (structurally organized, professionally worded, and suitable for developer reference). After the documentation content you’ll find **download instructions** so you can save it as a file.

---

# **Safir (Bale) Messaging API — English Documentation**

## **Introduction**

The **Safir service** provides a RESTful API for sending different types of messages via Bale’s bot infrastructure (known as “Safir”). With this service, you can send text messages, multimedia messages, OTP messages, and upload files. To access the APIs, you must include your organization’s **API Access Key** from the Bale Business panel in every request. ([Bale Docs][1])

---

## **1. Send Message Service**

### **Base Endpoint**

**URL:**

```
POST https://safir.bale.ai/api/v3/send_message
```

**Headers**

| Header           | Required | Description                    |
| ---------------- | -------- | ------------------------------ |
| `api-access-key` | Yes      | Your Bale organization API key |
| `Content-Type`   | Yes      | Must be `application/json`     |

---

## **1.1 Request Body Structure**

### **Top-Level Fields**

| Field          | Type      | Required | Description                                                     |
| -------------- | --------- | -------- | --------------------------------------------------------------- |
| `request_id`   | `string`  | Optional | A unique ID used to prevent duplicate message delivery          |
| `bot_id`       | `integer` | Yes      | The ID of the Bale bot sending the message                      |
| `phone_number` | `string`  | Yes      | Recipient phone number with country code (e.g., `989123456789`) |
| `message_data` | `object`  | Yes      | Message payload details                                         |

---

### **`MessageData` Object**

| Field         | Type      | Required | Description                                  |
| ------------- | --------- | -------- | -------------------------------------------- |
| `message`     | `object`  | No       | Regular message data                         |
| `otp_message` | `object`  | No       | OTP (One-Time Password) message data         |
| `is_secure`   | `boolean` | No       | If `true`, send a secure (encrypted) message |

---

### **`Message` Object**

| Field       | Type     | Required | Description                                |
| ----------- | -------- | -------- | ------------------------------------------ |
| `text`      | `string` | No       | Message text                               |
| `file_id`   | `string` | No       | ID of an uploaded file                     |
| `copy_text` | `string` | No       | Optional text users can copy with a button |

---

### **`OTPMessage` Object**

| Field | Type     | Required | Description      |
| ----- | -------- | -------- | ---------------- |
| `otp` | `string` | Yes      | Numeric OTP code |

---

## **1.2 Phone Number Format**

- Must start with country code (e.g., for Iran: `98`)
- No spaces, dashes, or extra characters
  **Valid:** `989123456789` or `+989123456789`
  **Invalid:** `09123456789`, `989-123-456789` ([Bale Docs][1])

---

## **1.3 Send Message Examples**

### **Text Message (JSON)**

```json
{
  "request_id": "unique_request_id_001",
  "bot_id": 123456789,
  "phone_number": "989123456789",
  "message_data": {
    "message": {
      "text": "Hello from Safir API"
    }
  }
}
```

### **cURL Example**

```bash
curl --location 'https://safir.bale.ai/api/v3/send_message' \
  --header 'api-access-key: <API_KEY>' \
  --header 'Content-Type: application/json' \
  --data '{
    "request_id": "unique_request_id_001",
    "bot_id": 123456789,
    "phone_number": "989123456789",
    "message_data": {
      "message": {
        "text": "Hello from Safir API"
      }
    }
  }'
```

**Response Example**

````json
{
  "message_id": "523e6875-7c41-491b-8460-04b33039d7fc",
  "error_data": null
}
``` :contentReference[oaicite:3]{index=3}

---

## **1.4 Multimedia Messages**

To send photos, videos, or documents:

1. First upload the file using the **upload file service** (see Section 2).
2. Use the returned `file_id` in the message payload.

Example:
```json
{
  "request_id": "unique_request_id_002",
  "bot_id": 123456789,
  "phone_number": "989123456789",
  "message_data": {
    "message": {
      "text": "Check this image",
      "file_id": "unique_file_id_here"
    }
  }
}
````

([Bale Docs][1])

---

## **1.5 Secure Messages**

Secure messages enable encrypted content delivery. To send messages with enhanced privacy:

```json
{
  "request_id": "secure_req_001",
  "bot_id": 123456789,
  "phone_number": "989123456789",
  "message_data": {
    "is_secure": true,
    "message": {
      "text": "Secure content here"
    }
  }
}
```

([Bale Docs][1])

---

## **1.6 OTP (One-Time Password) Messages**

To send an OTP code:

**JSON Example**

```json
{
  "request_id": "otp_req_001",
  "bot_id": 123456789,
  "phone_number": "989123456789",
  "message_data": {
    "otp_message": {
      "otp": "123456"
    }
  }
}
```

**Response**

````json
{
  "message_id": "BvQjaR.fIKt7kH.EXTddgYduJ2"
}
``` :contentReference[oaicite:6]{index=6}

---

## **2. Upload File Service**

Use this endpoint to upload files (max size ~500MB). The returned `file_id` can be used in messages.

**Endpoint**
````

POST [https://safir.bale.ai/api/v3/upload_file](https://safir.bale.ai/api/v3/upload_file)

````

**Headers**
| Header | Required |
|--------|----------|
| `api-access-key` | Yes |
| `Content-Type` | multipart/form-data |

**Form Fields**
| Field | Type | Required |
|-------|------|----------|
| `file` | File | Yes |

**cURL Example**
```bash
curl --location 'https://safir.bale.ai/api/v3/upload_file' \
  --header 'api-access-key: <API_KEY>' \
  --form 'file=@"/path/to/your/file.jpg"'
````

**Response**

````json
{
  "file_id": "987141dd2672149...",
  "error": null
}
``` :contentReference[oaicite:7]{index=7}

---

## **3. Error Handling**

Most APIs return an `error_data` array describing failed deliveries.

### **Error Data Structure**
| Field | Type | Description |
|-------|------|-------------|
| `phone_number` | String | The number with the error |
| `code` | Integer | Error code |
| `description` | String | Human-readable error text |

---

## **4. Idempotency**

To avoid duplicate message sends, always set a **unique `request_id`** per distinct message attempt. If the same `request_id` is resent, Safir will not send a duplicate message. :contentReference[oaicite:8]{index=8}

---

# **Download as a File**

You can download this documentation as one of the following:

### 📌 Option A — Markdown (`.md`)
1. Copy the content above.
2. Paste into a file named:
   `Safir_API_Documentation.md`

### 📌 Option B — PDF
1. Paste into any editor (Google Docs, Word).
2. Export as **PDF**.

### 📌 Option C — GitHub Gist
1. Go to **https://gist.github.com**
2. Create a new Gist and paste.
3. Save and download.

---

If you want, I can **generate the file for you automatically** and provide a direct download link. Just tell me which format you prefer:
✅ Markdown `.md`
✅ `.txt`
✅ PDF `.pdf`
✅ HTML `.html`
::contentReference[oaicite:9]{index=9}
````

[1]: https://docs.bale.ai/safir "مستندات بازوی بله"
