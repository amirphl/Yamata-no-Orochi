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
	defaultRubikaBaseURL = "https://messaging.rubika.ir"
	rubikaSendMaxRetries = 5
)

type RubikaCampaignScheduler struct {
	audRepo      repository.AudienceProfileRepository
	tagRepo      repository.TagRepository
	pcRepo       repository.ProcessedCampaignRepository
	notifier     NotificationSender
	logger       *log.Logger
	interval     time.Duration
	messageDelay time.Duration

	db        *gorm.DB
	adminCfg  config.AdminConfig
	rubikaCfg config.RubikaConfig
	botCfg    config.BotConfig

	botClient    BotClient
	rubikaClient RubikaClient

	logFile *os.File

	schedulerName string

	audienceCache *AudienceCache
}

type RubikaClient interface {
	SendBulkMessages(ctx context.Context, serviceID string, messages []RubikaMessagePayload) (*RubikaSendBulkMessagesResponse, error)
	UploadFile(ctx context.Context, path string) (*RubikaUploadFileResponse, error)
}

type RubikaMessagePayload struct {
	Phone  string  `json:"phone"`
	Text   string  `json:"text"`
	FileID *string `json:"file_id,omitempty"`
}

type rubikaAPICallRequest struct {
	Method     string `json:"method"`
	Data       any    `json:"data"`
	APIVersion int    `json:"api_version"`
}

type rubikaSendBulkMessagesData struct {
	ServiceID   string                 `json:"service_id"`
	MessageList []RubikaMessagePayload `json:"message_list"`
}

type RubikaMessageStatus struct {
	MessageID string `json:"message_id,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Status    string `json:"status,omitempty"`
	StatusDet string `json:"status_det,omitempty"`
}

type RubikaSendBulkMessagesResponse struct {
	Status     string `json:"status,omitempty"`
	StatusDet  string `json:"status_det,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Data       struct {
		MessageStatusList []RubikaMessageStatus `json:"message_status_list,omitempty"`
	} `json:"data"`
}

type RubikaUploadFileResponse struct {
	Status     string `json:"status,omitempty"`
	StatusDet  string `json:"status_det,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Data       struct {
		FileID string `json:"file_id,omitempty"`
	} `json:"data"`
}

type httpRubikaClient struct {
	cfg    config.RubikaConfig
	client *http.Client
}

func newHTTPRubikaClient(cfg config.RubikaConfig) *httpRubikaClient {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultRubikaBaseURL
	}
	cfg.BaseURL = baseURL
	return &httpRubikaClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *httpRubikaClient) SendBulkMessages(ctx context.Context, serviceID string, messages []RubikaMessagePayload) (*RubikaSendBulkMessagesResponse, error) {
	if strings.TrimSpace(c.cfg.Token) == "" {
		return nil, fmt.Errorf("rubika token is not configured")
	}
	if strings.TrimSpace(serviceID) == "" {
		return nil, fmt.Errorf("rubika service_id is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("rubika message_list is empty")
	}

	reqBody := rubikaAPICallRequest{
		Method: "sendBulkMessages",
		Data: rubikaSendBulkMessagesData{
			ServiceID:   serviceID,
			MessageList: messages,
		},
		APIVersion: 1,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("token", c.cfg.Token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out RubikaSendBulkMessagesResponse
	out.HTTPStatus = resp.StatusCode

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rubika sendBulkMessages http status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode rubika sendBulkMessages response: %w", err)
	}

	return &out, nil
}

func (c *httpRubikaClient) UploadFile(ctx context.Context, path string) (*RubikaUploadFileResponse, error) {
	if strings.TrimSpace(c.cfg.Token) == "" {
		return nil, fmt.Errorf("rubika token is not configured")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/uploadFile", buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", c.cfg.Token)
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

	var out RubikaUploadFileResponse
	out.HTTPStatus = resp.StatusCode

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rubika uploadFile http status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode rubika uploadFile response: %w", err)
		}
	}
	if strings.TrimSpace(out.Data.FileID) == "" {
		return nil, fmt.Errorf("rubika upload returned empty file_id")
	}

	return &out, nil
}

func NewRubikaCampaignScheduler(
	audRepo repository.AudienceProfileRepository,
	tagRepo repository.TagRepository,
	pcRepo repository.ProcessedCampaignRepository,
	notifier NotificationSender,
	db *gorm.DB,
	logger *log.Logger,
	interval time.Duration,
	messageDelay time.Duration,
	rubikaCfg config.RubikaConfig,
	botCfg config.BotConfig,
	adminCfg config.AdminConfig,
) *RubikaCampaignScheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	if botCfg.APIDomain == "" {
		botCfg.APIDomain = defaultBotAPIDomain
	}

	s := &RubikaCampaignScheduler{
		audRepo:       audRepo,
		tagRepo:       tagRepo,
		pcRepo:        pcRepo,
		notifier:      notifier,
		logger:        logger,
		db:            db,
		interval:      interval,
		messageDelay:  messageDelay,
		adminCfg:      adminCfg,
		rubikaCfg:     rubikaCfg,
		botCfg:        botCfg,
		botClient:     newHTTPBotClient(botCfg),
		rubikaClient:  newHTTPRubikaClient(rubikaCfg),
		audienceCache: NewAudienceCache(repository.NewAudienceSelectionRepository(db)),
		schedulerName: "rubika",
	}

	if err := s.initSchedulerLogger(); err != nil {
		s.logger = log.New(io.Discard, "rubika_scheduler ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		s.logger.Printf("rubika scheduler: failed to initialize file logger: %v", err)
	}
	return s
}

func (s *RubikaCampaignScheduler) initSchedulerLogger() error {
	l, f, err := initSchedulerLogger(s.schedulerName + "_scheduler")
	if err != nil {
		return err
	}
	s.logFile = f
	s.logger = l
	return nil
}

func (s *RubikaCampaignScheduler) Start(parent context.Context) func() {
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

func (s *RubikaCampaignScheduler) runOnce(ctx context.Context) {
	token, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("rubika scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Rubika scheduler bot login failed: %v", err))
		return
	}

	ready, err := s.botClient.ListReadyCampaigns(ctx, token, models.CampaignPlatformRubika)
	if err != nil {
		s.logger.Printf("rubika scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Rubika scheduler list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}

	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformRubika {
			continue
		}
		if err := s.validateRubikaCampaign(c); err != nil {
			s.logger.Printf("rubika scheduler: validate campaign failed for id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Rubika scheduler validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("rubika scheduler: check processed failed for id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Rubika scheduler check processed failed for id=%d: %v", c.ID, err))
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
			if err := s.processRubikaCampaign(ctx, token, c); err != nil {
				s.logger.Printf("rubika scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("Rubika scheduler process campaign failed for id=%d: %v", c.ID, err))
			}
		}()
	}
}

func (s *RubikaCampaignScheduler) processRubikaCampaign(ctx context.Context, token string, c dto.BotGetCampaignResponse) error {
	serviceID, err := s.extractRubikaServiceID(c)
	if err != nil {
		return err
	}

	if err := s.botClient.MoveCampaignToRunning(ctx, token, c.ID); err != nil {
		return fmt.Errorf("move to running: %w", err)
	}

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

		var selectionID uint
		correlationID := uuid.NewString()
		phones, ids, uids, selectionID, err = s.fetchRubikaAudiencePhones(txCtx, c, token, correlationID)
		if err != nil {
			return err
		}

		pc.AudienceIDs = pq.Int64Array(ids)
		pc.AudienceCodes = uids
		pc.AudienceSelectionID = utils.ToPtr(selectionID)
		pc.UpdatedAt = utils.UTCNow()
		return s.pcRepo.Update(txCtx, pc)
	}); err != nil {
		return err
	}

	fileID, err := s.uploadCampaignMedia(ctx, token, c)
	if err != nil {
		return err
	}

	var (
		totalRecords int64
		totalSent    int64
	)

	for i, phone := range phones {
		msgs := []RubikaMessagePayload{
			{
				Phone:  phone,
				Text:   s.buildRubikaMessageBody(c, uids[i]),
				FileID: fileID,
			},
		}

		resp, sendErr := s.sendWithRetry(ctx, serviceID, msgs)
		successCount := countRubikaSuccessfulSends(resp, sendErr, 1)
		totalRecords++
		totalSent += int64(successCount)

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

	stats := map[string]any{
		"aggregatedTotalRecords":          totalRecords,
		"aggregatedTotalSent":             totalSent,
		"aggregatedTotalParts":            totalSent,
		"aggregatedTotalDeliveredParts":   totalSent,
		"aggregatedTotalUnDeliveredParts": totalRecords - totalSent,
		"aggregatedTotalUnKnownParts":     int64(0),
		"updatedAt":                       utils.UTCNow().Format(time.RFC3339),
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	pc.Statistics = data
	pc.UpdatedAt = utils.UTCNow()
	if err := s.pcRepo.Update(ctx, pc); err != nil {
		return err
	}

	if err := s.botClient.PushCampaignStatistics(ctx, c.ID, stats); err != nil {
		return err
	}
	if err := s.botClient.MoveCampaignToExecuted(ctx, token, c.ID); err != nil {
		return err
	}

	return nil
}

func (s *RubikaCampaignScheduler) uploadCampaignMedia(ctx context.Context, token string, c dto.BotGetCampaignResponse) (*string, error) {
	if c.MediaUUID == nil {
		return nil, nil
	}

	path, err := s.downloadCampaignMediaViaBotAPI(ctx, token, c.MediaUUID.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(path) }()

	resp, err := s.rubikaClient.UploadFile(ctx, path)
	if err != nil {
		return nil, err
	}
	fileID := strings.TrimSpace(resp.Data.FileID)
	if fileID == "" {
		return nil, fmt.Errorf("rubika upload returned empty file_id")
	}
	return &fileID, nil
}

func (s *RubikaCampaignScheduler) downloadCampaignMediaViaBotAPI(ctx context.Context, token, mediaUUID string) (string, error) {
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

	tmpFile, err := os.CreateTemp("", "rubika-media-*")
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

func (s *RubikaCampaignScheduler) validateRubikaCampaign(c dto.BotGetCampaignResponse) error {
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
	if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformRubika {
		return fmt.Errorf("campaign platform is not rubika")
	}
	if strings.TrimSpace(s.rubikaCfg.Token) == "" {
		return fmt.Errorf("rubika token is not configured")
	}
	_, err := s.extractRubikaServiceID(c)
	return err
}

func (s *RubikaCampaignScheduler) extractRubikaServiceID(c dto.BotGetCampaignResponse) (string, error) {
	parseRaw := func(raw any) (string, bool) {
		switch v := raw.(type) {
		case string:
			x := strings.TrimSpace(v)
			if x != "" {
				return x, true
			}
		case int:
			if v > 0 {
				return strconv.Itoa(v), true
			}
		case int64:
			if v > 0 {
				return strconv.FormatInt(v, 10), true
			}
		case float64:
			if v > 0 {
				if v == float64(int64(v)) {
					return strconv.FormatInt(int64(v), 10), true
				}
				return strconv.FormatFloat(v, 'f', -1, 64), true
			}
		case json.Number:
			x := strings.TrimSpace(v.String())
			if x != "" {
				return x, true
			}
		}
		return "", false
	}

	if c.PlatformSettings != nil && c.PlatformSettings.Metadata != nil {
		if raw, ok := c.PlatformSettings.Metadata["rubika_service_id"]; ok {
			if serviceID, ok := parseRaw(raw); ok {
				return serviceID, nil
			}
			return "", fmt.Errorf("campaign platform_settings.metadata.rubika_service_id is invalid")
		}
	}

	serviceID := strings.TrimSpace(s.rubikaCfg.ServiceID)
	if serviceID == "" {
		return "", fmt.Errorf("rubika service_id is not configured")
	}
	return serviceID, nil
}

func (s *RubikaCampaignScheduler) sendWithRetry(ctx context.Context, serviceID string, messages []RubikaMessagePayload) (*RubikaSendBulkMessagesResponse, error) {
	var (
		resp *RubikaSendBulkMessagesResponse
		err  error
	)
	for attempt := 0; attempt <= rubikaSendMaxRetries; attempt++ {
		resp, err = s.rubikaClient.SendBulkMessages(ctx, serviceID, messages)
		if !isRubikaRateLimit(err, resp) {
			return resp, err
		}
		if attempt == rubikaSendMaxRetries {
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

func isRubikaRateLimit(err error, resp *RubikaSendBulkMessagesResponse) bool {
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "ratelimit") {
			return true
		}
	}
	if resp == nil {
		return false
	}
	lowStatus := strings.ToLower(strings.TrimSpace(resp.Status))
	lowDetail := strings.ToLower(strings.TrimSpace(resp.StatusDet))
	return strings.Contains(lowStatus, "ratelimit") ||
		strings.Contains(lowStatus, "rate_limit") ||
		strings.Contains(lowDetail, "ratelimit") ||
		strings.Contains(lowDetail, "rate limit")
}

func countRubikaSuccessfulSends(resp *RubikaSendBulkMessagesResponse, err error, attempted int) int {
	if err != nil || resp == nil {
		return 0
	}
	if len(resp.Data.MessageStatusList) == 0 {
		return 0
	}
	success := 0
	for _, st := range resp.Data.MessageStatusList {
		if rubikaMessageStatusSuccessful(st) {
			success++
		}
	}
	if success > attempted {
		return attempted
	}
	return success
}

func rubikaMessageStatusSuccessful(st RubikaMessageStatus) bool {
	status := strings.ToLower(strings.TrimSpace(st.Status))
	statusDet := strings.ToLower(strings.TrimSpace(st.StatusDet))
	if strings.TrimSpace(st.MessageID) != "" {
		return true
	}
	if status == "ok" || status == "success" || status == "sent" || status == "queued" {
		return true
	}
	if strings.Contains(statusDet, "success") || strings.Contains(statusDet, "sent") {
		return true
	}
	return false
}

func (s *RubikaCampaignScheduler) fetchRubikaAudiencePhones(ctx context.Context, c dto.BotGetCampaignResponse, token string, correlationID string) ([]string, []int64, []string, uint, error) {
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
			return nil, nil, nil, 0, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}

	const limit = 10000000
	tagsHash := rubikaHashTags(c.Tags)
	selection, err := s.audienceCache.Latest(ctx, c.CustomerID, tagsHash)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	selectAudiences := func(exclude map[int64]struct{}) ([]string, []int64, error) {
		phones := make([]string, 0, numAudiences)
		ids := make([]int64, 0, numAudiences)

		filter := models.AudienceProfileFilter{Tags: &tagIDs, Color: utils.ToPtr("white")}
		whites, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
		if err != nil {
			return nil, nil, err
		}

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
				return nil, nil, err
			}
			for _, ap := range pinks {
				if int64(len(phones)) >= numAudiences {
					break
				}
				appendIfFresh(ap)
			}
		}

		return phones, ids, nil
	}

	var exclude map[int64]struct{}
	if selection != nil && selection.IDs != nil {
		exclude = selection.IDs
	}
	phones, ids, err := selectAudiences(exclude)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	resetUsed := false
	if int64(len(phones)) < numAudiences {
		resetUsed = true
		phones, ids, err = selectAudiences(nil)
		if err != nil {
			return nil, nil, nil, 0, err
		}
	}

	var sel *AudienceSelection
	if resetUsed {
		sel, err = s.audienceCache.SaveSnapshot(ctx, c.CustomerID, tagsHash, correlationID, ids)
	} else {
		sel, err = s.audienceCache.SaveWithMerge(ctx, c.CustomerID, tagsHash, correlationID, ids)
	}
	if err != nil {
		return nil, nil, nil, 0, err
	}

	if !hasCampaignAdLink(c.AdLink) {
		return phones, ids, make([]string, len(phones)), sel.ID, nil
	}

	codes, err := s.botClient.AllocateShortLinks(ctx, token, c.ID, c.AdLink, phones)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	return phones, ids, codes, sel.ID, nil
}

func rubikaHashTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	cp := make([]string, len(tags))
	copy(cp, tags)
	sort.Strings(cp)
	h := sha1.Sum([]byte(strings.Join(cp, ",")))
	return hex.EncodeToString(h[:])
}

func (s *RubikaCampaignScheduler) buildRubikaMessageBody(c dto.BotGetCampaignResponse, code string) string {
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

func (s *RubikaCampaignScheduler) notifyAdmin(message string) {
	if s.notifier == nil {
		return
	}
	go func(msg string) {
		for _, mobile := range s.adminCfg.ActiveMobiles() {
			_ = s.notifier.SendSMS(context.Background(), mobile, msg, nil)
		}
	}(message)
}
