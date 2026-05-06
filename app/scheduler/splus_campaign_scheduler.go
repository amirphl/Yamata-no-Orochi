package scheduler

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	defaultSplusBaseURL = "https://bui.splus.ir"
	splusSendMaxRetries = 5
	splusMediaMaxSize   = int64(8 * 1024 * 1024)
)

type SplusCampaignScheduler struct {
	audRepo      repository.AudienceProfileRepository
	tagRepo      repository.TagRepository
	sentRepo     repository.SentSplusMessageRepository
	pcRepo       repository.ProcessedCampaignRepository
	notifier     NotificationSender
	logger       *log.Logger
	interval     time.Duration
	messageDelay time.Duration

	db       *gorm.DB
	adminCfg config.AdminConfig
	splusCfg config.SplusConfig
	botCfg   config.BotConfig

	botClient   BotClient
	splusClient SplusClient

	logFile *os.File

	schedulerName string

	audienceCache *AudienceCache
}

type SplusClient interface {
	SendMessage(ctx context.Context, botID string, req *SplusSendMessageRequest) (*SplusResponse, error)
	UploadFile(ctx context.Context, botID string, path string) (*SplusUploadResponse, error)
}

type SplusSendMessageRequest struct {
	PhoneNumber string  `json:"phone_number,omitempty"`
	UserID      string  `json:"user_id,omitempty"`
	Text        string  `json:"text,omitempty"`
	FileID      *string `json:"file_id,omitempty"`
}

type SplusResponse struct {
	ResultCode    int     `json:"result_code"`
	ResultMessage string  `json:"result_message"`
	MessageID     *string `json:"message_id,omitempty"`
	RequestID     *int64  `json:"request_id,omitempty"`
	UserID        *string `json:"user_id,omitempty"`
	HTTPStatus    int     `json:"-"`
}

type SplusUploadResponse struct {
	ResultCode    int    `json:"result_code"`
	ResultMessage string `json:"result_message"`
	FileID        string `json:"file_id"`
	HTTPStatus    int    `json:"-"`
}

type httpSplusClient struct {
	cfg    config.SplusConfig
	client *http.Client
}

func newHTTPSplusClient(cfg config.SplusConfig) *httpSplusClient {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultSplusBaseURL
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	return &httpSplusClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *httpSplusClient) SendMessage(ctx context.Context, botID string, reqBody *SplusSendMessageRequest) (*SplusResponse, error) {
	if strings.TrimSpace(botID) == "" {
		return nil, fmt.Errorf("splus bot id is not configured")
	}
	if reqBody == nil {
		return nil, fmt.Errorf("splus request body is nil")
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/messages/send", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", botID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out SplusResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode splus send message response: %w", err)
		}
	}
	out.HTTPStatus = resp.StatusCode

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if out.ResultCode != 0 || strings.TrimSpace(out.ResultMessage) != "" {
			return &out, fmt.Errorf("splus send http status: %d result_code=%d result_message=%s", resp.StatusCode, out.ResultCode, strings.TrimSpace(out.ResultMessage))
		}
		return nil, fmt.Errorf("splus send http status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return &out, nil
}

func (c *httpSplusClient) UploadFile(ctx context.Context, botID string, path string) (*SplusUploadResponse, error) {
	if strings.TrimSpace(botID) == "" {
		return nil, fmt.Errorf("splus bot id is not configured")
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("upload path is empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > splusMediaMaxSize {
		return nil, fmt.Errorf("file size exceeds splus upload limit (8MB)")
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/file/upload", buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", botID)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out SplusUploadResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode splus upload response: %w", err)
		}
	}
	out.HTTPStatus = resp.StatusCode

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if out.ResultCode != 0 || strings.TrimSpace(out.ResultMessage) != "" {
			return &out, fmt.Errorf("splus upload http status: %d result_code=%d result_message=%s", resp.StatusCode, out.ResultCode, strings.TrimSpace(out.ResultMessage))
		}
		return nil, fmt.Errorf("splus upload http status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if strings.TrimSpace(out.FileID) == "" {
		return nil, fmt.Errorf("splus upload returned empty file_id")
	}

	return &out, nil
}

func NewSplusCampaignScheduler(
	audRepo repository.AudienceProfileRepository,
	tagRepo repository.TagRepository,
	sentRepo repository.SentSplusMessageRepository,
	pcRepo repository.ProcessedCampaignRepository,
	notifier NotificationSender,
	db *gorm.DB,
	logger *log.Logger,
	interval time.Duration,
	messageDelay time.Duration,
	splusCfg config.SplusConfig,
	botCfg config.BotConfig,
	adminCfg config.AdminConfig,
) *SplusCampaignScheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	if botCfg.APIDomain == "" {
		botCfg.APIDomain = defaultBotAPIDomain
	}

	s := &SplusCampaignScheduler{
		audRepo:       audRepo,
		tagRepo:       tagRepo,
		sentRepo:      sentRepo,
		pcRepo:        pcRepo,
		notifier:      notifier,
		logger:        logger,
		db:            db,
		interval:      interval,
		messageDelay:  messageDelay,
		adminCfg:      adminCfg,
		splusCfg:      splusCfg,
		botCfg:        botCfg,
		botClient:     newHTTPBotClient(botCfg),
		splusClient:   newHTTPSplusClient(splusCfg),
		audienceCache: NewAudienceCache(repository.NewAudienceSelectionRepository(db)),
		schedulerName: "splus",
	}

	if err := s.initSchedulerLogger(); err != nil {
		s.logger = log.New(io.Discard, "splus_scheduler ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		s.logger.Printf("splus scheduler: failed to initialize file logger: %v", err)
	}
	return s
}

func (s *SplusCampaignScheduler) initSchedulerLogger() error {
	l, f, err := initSchedulerLogger(s.schedulerName + "_scheduler")
	if err != nil {
		return err
	}
	s.logFile = f
	s.logger = l
	return nil
}

func (s *SplusCampaignScheduler) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce(context.Background())
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// TODO: This is not the correct way of context handling.
				s.runOnce(context.Background())
			}
		}
	}()

	return func() {
		cancel()
		if s.logFile != nil {
			_ = s.logFile.Close()
		}
	}
}

func (s *SplusCampaignScheduler) runOnce(ctx context.Context) {
	token, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("splus scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Splus scheduler bot login failed: %v", err))
		return
	}

	ready, err := s.botClient.ListReadyCampaigns(ctx, token, models.CampaignPlatformSPlus)
	if err != nil {
		s.logger.Printf("splus scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Splus scheduler list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}

	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformSPlus {
			// NOTE: Just skip.
			continue
		}
		if err := s.validateSplusCampaign(c); err != nil {
			s.logger.Printf("splus scheduler: validate campaign failed for id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Splus scheduler validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}

		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("splus scheduler: check processed failed for id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Splus scheduler check processed failed for id=%d: %v", c.ID, err))
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		}
	}
	if len(pending) == 0 {
		return
	}

	for _, camp := range pending {
		c := camp
		go func() {
			if err := s.processSplusCampaign(ctx, token, c); err != nil {
				s.logger.Printf("splus scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("Splus scheduler process campaign failed for id=%d: %v", c.ID, err))
			}
		}()
	}
}

func (s *SplusCampaignScheduler) processSplusCampaign(ctx context.Context, token string, c dto.BotGetCampaignResponse) error {
	botID, err := extractSplusBotID(c)
	if err != nil {
		return fmt.Errorf("resolve splus bot id: %w", err)
	}

	if err := s.botClient.MoveCampaignToRunning(ctx, token, c.ID); err != nil {
		return fmt.Errorf("move to running: %w", err)
	}
	s.logger.Printf("splus scheduler: campaign id=%d moved to running", c.ID)

	// First transaction: create processed_campaign and persist full audience IDs
	var (
		phones []string
		ids    []int64
		uids   []string
		pc     *models.ProcessedCampaign
	)

	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		pc = &models.ProcessedCampaign{
			CampaignID:          c.ID,
			CampaignJSON:        func() json.RawMessage { b, _ := json.Marshal(c); return b }(),
			AudienceIDs:         pq.Int64Array{},
			AudienceCodes:       []string{},
			LastAudienceID:      nil,
			AudienceSelectionID: nil,
			Statistics:          nil,
		}
		if err := s.pcRepo.Save(txCtx, pc); err != nil {
			return err
		}
		s.logger.Printf("splus scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

		// Fetch audiences (white then pink, DB-shuffled), and sort order is enforced inside repo
		var (
			selectionID uint
			err         error
		)
		correlationID := uuid.NewString()
		phones, ids, uids, selectionID, err = s.fetchSplusAudiencePhones(txCtx, c, token, correlationID)
		if err != nil {
			return err
		}
		s.logger.Printf("splus scheduler: fetched %d audience phones for campaign id=%d", len(phones), c.ID)

		pc.AudienceIDs = pq.Int64Array(ids)
		pc.AudienceCodes = uids
		pc.AudienceSelectionID = utils.ToPtr(selectionID)
		pc.UpdatedAt = utils.UTCNow()
		if err := s.pcRepo.Update(txCtx, pc); err != nil {
			return err
		}
		s.logger.Printf("splus scheduler: updated processed campaign id=%d with audience ids", pc.ID)

		return nil
	}); err != nil {
		return err
	}
	s.logger.Printf("splus scheduler: persisted processed campaign id=%d num_audiences=%d", pc.ID, len(ids))

	fileID, err := s.uploadCampaignMedia(ctx, token, botID, c)
	if err != nil {
		return err
	}

	for i, phone := range phones {
		trackingID := uuid.NewString()
		row := &models.SentSplusMessage{
			ProcessedCampaignID: pc.ID,
			PhoneNumber:         phone,
			PartsDelivered:      0,
			Status:              models.SplusSendStatusPending,
			TrackingID:          trackingID,
		}
		if err := s.sentRepo.Save(ctx, row); err != nil {
			return err
		}

		msg := &SplusSendMessageRequest{
			PhoneNumber: phone,
			Text:        s.buildSplusMessageBody(c, uids[i]),
			FileID:      fileID,
		}

		resp, sendErr := s.sendWithRetry(ctx, botID, msg)
		status, parts, serverID, errorCode, desc := s.resolveSendResult(resp, sendErr)
		if err := s.sentRepo.UpdateSendResultByTrackingID(ctx, trackingID, status, parts, serverID, errorCode, desc); err != nil {
			return err
		}

		pc.LastAudienceID = &ids[i]
		pc.UpdatedAt = utils.UTCNow()
		if err := s.pcRepo.Update(ctx, pc); err != nil {
			return err
		}
		if i < len(phones)-1 {
			if err := sleepWithContext(ctx, s.messageDelay); err != nil {
				return err
			}
		}
	}

	stats, err := s.updateProcessedCampaignStatsFromSentRows(ctx, pc.ID)
	if err != nil {
		return err
	}
	if err := s.botClient.PushCampaignStatistics(ctx, c.ID, stats); err != nil {
		return err
	}

	s.logger.Printf("splus scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, token, c.ID); err != nil {
		s.logger.Printf("splus scheduler: move executed failed for campaign id=%d: %v", c.ID, err)
		return err
	}
	s.logger.Printf("splus scheduler: campaign id=%d moved to executed", c.ID)

	return nil
}

func (s *SplusCampaignScheduler) uploadCampaignMedia(ctx context.Context, token, botID string, c dto.BotGetCampaignResponse) (*string, error) {
	if c.MediaUUID == nil {
		return nil, nil
	}

	path, err := s.downloadCampaignMediaViaBotAPI(ctx, token, c.MediaUUID.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(path) }()

	resp, err := s.splusClient.UploadFile(ctx, botID, path)
	if err != nil {
		return nil, err
	}
	return &resp.FileID, nil
}

func (s *SplusCampaignScheduler) downloadCampaignMediaViaBotAPI(ctx context.Context, token, mediaUUID string) (string, error) {
	url := strings.TrimRight(s.botCfg.APIDomain, "/") + "/api/v1/bot/media/" + mediaUUID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "*/*")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("bot media download http status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmpFile, err := os.CreateTemp("", "splus-media-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func (s *SplusCampaignScheduler) sendWithRetry(ctx context.Context, botID string, req *SplusSendMessageRequest) (*SplusResponse, error) {
	var (
		resp *SplusResponse
		err  error
	)
	for attempt := 0; attempt <= splusSendMaxRetries; attempt++ {
		resp, err = s.splusClient.SendMessage(ctx, botID, req)
		if !isRetryableSplusError(resp, err) {
			return resp, err
		}
		if attempt == splusSendMaxRetries {
			break
		}
		backoff := time.Duration(1<<attempt) * time.Second
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return resp, ctx.Err()
		case <-timer.C:
		}
	}
	return resp, err
}

func (s *SplusCampaignScheduler) resolveSendResult(resp *SplusResponse, sendErr error) (models.SplusSendStatus, int, *string, *string, *string) {
	if sendErr != nil {
		code := "SEND_FAILED"
		desc := sendErr.Error()
		if resp != nil && resp.ResultCode != 0 {
			code = strconv.Itoa(resp.ResultCode)
			if strings.TrimSpace(resp.ResultMessage) != "" {
				desc = resp.ResultMessage
			}
		}
		return models.SplusSendStatusUnsuccessful, 0, nil, &code, &desc
	}

	if resp == nil {
		code := "EMPTY_RESPONSE"
		desc := "empty response from splus"
		return models.SplusSendStatusUnsuccessful, 0, nil, &code, &desc
	}

	if resp.ResultCode == 200 || resp.ResultCode == 202 {
		var serverID *string
		if resp.MessageID != nil && strings.TrimSpace(*resp.MessageID) != "" {
			id := strings.TrimSpace(*resp.MessageID)
			serverID = &id
		} else if resp.RequestID != nil {
			id := strconv.FormatInt(*resp.RequestID, 10)
			serverID = &id
		}
		return models.SplusSendStatusSuccessful, 1, serverID, nil, nil
	}

	code := strconv.Itoa(resp.ResultCode)
	desc := strings.TrimSpace(resp.ResultMessage)
	if desc == "" {
		if msg, ok := splusErrorDescriptions[resp.ResultCode]; ok {
			desc = msg
		}
	}
	return models.SplusSendStatusUnsuccessful, 0, nil, &code, &desc
}

func isRetryableSplusError(resp *SplusResponse, err error) bool {
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "429") || strings.Contains(msg, "too many requests") {
			return true
		}
	}

	if resp == nil {
		return false
	}

	if resp.HTTPStatus == http.StatusTooManyRequests {
		return true
	}
	if resp.HTTPStatus >= 500 && resp.HTTPStatus <= 599 {
		return true
	}

	switch resp.ResultCode {
	case 429, 500, 724, 730, 736, 738:
		return true
	default:
		return false
	}
}

func (s *SplusCampaignScheduler) updateProcessedCampaignStatsFromSentRows(ctx context.Context, processedCampaignID uint) (map[string]any, error) {
	type row struct {
		Total      int64
		Successful int64
	}
	var agg row
	if err := s.db.WithContext(ctx).Table("sent_splus_messages").
		Select(`
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status = 'successful' THEN 1 ELSE 0 END), 0) AS successful`).
		Where("processed_campaign_id = ?", processedCampaignID).
		Scan(&agg).Error; err != nil {
		return nil, err
	}

	pc, err := s.pcRepo.ByID(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, fmt.Errorf("processed campaign not found for processed_campaign_id=%d", processedCampaignID)
	}

	stats := map[string]any{
		"aggregatedTotalRecords":          agg.Total,
		"aggregatedTotalSent":             agg.Successful,
		"aggregatedTotalParts":            agg.Successful,
		"aggregatedTotalDeliveredParts":   agg.Successful,
		"aggregatedTotalUnDeliveredParts": agg.Total - agg.Successful,
		"aggregatedTotalUnKnownParts":     int64(0),
		"updatedAt":                       utils.UTCNow().Format(time.RFC3339),
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}
	pc.Statistics = data
	pc.UpdatedAt = utils.UTCNow()
	if err := s.pcRepo.Update(ctx, pc); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *SplusCampaignScheduler) validateSplusCampaign(c dto.BotGetCampaignResponse) error {
	if c.Status != string(models.CampaignStatusApproved) {
		return fmt.Errorf("campaign status is not approved")
	}
	now := utils.UTCNow()
	if c.ScheduleAt != nil && c.ScheduleAt.After(now) {
		return fmt.Errorf("campaign schedule_at is after now")
	}
	if c.CreatedAt.After(now) {
		return fmt.Errorf("campaign created_at is after now")
	}
	if c.UpdatedAt != nil && c.UpdatedAt.After(now) {
		return fmt.Errorf("campaign updated_at is after now")
	}
	if _, err := extractSplusBotID(c); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformSPlus {
		return fmt.Errorf("campaign platform is not splus")
	}
	return nil
}

func extractSplusBotID(c dto.BotGetCampaignResponse) (string, error) {
	if c.PlatformSettings == nil {
		return "", fmt.Errorf("campaign platform_settings is missing")
	}
	if c.PlatformSettings.Metadata == nil {
		return "", fmt.Errorf("campaign platform_settings.metadata is missing")
	}
	raw, ok := c.PlatformSettings.Metadata["splus_bot_id"]
	if !ok {
		return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id is missing")
	}

	switch v := raw.(type) {
	case int:
		if v <= 0 {
			return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id must be positive")
		}
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		if v <= 0 {
			return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id must be positive")
		}
		return strconv.FormatInt(v, 10), nil
	case float64:
		if v <= 0 || v != float64(int64(v)) {
			return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id must be a positive integer")
		}
		return strconv.FormatInt(int64(v), 10), nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id must not be empty")
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id must be a positive integer")
		}
		return strconv.FormatInt(id, 10), nil
	case json.Number:
		id, err := v.Int64()
		if err != nil || id <= 0 {
			return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id must be a positive integer")
		}
		return strconv.FormatInt(id, 10), nil
	default:
		return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id has unsupported type %T", raw)
	}
}

func (s *SplusCampaignScheduler) fetchSplusAudiencePhones(ctx context.Context, c dto.BotGetCampaignResponse, token string, correlationID string) ([]string, []int64, []string, uint, error) {
	numAudiences := int64(0)
	if c.NumAudiences != nil {
		numAudiences = int64(*c.NumAudiences)
	}
	if numAudiences <= 0 {
		return nil, nil, nil, 0, fmt.Errorf("campaign num_audiences must be positive")
	}

	toExtract := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			s.logger.Printf("fetchSplusAudiencePhones tag parse failed: campaign_id=%d tag=%q err=%v", c.ID, tag, err)
			return nil, nil, nil, 0, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhones tags lookup failed: campaign_id=%d err=%v", c.ID, err)
		return nil, nil, nil, 0, err
	}

	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}

	// NOTE: len(tagIDs) <= len(c.Tags) because some tags may not be found or are inactive

	const limit = 10000000

	tagsHash := splusHashTags(c.Tags)
	selection, err := s.audienceCache.Latest(ctx, c.CustomerID, tagsHash)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhones latest selection failed: campaign_id=%d customer_id=%d tags_hash=%s err=%v", c.ID, c.CustomerID, tagsHash, err)
		return nil, nil, nil, 0, err
	}
	if selection != nil {
		s.logger.Printf("fetchSplusAudiencePhones selection hit: campaign_id=%d selection_id=%d prior_ids_length=%d", c.ID, selection.ID, len(selection.IDs))
	} else {
		s.logger.Printf("fetchSplusAudiencePhones selection miss: campaign_id=%d", c.ID)
	}

	selectAudiences := func(exclude map[int64]struct{}) ([]string, []int64, error) {
		phones := make([]string, 0, numAudiences)
		ids := make([]int64, 0, numAudiences)

		filter := models.AudienceProfileFilter{Tags: &tagIDs, Color: utils.ToPtr("white")}
		whites, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
		if err != nil {
			s.logger.Printf("fetchSplusAudiencePhones fetch white failed: campaign_id=%d err=%v", c.ID, err)
			return nil, nil, err
		}
		s.logger.Printf("fetchSplusAudiencePhones white candidates: campaign_id=%d count=%d", c.ID, len(whites))

		appendIfFresh := func(ap *models.AudienceProfile) {
			if ap == nil || ap.PhoneNumber == nil || *ap.PhoneNumber == "" {
				return
			}
			if exclude != nil {
				if _, ok := exclude[int64(ap.ID)]; ok {
					return
				}
			}
			phones = append(phones, *ap.PhoneNumber)
			ids = append(ids, int64(ap.ID))
		}

		for _, ap := range whites {
			if int64(len(phones)) >= numAudiences {
				break
			}
			appendIfFresh(ap)
		}

		if int64(len(phones)) < numAudiences {
			filter := models.AudienceProfileFilter{Tags: &tagIDs, Color: utils.ToPtr("pink")}
			pinks, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
			if err != nil {
				s.logger.Printf("fetchSplusAudiencePhones fetch pink failed: campaign_id=%d err=%v", c.ID, err)
				return nil, nil, err
			}
			s.logger.Printf("fetchSplusAudiencePhones pink candidates: campaign_id=%d count=%d", c.ID, len(pinks))
			for _, ap := range pinks {
				if int64(len(phones)) >= numAudiences {
					break
				}
				appendIfFresh(ap)
			}
		}

		return phones, ids, nil
	}

	// First attempt excluding prior picks for this customer/tags
	var exclude map[int64]struct{}
	if selection != nil && selection.IDs != nil {
		exclude = selection.IDs
	}
	phones, ids, err := selectAudiences(exclude)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	s.logger.Printf("fetchSplusAudiencePhones selected (with exclusions): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), c.NumAudiences)

	resetUsed := false
	if int64(len(phones)) < numAudiences {
		// Not enough fresh; retry from scratch without exclusions
		resetUsed = true
		phones, ids, err = selectAudiences(nil)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		s.logger.Printf("fetchSplusAudiencePhones selected (reset): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), c.NumAudiences)
	}

	// Persist selection history with correlation id and merged audience IDs
	var sel *AudienceSelection
	if resetUsed {
		sel, err = s.audienceCache.SaveSnapshot(ctx, c.CustomerID, tagsHash, correlationID, ids)
	} else {
		sel, err = s.audienceCache.SaveWithMerge(ctx, c.CustomerID, tagsHash, correlationID, ids)
	}
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhones selection save failed: campaign_id=%d err=%v reset=%t", c.ID, err, resetUsed)
		return nil, nil, nil, 0, err
	}
	s.logger.Printf("fetchSplusAudiencePhones selection saved: campaign_id=%d selection_id=%d reset=%t selected=%d", c.ID, sel.ID, resetUsed, len(ids))

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchSplusAudiencePhones skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		s.logger.Printf("fetchSplusAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d ad_link=empty", c.ID, len(phones), len(phones), sel.ID)
		return phones, ids, make([]string, len(phones)), sel.ID, nil
	}

	// Generate sequential UIDs via bot API and persist short links centrally
	codes, err := s.botClient.AllocateShortLinks(ctx, token, c.ID, c.AdLink, phones)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhones allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, nil, nil, 0, err
	}
	s.logger.Printf("fetchSplusAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d", c.ID, len(phones), len(codes), sel.ID)
	return phones, ids, codes, sel.ID, nil
}

func splusHashTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	cp := make([]string, len(tags))
	copy(cp, tags)
	sort.Strings(cp)
	h := sha1.Sum([]byte(strings.Join(cp, ",")))
	return hex.EncodeToString(h[:])
}

func (s *SplusCampaignScheduler) buildSplusMessageBody(c dto.BotGetCampaignResponse, code string) string {
	content := ""
	if c.Content != nil {
		content = *c.Content
	}
	if hasCampaignAdLink(c.AdLink) {
		shortened := "jo1n.ir/" + code
		return strings.ReplaceAll(content, "🔗", shortened)
	}
	return strings.ReplaceAll(content, "🔗", "")
}

func (s *SplusCampaignScheduler) notifyAdmin(message string) {
	if s.notifier == nil {
		return
	}
	go func(msg string) {
		for _, mobile := range s.adminCfg.ActiveMobiles() {
			_ = s.notifier.SendSMS(context.Background(), mobile, msg, nil)
		}
	}(message)
}

var splusErrorDescriptions = map[int]string{
	400: "Bad Request",
	401: "Unauthorized",
	404: "Not Found",
	429: "Too Many Requests",
	470: "Input Validation Error",
	500: "Internal Server Error",
	700: "Invalid Phone Number",
	701: "No User Account",
	702: "Suspended User",
	712: "User Is Inactive",
	715: "User Type Not Authorized",
	716: "File Not Found",
	723: "Insufficient Balance",
	724: "Cannot Download File",
	726: "File Invalid",
	727: "File Size Invalid",
	728: "File Name Invalid",
	729: "Mime Type Invalid",
	730: "Cannot Create User",
	731: "No Active Conversation",
	732: "Receiver Not Specified",
	733: "URL Not Allowed",
	734: "APK Not Allowed",
	735: "Sender Is Blocked",
	736: "Server Connection Error",
	738: "Data Persist Error",
	739: "Request Not Allowed",
	740: "File Access Denied",
}
