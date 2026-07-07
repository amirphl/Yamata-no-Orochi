package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
)

func TestNormalizeBaleProvider(t *testing.T) {
	t.Parallel()

	if got := normalizeBaleProvider(""); got != baleProviderAuto {
		t.Fatalf("normalize empty: got=%q want=%q", got, baleProviderAuto)
	}
	if got := normalizeBaleProvider("legacy"); got != baleProviderLegacy {
		t.Fatalf("normalize legacy: got=%q want=%q", got, baleProviderLegacy)
	}
	if got := normalizeBaleProvider("NAJVA"); got != baleProviderNajvaV2 {
		t.Fatalf("normalize najva: got=%q want=%q", got, baleProviderNajvaV2)
	}
}

func TestSupportsStatusTracking(t *testing.T) {
	t.Parallel()

	legacy := &httpBaleClient{provider: baleProviderLegacy}
	if legacy.SupportsStatusTracking() {
		t.Fatalf("legacy provider must not support status tracking")
	}

	najva := &httpBaleClient{provider: baleProviderNajvaV2}
	if !najva.SupportsStatusTracking() {
		t.Fatalf("najva provider must support status tracking")
	}

	auto := &httpBaleClient{provider: baleProviderAuto}
	if !auto.SupportsStatusTracking() {
		t.Fatalf("auto provider must support status tracking")
	}
}

func TestCanSendNajvaBatchMainFlow(t *testing.T) {
	t.Parallel()

	reqs := []BaleSendMessageRequest{
		{
			BotID:       123,
			PhoneNumber: "09111111111",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "hello"}},
		},
		{
			BotID:       123,
			PhoneNumber: "09222222222",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "hello"}},
		},
	}
	if !canSendNajvaBatch(reqs) {
		t.Fatalf("expected najva batch main flow to be allowed")
	}

	reqs[1].MessageData.Message.Text = "different"
	if canSendNajvaBatch(reqs) {
		t.Fatalf("batch should be rejected when messages differ")
	}
}

func TestCanSendNajvaP2PBatchMainFlow(t *testing.T) {
	t.Parallel()

	reqs := []BaleSendMessageRequest{
		{
			BotID:       123,
			PhoneNumber: "09111111111",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "msg1"}},
		},
		{
			BotID:       123,
			PhoneNumber: "09222222222",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "msg2"}},
		},
	}
	if !canSendNajvaP2PBatch(reqs) {
		t.Fatalf("expected najva p2p batch main flow to be allowed")
	}
}

func TestValidateNajvaRequestsMainFlow(t *testing.T) {
	t.Parallel()

	if err := validateNajvaBulkSendRequest([]string{"09111111111"}, "hello", "123"); err != nil {
		t.Fatalf("bulk validation failed: %v", err)
	}
	if err := validateNajvaP2PSendRequest(
		[]string{"09111111111", "09222222222"},
		[]string{"hello", "world"},
		"123",
	); err != nil {
		t.Fatalf("p2p validation failed: %v", err)
	}
}

func TestDecodeNajvaSendAndStatusItemsMainFlow(t *testing.T) {
	t.Parallel()

	sendBody := []byte(`{
		"return": {"status": 200, "message": "ok"},
		"entries": [
			{"messageid":"1001","status":1,"statustext":"ok","receptor":"09111111111"}
		]
	}`)
	sendItems, err := decodeNajvaSendItems(sendBody)
	if err != nil {
		t.Fatalf("decodeNajvaSendItems failed: %v", err)
	}
	if len(sendItems) != 1 || normalizeAnyToString(sendItems[0].MessageID) != "1001" {
		t.Fatalf("unexpected send items: %+v", sendItems)
	}

	statusBody := []byte(`{
		"return": {"status": 200, "message": "ok"},
		"entries": [
			{"messageid":"1001","status":10,"statustext":"delivered"}
		]
	}`)
	statusItems, err := decodeNajvaStatusItems(statusBody)
	if err != nil {
		t.Fatalf("decodeNajvaStatusItems failed: %v", err)
	}
	if len(statusItems) != 1 || normalizeAnyToInt(statusItems[0].Status) != 10 {
		t.Fatalf("unexpected status items: %+v", statusItems)
	}
}

func TestNormalizeHelpersMainFlow(t *testing.T) {
	t.Parallel()

	if got := normalizeAnyToString(json.Number("123")); got != "123" {
		t.Fatalf("normalizeAnyToString(json.Number): got=%q", got)
	}
	if got := normalizeAnyToInt("42"); got != 42 {
		t.Fatalf("normalizeAnyToInt(string): got=%d", got)
	}
	if !isNajvaSendImmediateFailure(11) {
		t.Fatalf("expected failure code 11 to be immediate failure")
	}
	if firstNonEmpty("", "  ", "x") != "x" {
		t.Fatalf("firstNonEmpty did not return expected value")
	}
	if errString(nil) != "" {
		t.Fatalf("errString(nil) must return empty string")
	}
}

func TestBackoffAndRetryMainFlow(t *testing.T) {
	t.Parallel()

	if d := baleRetryBackoffDelay(0); d != baleRetryBaseDelay {
		t.Fatalf("attempt=0 delay mismatch: got=%s want=%s", d, baleRetryBaseDelay)
	}
	if d := baleRetryBackoffDelay(100); d != baleRetryMaxDelay {
		t.Fatalf("large attempt delay mismatch: got=%s want=%s", d, baleRetryMaxDelay)
	}

	retryErr := newBaleHTTPError("op", 503, []byte(`temporary unavailable`))
	if !isBaleRetryableError(retryErr) {
		t.Fatalf("503 http error should be retryable")
	}
	retryErr = newBaleHTTPError("op", 500, []byte(`internal server error`))
	if !isBaleRetryableError(retryErr) {
		t.Fatalf("500 http error should be retryable")
	}
	if !isBaleStatusRetryable(retryErr) {
		t.Fatalf("status retry check should treat retryable http statuses as retryable")
	}
}

func TestEndpointNotSupportedMainFlow(t *testing.T) {
	t.Parallel()

	err := newBaleHTTPError("op", http.StatusNotFound, []byte("not found"))
	if !isEndpointNotSupported(err) {
		t.Fatalf("404 should be treated as endpoint-not-supported")
	}

	err = newBaleHTTPError("op", http.StatusBadRequest, []byte("bad request"))
	if isEndpointNotSupported(err) {
		t.Fatalf("400 should not be treated as endpoint-not-supported")
	}
}

func TestExtractAndAuthHeaderMainFlow(t *testing.T) {
	t.Parallel()

	fileID := "abc"
	reqBody := &BaleSendMessageRequest{
		MessageData: BaleSendMessageData{
			Message: &BaleMessage{
				Text:   " hi ",
				FileID: &fileID,
			},
		},
	}
	text, outFileID := extractBaleMessagePayload(reqBody)
	if text != "hi" {
		t.Fatalf("extractBaleMessagePayload text mismatch: got=%q", text)
	}
	if outFileID == nil || *outFileID != "abc" {
		t.Fatalf("extractBaleMessagePayload file id mismatch")
	}

	trimmed := normalizeOptionalStringPtrRef(&fileID)
	if trimmed == nil || *trimmed != "abc" {
		t.Fatalf("normalizeOptionalStringPtrRef mismatch")
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	setBaleAuthHeaders(req, "raw-token")
	if got := req.Header.Get("Authorization"); got != "Bearer raw-token" {
		t.Fatalf("Authorization header mismatch: got=%q", got)
	}
	if got := req.Header.Get("api-access-key"); got != "Bearer raw-token" {
		t.Fatalf("api-access-key header mismatch: got=%q", got)
	}

	req2, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request 2 failed: %v", err)
	}
	setBaleAuthHeaders(req2, "Bearer already-prefixed")
	if got := req2.Header.Get("Authorization"); got != "Bearer already-prefixed" {
		t.Fatalf("Authorization header should preserve bearer prefix: got=%q", got)
	}
}

func TestRetryableResponseMainFlow(t *testing.T) {
	t.Parallel()

	resp := &BaleSendMessageResponse{
		ErrorData: []BaleErrorData{
			{CodeRaw: "RATE_LIMIT", Description: "too many requests"},
		},
	}
	if !isBaleRetryableResponse(resp) {
		t.Fatalf("rate-limit response should be retryable")
	}
}

func TestIsPositiveSenderID(t *testing.T) {
	t.Parallel()

	if !isPositiveSenderID(1) {
		t.Fatalf("positive sender id must be valid")
	}
	if isPositiveSenderID(0) {
		t.Fatalf("sender id 0 must be invalid")
	}
}

func TestValidateNajvaUploadFileInvalidExt(t *testing.T) {
	t.Parallel()

	err := validateNajvaUploadFile("sample.txt")
	if err == nil {
		t.Fatalf("expected invalid extension error")
	}
}

func TestBaleHTTPErrorMessage(t *testing.T) {
	t.Parallel()

	err := newBaleHTTPError("send", 500, []byte("x"))
	if err == nil {
		t.Fatalf("expected non-nil error")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("expected non-empty error text")
	}
}

func TestBaleRetryableTimeoutString(t *testing.T) {
	t.Parallel()

	err := errorsNew("request timed out")
	if !isBaleRetryableError(err) {
		t.Fatalf("timeout string should be retryable")
	}
}

// errorsNew allows local construction without importing extra packages in every test body.
func errorsNew(msg string) error { return &timeoutLikeErr{s: msg} }

type timeoutLikeErr struct{ s string }

func (e *timeoutLikeErr) Error() string   { return e.s }
func (e *timeoutLikeErr) Timeout() bool   { return true }
func (e *timeoutLikeErr) Temporary() bool { return true }

func TestBaleRetryDelayMonotonic(t *testing.T) {
	t.Parallel()

	prev := time.Duration(0)
	for i := 0; i < 8; i++ {
		cur := baleRetryBackoffDelay(i)
		if cur < prev {
			t.Fatalf("delay must be monotonic: prev=%s cur=%s", prev, cur)
		}
		prev = cur
	}
}

func TestNajvaDocumentedLimits(t *testing.T) {
	t.Parallel()

	if najvaMaxRecipients != 9000 {
		t.Fatalf("najvaMaxRecipients mismatch: got=%d want=9000", najvaMaxRecipients)
	}
	if najvaMaxStatusIDs != 900 {
		t.Fatalf("najvaMaxStatusIDs mismatch: got=%d want=900", najvaMaxStatusIDs)
	}
	if najvaMaxFileBytes != 10*1024*1024 {
		t.Fatalf("najvaMaxFileBytes mismatch: got=%d want=%d", najvaMaxFileBytes, 10*1024*1024)
	}
}

func TestNajvaStatusCodesCoverage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code         int
		shouldFail   bool
		hasStatusTxt bool
	}{
		{code: 1, shouldFail: false, hasStatusTxt: true},
		{code: 2, shouldFail: false, hasStatusTxt: true},
		{code: 4, shouldFail: false, hasStatusTxt: true},
		{code: 6, shouldFail: true, hasStatusTxt: true},
		{code: 10, shouldFail: false, hasStatusTxt: true},
		{code: 11, shouldFail: true, hasStatusTxt: true},
		{code: 13, shouldFail: true, hasStatusTxt: true},
		{code: 14, shouldFail: true, hasStatusTxt: true},
		{code: 100, shouldFail: true, hasStatusTxt: true},
	}

	for _, tc := range cases {
		if got := isNajvaSendImmediateFailure(tc.code); got != tc.shouldFail {
			t.Fatalf("isNajvaSendImmediateFailure(%d): got=%v want=%v", tc.code, got, tc.shouldFail)
		}
		text := najvaStatusText(tc.code)
		if tc.hasStatusTxt && strings.TrimSpace(text) == "" {
			t.Fatalf("najvaStatusText(%d) should not be empty", tc.code)
		}
	}
}

func TestNajvaErrorCodeCoverage(t *testing.T) {
	t.Parallel()

	sendCodes := map[int]bool{
		400: true,
		414: true,
		418: true,
	}
	for code := range sendCodes {
		desc := najvaHTTPErrorDescription("najva send", code)
		if strings.TrimSpace(desc) == "" {
			t.Fatalf("najva send code %d must have documented description", code)
		}
	}

	uploadCodes := map[int]bool{
		400: true,
		413: true,
	}
	for code := range uploadCodes {
		desc := najvaHTTPErrorDescription("najva upload_file", code)
		if strings.TrimSpace(desc) == "" {
			t.Fatalf("najva upload code %d must have documented description", code)
		}
	}

	statusCodes := map[int]bool{
		400: true,
		414: true,
	}
	for code := range statusCodes {
		desc := najvaHTTPErrorDescription("najva status", code)
		if strings.TrimSpace(desc) == "" {
			t.Fatalf("najva status code %d must have documented description", code)
		}
	}
}

func TestNewBaleHTTPErrorFallbackBody(t *testing.T) {
	t.Parallel()

	err := newBaleHTTPError("najva upload_file", 413, nil)
	if err == nil {
		t.Fatalf("expected non-nil error")
	}
	got := err.Error()
	if !strings.Contains(strings.ToLower(got), "15mb") {
		t.Fatalf("expected fallback description in error body, got=%q", got)
	}
}

func TestNormalizeNajvaPhoneNumber(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "09123456789", want: "09123456789"},
		{in: "989123456789", want: "09123456789"},
		{in: "+989123456789", want: "09123456789"},
		{in: "00989123456789", want: "09123456789"},
		{in: "9123456789", want: "09123456789"},
		{in: "  +989123456789  ", want: "09123456789"},
	}

	for _, tc := range cases {
		if got := normalizeNajvaPhoneNumber(tc.in); got != tc.want {
			t.Fatalf("normalizeNajvaPhoneNumber(%q): got=%q want=%q", tc.in, got, tc.want)
		}
	}
}

func TestSendNajvaUsesReceiversAndNormalizesPhones(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/sms/send" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if _, ok := payload["receptor"]; ok {
			t.Fatalf("payload must not include receptor key")
		}

		receivers, ok := payload["receivers"].([]any)
		if !ok || len(receivers) != 1 {
			t.Fatalf("invalid receivers payload: %#v", payload["receivers"])
		}
		if got := strings.TrimSpace(receivers[0].(string)); got != "09121111111" {
			t.Fatalf("normalized receiver mismatch: got=%q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"return": {"status": 200, "message": "ok"},
			"entries": [
				{"messageid":"m1","status":1,"statustext":"ok","receptor":"09121111111"}
			]
		}`))
	}))
	defer srv.Close()

	client := newHTTPBaleClient(config.BaleConfig{
		APIAccessKey: "k",
		Provider:     baleProviderNajvaV2,
		NajvaDomain:  srv.URL,
	})

	resp, err := client.sendNajva(context.Background(), &BaleSendMessageRequest{
		BotID:       123,
		PhoneNumber: "+989121111111",
		MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("sendNajva failed: %v", err)
	}
	if resp == nil || resp.MessageID != "m1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSendNajvaP2PUsesReceiversAndMatchesByNormalizedPhone(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/sms/send-p2p" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if _, ok := payload["receptor"]; ok {
			t.Fatalf("payload must not include receptor key")
		}

		if _, ok := payload["receivers"]; ok {
			t.Fatalf("payload must not include receivers key for p2p")
		}
		messages, ok := payload["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("invalid messages payload: %#v", payload["messages"])
		}
		first, ok := messages[0].(map[string]any)
		if !ok {
			t.Fatalf("messages[0] invalid type: %#v", messages[0])
		}
		second, ok := messages[1].(map[string]any)
		if !ok {
			t.Fatalf("messages[1] invalid type: %#v", messages[1])
		}
		if got := strings.TrimSpace(first["receiver"].(string)); got != "09121111111" {
			t.Fatalf("messages[0].receiver mismatch: got=%q", got)
		}
		if got := strings.TrimSpace(second["receiver"].(string)); got != "09123333333" {
			t.Fatalf("messages[1].receiver mismatch: got=%q", got)
		}
		if got := strings.TrimSpace(first["message"].(string)); got != "a" {
			t.Fatalf("messages[0].message mismatch: got=%q", got)
		}
		if got := strings.TrimSpace(second["message"].(string)); got != "b" {
			t.Fatalf("messages[1].message mismatch: got=%q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		// Intentionally reverse item order to ensure mapping uses receiver value.
		_, _ = w.Write([]byte(`{
			"return": {"status": 200, "message": "ok"},
			"entries": [
				{"messageid":"m2","status":1,"statustext":"ok","receptor":"09123333333"},
				{"messageid":"m1","status":1,"statustext":"ok","receptor":"09121111111"}
			]
		}`))
	}))
	defer srv.Close()

	client := newHTTPBaleClient(config.BaleConfig{
		APIAccessKey: "k",
		Provider:     baleProviderNajvaV2,
		NajvaDomain:  srv.URL,
	})

	out, err := client.sendNajvaP2PBatchOnce(context.Background(), []BaleSendMessageRequest{
		{
			RequestID:   "r1",
			BotID:       123,
			PhoneNumber: "+989121111111",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "a"}},
		},
		{
			RequestID:   "r2",
			BotID:       123,
			PhoneNumber: "989123333333",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "b"}},
		},
	})
	if err != nil {
		t.Fatalf("sendNajvaP2PBatchOnce failed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("unexpected response size: %d", len(out))
	}
	if out[0].MessageID != "m1" || out[1].MessageID != "m2" {
		t.Fatalf("unexpected mapped message ids: %+v", out)
	}
}

func TestUploadFileNajvaMultipartAndEnvelopeResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload-file/bale" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if ct := strings.TrimSpace(r.Header.Get("Content-Type")); !strings.HasPrefix(ct, "multipart/form-data;") {
			t.Fatalf("expected multipart content type, got %q", ct)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		fileHeaders := r.MultipartForm.File["file"]
		if len(fileHeaders) != 1 {
			t.Fatalf("expected one file part, got=%d", len(fileHeaders))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"return": {"status": 200, "message": "ok"},
			"entries": {"fild_id":"uuid-123"}
		}`))
	}))
	defer srv.Close()

	tmpPath := filepath.Join(t.TempDir(), "upload.png")
	if err := os.WriteFile(tmpPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	client := newHTTPBaleClient(config.BaleConfig{
		APIAccessKey: "k",
		Provider:     baleProviderNajvaV2,
		NajvaDomain:  srv.URL,
	})

	resp, err := client.uploadFileNajva(context.Background(), tmpPath)
	if err != nil {
		t.Fatalf("uploadFileNajva failed: %v", err)
	}
	if resp == nil || strings.TrimSpace(resp.FileID) != "uuid-123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestUploadFileNajvaInfersExtensionWhenPathHasNoExt(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload-file/bale" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		fileHeaders := r.MultipartForm.File["file"]
		if len(fileHeaders) != 1 {
			t.Fatalf("expected one file part, got=%d", len(fileHeaders))
		}
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(fileHeaders[0].Filename)), ".png") {
			t.Fatalf("expected inferred .png filename, got=%q", fileHeaders[0].Filename)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"return": {"status": 200, "message": "ok"},
			"entries": {"fild_id":"uuid-456"}
		}`))
	}))
	defer srv.Close()

	tmpPath := filepath.Join(t.TempDir(), "upload_no_ext")
	// Minimal PNG signature so DetectContentType can infer image/png.
	content := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if err := os.WriteFile(tmpPath, content, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	client := newHTTPBaleClient(config.BaleConfig{
		APIAccessKey: "k",
		Provider:     baleProviderNajvaV2,
		NajvaDomain:  srv.URL,
	})

	resp, err := client.uploadFileNajva(context.Background(), tmpPath)
	if err != nil {
		t.Fatalf("uploadFileNajva failed: %v", err)
	}
	if resp == nil || strings.TrimSpace(resp.FileID) != "uuid-456" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestDecodeNajvaUploadFileIDSupportsEnvelopeAndFlat(t *testing.T) {
	t.Parallel()

	id, err := decodeNajvaUploadFileID([]byte(`{"file_id":"flat-1"}`))
	if err != nil || id != "flat-1" {
		t.Fatalf("flat decode failed: id=%q err=%v", id, err)
	}

	id, err = decodeNajvaUploadFileID([]byte(`{
		"return": {"status": 200, "message": "ok"},
		"entries": {"fild_id":"env-1"}
	}`))
	if err != nil || id != "env-1" {
		t.Fatalf("envelope decode failed: id=%q err=%v", id, err)
	}
}

func TestFetchStatusChunkUsesMessageIDsPayloadAndEntriesResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/sms/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		ids, ok := payload["messageids"].([]any)
		if !ok || len(ids) != 3 {
			t.Fatalf("invalid messageids payload: %#v", payload["messageids"])
		}
		if normalizeAnyToInt(ids[0]) != 1 || normalizeAnyToInt(ids[1]) != 2 || normalizeAnyToInt(ids[2]) != 3 {
			t.Fatalf("unexpected messageids: %#v", ids)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"return": {"status": 200, "message": "ok"},
			"entries": [
				{"messageid":1,"status":10,"statustext":"رسیده به گیرنده"},
				{"messageid":2,"status":4,"statustext":"رسیده به مخابرات"},
				{"messageid":3,"status":100,"statustext":"شناسه پیامک نامعتبر است"}
			]
		}`))
	}))
	defer srv.Close()

	client := newHTTPBaleClient(config.BaleConfig{
		APIAccessKey: "k",
		Provider:     baleProviderNajvaV2,
		NajvaDomain:  srv.URL,
	})

	out, err := client.fetchStatusChunkOnce(context.Background(), []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("fetchStatusChunkOnce failed: %v", err)
	}
	if len(out.Items) != 3 {
		t.Fatalf("unexpected response size: %d", len(out.Items))
	}
	if out.Items[0].MessageID != "1" || out.Items[0].Status != 10 {
		t.Fatalf("unexpected first item: %+v", out.Items[0])
	}
	if out.Items[1].MessageID != "2" || out.Items[1].Status != 4 {
		t.Fatalf("unexpected second item: %+v", out.Items[1])
	}
	if out.Items[2].MessageID != "3" || out.Items[2].Status != 100 {
		t.Fatalf("unexpected third item: %+v", out.Items[2])
	}
	if out.RawResponse == nil || !strings.Contains(*out.RawResponse, `"messageid":1`) {
		t.Fatalf("expected raw Najva response, got=%v", out.RawResponse)
	}
}

func TestFetchStatusChunkRetainsRawErrorResponse(t *testing.T) {
	t.Parallel()

	const rawResponse = `{"error":"invalid message id"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(rawResponse))
	}))
	defer srv.Close()

	client := newHTTPBaleClient(config.BaleConfig{
		APIAccessKey: "k",
		Provider:     baleProviderNajvaV2,
		NajvaDomain:  srv.URL,
	})

	result, err := client.fetchStatusChunkOnce(context.Background(), []int64{1})
	if err == nil {
		t.Fatalf("expected status request to fail")
	}
	if result.RawResponse == nil || *result.RawResponse != rawResponse {
		t.Fatalf("raw response mismatch: got=%v want=%q", result.RawResponse, rawResponse)
	}
}
