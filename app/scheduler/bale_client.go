package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
)

const (
	defaultBaleAPIDomain = "https://safir.bale.ai"
)

type BaleSendMessageRequest struct {
	RequestID   string              `json:"request_id,omitempty"`
	BotID       int64               `json:"bot_id"`
	PhoneNumber string              `json:"phone_number"`
	MessageData BaleSendMessageData `json:"message_data"`
}

type BaleSendMessageData struct {
	Message  *BaleMessage `json:"message,omitempty"`
	OTP      *BaleOTP     `json:"otp_message,omitempty"`
	IsSecure *bool        `json:"is_secure,omitempty"`
}

type BaleMessage struct {
	Text     string  `json:"text,omitempty"`
	FileID   *string `json:"file_id,omitempty"`
	CopyText *string `json:"copy_text,omitempty"`
}

type BaleOTP struct {
	OTP string `json:"otp"`
}

type BaleErrorData struct {
	PhoneNumber string `json:"phone_number,omitempty"`
	CodeRaw     any    `json:"code,omitempty"`
	Description string `json:"description,omitempty"`
}

func (e BaleErrorData) CodeString() string {
	switch v := e.CodeRaw.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

type BaleSendMessageResponse struct {
	MessageID string          `json:"message_id"`
	ErrorData []BaleErrorData `json:"error_data"`
}

type BaleUploadFileResponse struct {
	FileID string `json:"file_id"`
	Error  any    `json:"error"`
}

type BaleClient interface {
	SendMessage(ctx context.Context, req *BaleSendMessageRequest) (*BaleSendMessageResponse, error)
	UploadFile(ctx context.Context, path string) (*BaleUploadFileResponse, error)
}

type httpBaleClient struct {
	cfg    config.BaleConfig
	base   string
	client *http.Client
}

func newHTTPBaleClient(cfg config.BaleConfig) *httpBaleClient {
	return &httpBaleClient{
		cfg:  cfg,
		base: defaultBaleAPIDomain,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *httpBaleClient) SendMessage(ctx context.Context, reqBody *BaleSendMessageRequest) (*BaleSendMessageResponse, error) {
	if c.cfg.APIAccessKey == "" {
		return nil, fmt.Errorf("bale api access key is not configured")
	}
	if reqBody == nil {
		return nil, fmt.Errorf("request body is nil")
	}

	payload, _ := json.Marshal(reqBody)
	url := c.base + "/api/v3/send_message"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("api-access-key", c.cfg.APIAccessKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bale send_message http status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out BaleSendMessageResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode bale send response: %w", err)
	}
	return &out, nil
}

func (c *httpBaleClient) UploadFile(ctx context.Context, path string) (*BaleUploadFileResponse, error) {
	if c.cfg.APIAccessKey == "" {
		return nil, fmt.Errorf("bale api access key is not configured")
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("upload path is empty")
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile("file", f.Name())
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	url := c.base + "/api/v3/upload_file"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("api-access-key", c.cfg.APIAccessKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bale upload_file http status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out BaleUploadFileResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode bale upload response: %w", err)
	}
	if out.FileID == "" {
		return nil, fmt.Errorf("bale upload returned empty file_id")
	}
	return &out, nil
}
