package scheduler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
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
			PhoneNumber: "989111111111",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "hello"}},
		},
		{
			BotID:       123,
			PhoneNumber: "989222222222",
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
			PhoneNumber: "989111111111",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "msg1"}},
		},
		{
			BotID:       123,
			PhoneNumber: "989222222222",
			MessageData: BaleSendMessageData{Message: &BaleMessage{Text: "msg2"}},
		},
	}
	if !canSendNajvaP2PBatch(reqs) {
		t.Fatalf("expected najva p2p batch main flow to be allowed")
	}
}

func TestValidateNajvaRequestsMainFlow(t *testing.T) {
	t.Parallel()

	if err := validateNajvaBulkSendRequest([]string{"989111111111"}, "hello", "123"); err != nil {
		t.Fatalf("bulk validation failed: %v", err)
	}
	if err := validateNajvaP2PSendRequest(
		[]string{"989111111111", "989222222222"},
		[]string{"hello", "world"},
		"123",
	); err != nil {
		t.Fatalf("p2p validation failed: %v", err)
	}
}

func TestDecodeNajvaSendAndStatusItemsMainFlow(t *testing.T) {
	t.Parallel()

	sendBody := []byte(`[{"messageid":"1001","status":1,"statustext":"ok","receptor":"989111111111"}]`)
	sendItems, err := decodeNajvaSendItems(sendBody)
	if err != nil {
		t.Fatalf("decodeNajvaSendItems failed: %v", err)
	}
	if len(sendItems) != 1 || normalizeAnyToString(sendItems[0].MessageID) != "1001" {
		t.Fatalf("unexpected send items: %+v", sendItems)
	}

	statusBody := []byte(`{"items":[{"messageid":"1001","status":10,"statustext":"delivered"}]}`)
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

	if najvaMaxRecipients != 10000 {
		t.Fatalf("najvaMaxRecipients mismatch: got=%d want=10000", najvaMaxRecipients)
	}
	if najvaMaxStatusIDs != 1000 {
		t.Fatalf("najvaMaxStatusIDs mismatch: got=%d want=1000", najvaMaxStatusIDs)
	}
	if najvaMaxFileBytes != 15*1024*1024 {
		t.Fatalf("najvaMaxFileBytes mismatch: got=%d want=%d", najvaMaxFileBytes, 15*1024*1024)
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
