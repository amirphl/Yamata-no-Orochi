package scheduler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

// TODO: Tx management in queries

const (
	baleSendMaxRetries = 5
	baleSendBatchSize  = 100
)

type BaleCampaignScheduler struct {
	audRepo      repository.AudienceProfileRepository
	tagRepo      repository.TagRepository
	sentRepo     repository.SentBaleMessageRepository
	pcRepo       repository.ProcessedCampaignRepository
	notifier     NotificationSender
	logger       *log.Logger
	interval     time.Duration
	messageDelay time.Duration

	db       *gorm.DB
	adminCfg config.AdminConfig
	baleCfg  config.BaleConfig
	botCfg   config.BotConfig

	botClient  BotClient
	baleClient BaleClient

	logFile *os.File

	schedulerName string

	audienceCache *AudienceCache
}

func NewBaleCampaignScheduler(
	audRepo repository.AudienceProfileRepository,
	tagRepo repository.TagRepository,
	sentRepo repository.SentBaleMessageRepository,
	pcRepo repository.ProcessedCampaignRepository,
	notifier NotificationSender,
	db *gorm.DB,
	logger *log.Logger,
	interval time.Duration,
	messageDelay time.Duration,
	baleCfg config.BaleConfig,
	botCfg config.BotConfig,
	adminCfg config.AdminConfig,
) *BaleCampaignScheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	if botCfg.APIDomain == "" {
		botCfg.APIDomain = defaultBotAPIDomain
	}

	s := &BaleCampaignScheduler{
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
		baleCfg:       baleCfg,
		botCfg:        botCfg,
		botClient:     newHTTPBotClient(botCfg),
		baleClient:    newHTTPBaleClient(baleCfg),
		audienceCache: NewAudienceCache(repository.NewAudienceSelectionRepository(db)),
		schedulerName: "bale",
	}

	if err := s.initSchedulerLogger(); err != nil {
		s.logger = log.New(io.Discard, "bale_scheduler ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		s.logger.Printf("bale scheduler: failed to initialize file logger: %v", err)
	}
	return s
}

func (s *BaleCampaignScheduler) initSchedulerLogger() error {
	l, f, err := initSchedulerLogger(s.schedulerName + "_scheduler")
	if err != nil {
		return err
	}
	s.logFile = f
	s.logger = l
	return nil
}

func (s *BaleCampaignScheduler) Start(parent context.Context) func() {
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

func (s *BaleCampaignScheduler) runOnce(ctx context.Context) {
	token, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("bale scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Bale scheduler bot login failed: %v", err))
		return
	}

	ready, err := s.botClient.ListReadyCampaigns(ctx, token, models.CampaignPlatformBale)
	if err != nil {
		s.logger.Printf("bale scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Bale scheduler list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}
	s.logger.Printf("bale scheduler: listed %d ready campaigns", len(ready))

	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformBale {
			// NOTE: Just skip.
			continue
		}
		if err := s.validateBaleCampaign(c); err != nil {
			s.logger.Printf("bale scheduler: validate campaign failed for campaign id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Bale scheduler validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("bale scheduler: check processed failed for campaign id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Bale scheduler check processed failed for id=%d: %v", c.ID, err))
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		}
	}
	if len(pending) == 0 {
		return
	}
	s.logger.Printf("bale scheduler: %d campaigns pending processing...", len(pending))

	for _, camp := range pending {
		c := camp
		go func() {
			if err := s.processBaleCampaign(ctx, token, c); err != nil {
				s.logger.Printf("bale scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("Bale scheduler process campaign failed for campaign id=%d: %v", c.ID, err))
			}
		}()
	}
	// Do not wait to keep scheduler loop non-blocking; optionally wait if desired
	// wg.Wait()
}

func (s *BaleCampaignScheduler) processBaleCampaign(ctx context.Context, token string, c dto.BotGetCampaignResponse) error {
	botID, err := extractBaleBotID(c)
	if err != nil {
		return fmt.Errorf("resolve bale bot id: %w", err)
	}

	if err := s.botClient.MoveCampaignToRunning(ctx, token, c.ID); err != nil {
		return fmt.Errorf("move to running: %w", err)
	}
	s.logger.Printf("bale scheduler: campaign id=%d moved to running", c.ID)

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
		s.logger.Printf("bale scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

		// Fetch audiences (white then pink, DB-shuffled), and sort order is enforced inside repo
		var (
			selectionID uint
			err         error
		)
		correlationID := uuid.NewString()
		phones, ids, uids, selectionID, err = s.fetchBaleAudiencePhones(txCtx, c, token, correlationID)
		if err != nil {
			return err
		}
		s.logger.Printf("bale scheduler: fetched %d audience phones for campaign id=%d", len(phones), c.ID)

		pc.AudienceIDs = pq.Int64Array(ids)
		pc.AudienceCodes = uids
		pc.AudienceSelectionID = utils.ToPtr(selectionID)
		pc.UpdatedAt = utils.UTCNow()
		if err := s.pcRepo.Update(txCtx, pc); err != nil {
			return err
		}
		s.logger.Printf("bale scheduler: updated processed campaign id=%d with audience ids", pc.ID)

		return nil
	}); err != nil {
		return err
	}
	s.logger.Printf("bale scheduler: persisted processed campaign id=%d num_audiences=%d", pc.ID, len(ids))

	var fileID *string
	if c.MediaUUID != nil {
		id, err := s.uploadCampaignMedia(ctx, token, c)
		if err != nil {
			return err
		}
		fileID = id
	}

	for start := 0; start < len(phones); start += baleSendBatchSize {
		end := min(start+baleSendBatchSize, len(phones))
		batchPhones := phones[start:end]
		batchIDs := ids[start:end]
		batchUIDs := uids[start:end]

		rows := make([]*models.SentBaleMessage, 0, len(batchPhones))
		// Build Bale message rows from response and save them
		trackingIDs := make([]string, 0, len(batchPhones))
		for _, p := range batchPhones {
			trackingID := uuid.NewString()
			trackingIDs = append(trackingIDs, trackingID)
			rows = append(rows, &models.SentBaleMessage{
				ProcessedCampaignID: pc.ID,
				PhoneNumber:         p,
				PartsDelivered:      0,
				Status:              models.BaleSendStatusPending,
				TrackingID:          trackingID,
			})
		}

		if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
			if len(rows) > 0 {
				if err := s.sentRepo.SaveBatch(txCtx, rows); err != nil {
					return err
				}
			}
			lastBatchID := batchIDs[len(batchIDs)-1]
			pc.LastAudienceID = &lastBatchID
			pc.UpdatedAt = utils.UTCNow()
			if err := s.pcRepo.Update(txCtx, pc); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return err
		}

		for i, phone := range batchPhones {
			body := s.buildBaleMessageBody(c, batchUIDs[i])
			req := &BaleSendMessageRequest{
				RequestID:   trackingIDs[i],
				BotID:       botID,
				PhoneNumber: phone,
				MessageData: BaleSendMessageData{
					Message: &BaleMessage{
						Text:   body,
						FileID: fileID,
					},
				},
			}
			resp, sendErr := s.sendWithRetry(ctx, req)
			if err := s.persistBaleSendResult(ctx, trackingIDs[i], resp, sendErr); err != nil {
				s.logger.Printf("bale scheduler: persist send result failed for campaign id=%d tracking_id=%s: %v", c.ID, trackingIDs[i], err)
				// TODO: How to handle this error? Retry sending? Skip to next batch?
			}
			if start+i < len(phones)-1 {
				if err := sleepWithContext(ctx, s.messageDelay); err != nil {
					return err
				}
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

	s.logger.Printf("bale scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, token, c.ID); err != nil {
		s.logger.Printf("bale scheduler: move executed failed for campaign id=%d: %v", c.ID, err)
		return err
	}
	s.logger.Printf("bale scheduler: campaign id=%d moved to executed", c.ID)

	return nil
}

func (s *BaleCampaignScheduler) uploadCampaignMedia(ctx context.Context, token string, c dto.BotGetCampaignResponse) (*string, error) {
	if c.MediaUUID == nil {
		return nil, nil
	}

	path, err := s.downloadCampaignMediaViaBotAPI(ctx, token, c.MediaUUID.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(path) }()

	resp, err := s.baleClient.UploadFile(ctx, path)
	if err != nil {
		return nil, err
	}
	return &resp.FileID, nil
}

func (s *BaleCampaignScheduler) downloadCampaignMediaViaBotAPI(ctx context.Context, token, mediaUUID string) (string, error) {
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

	tmpFile, err := os.CreateTemp("", "bale-media-*")
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

func (s *BaleCampaignScheduler) persistBaleSendResult(ctx context.Context, trackingID string, resp *BaleSendMessageResponse, sendErr error) error {
	if sendErr != nil {
		code := "SEND_FAILED"
		desc := sendErr.Error()
		return s.sentRepo.UpdateSendResultByTrackingID(
			ctx,
			trackingID,
			models.BaleSendStatusUnsuccessful,
			0,
			nil,
			&code,
			&desc,
		)
	}

	if resp != nil && len(resp.ErrorData) > 0 {
		first := resp.ErrorData[0]
		code := first.CodeString()
		desc := marshalBaleErrorSpec(resp.ErrorData)
		return s.sentRepo.UpdateSendResultByTrackingID(
			ctx,
			trackingID,
			models.BaleSendStatusUnsuccessful,
			0,
			nil,
			&code,
			&desc,
		)
	}

	var serverID *string
	if resp != nil && strings.TrimSpace(resp.MessageID) != "" {
		id := strings.TrimSpace(resp.MessageID)
		serverID = &id
	}
	return s.sentRepo.UpdateSendResultByTrackingID(
		ctx,
		trackingID,
		models.BaleSendStatusSuccessful,
		1,
		serverID,
		nil,
		nil,
	)
}

func (s *BaleCampaignScheduler) sendWithRetry(ctx context.Context, req *BaleSendMessageRequest) (*BaleSendMessageResponse, error) {
	var (
		resp *BaleSendMessageResponse
		err  error
	)
	for attempt := 0; attempt <= baleSendMaxRetries; attempt++ {
		resp, err = s.baleClient.SendMessage(ctx, req)
		if !isBaleRateLimit(err, resp) {
			return resp, err
		}
		if attempt == baleSendMaxRetries {
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

func isBaleRateLimit(err error, resp *BaleSendMessageResponse) bool {
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "429") ||
			strings.Contains(msg, "ratelimit") ||
			strings.Contains(msg, "rate limit") ||
			strings.Contains(msg, "RateLimitExceeded") {
			return true
		}
	}
	if resp == nil {
		return false
	}
	for _, e := range resp.ErrorData {
		code := strings.ToLower(strings.TrimSpace(e.CodeString()))
		desc := strings.ToLower(strings.TrimSpace(e.Description))
		if strings.Contains(code, "ratelimit") ||
			strings.Contains(code, "rate_limit") ||
			strings.Contains(code, "rate limit") ||
			strings.Contains(code, "RateLimitExceeded") {
			return true
		}
		if strings.Contains(desc, "ratelimit") ||
			strings.Contains(desc, "rate limit") ||
			strings.Contains(desc, "RateLimitExceeded") {
			return true
		}
	}
	return false
}

func marshalBaleErrorSpec(spec []BaleErrorData) string {
	if len(spec) == 0 {
		return ""
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return fmt.Sprintf("%v", spec)
	}
	return string(b)
}

func (s *BaleCampaignScheduler) updateProcessedCampaignStatsFromSentRows(ctx context.Context, processedCampaignID uint) (map[string]any, error) {
	type row struct {
		Total      int64
		Successful int64
	}
	var agg row
	if err := s.db.WithContext(ctx).Table("sent_bale_messages").
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

func (s *BaleCampaignScheduler) validateBaleCampaign(c dto.BotGetCampaignResponse) error {
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
	if strings.TrimSpace(s.baleCfg.APIAccessKey) == "" {
		return fmt.Errorf("bale api-access-key is not configured")
	}
	if _, err := extractBaleBotID(c); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformBale {
		return fmt.Errorf("campaign platform is not bale")
	}
	return nil
}

func extractBaleBotID(c dto.BotGetCampaignResponse) (int64, error) {
	if c.PlatformSettings == nil {
		return 0, fmt.Errorf("campaign platform_settings is missing")
	}
	if c.PlatformSettings.Metadata == nil {
		return 0, fmt.Errorf("campaign platform_settings.metadata is missing")
	}
	raw, ok := c.PlatformSettings.Metadata["bale_bot_id"]
	if !ok {
		return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id is missing")
	}

	switch v := raw.(type) {
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id must be positive")
		}
		return int64(v), nil
	case int64:
		if v <= 0 {
			return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id must be positive")
		}
		return v, nil
	case float64:
		if v <= 0 || v != float64(int64(v)) {
			return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id must be a positive integer")
		}
		return int64(v), nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id must not be empty")
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id must be a positive integer")
		}
		return id, nil
	case json.Number:
		id, err := v.Int64()
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id must be a positive integer")
		}
		return id, nil
	default:
		return 0, fmt.Errorf("campaign platform_settings.metadata.bale_bot_id has unsupported type %T", raw)
	}
}

func (s *BaleCampaignScheduler) fetchBaleAudiencePhones(ctx context.Context, c dto.BotGetCampaignResponse, token string, correlationID string) ([]string, []int64, []string, uint, error) {
	s.logger.Printf("fetchBaleAudiencePhones start: campaign_id=%d customer_id=%d num_audiences=%d tags_length=%d correlation_id=%s", c.ID, c.CustomerID, c.NumAudiences, len(c.Tags), correlationID)

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
			s.logger.Printf("fetchBaleAudiencePhones tag parse failed: campaign_id=%d tag=%q err=%v", c.ID, tag, err)
			return nil, nil, nil, 0, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		s.logger.Printf("fetchBaleAudiencePhones tags lookup failed: campaign_id=%d err=%v", c.ID, err)
		return nil, nil, nil, 0, err
	}

	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}

	// NOTE: len(tagIDs) <= len(c.Tags) because some tags may not be found or are inactive

	const limit = 10000000

	tagsHash := baleHashTags(c.Tags)
	selection, err := s.audienceCache.Latest(ctx, c.CustomerID, tagsHash)
	if err != nil {
		s.logger.Printf("fetchBaleAudiencePhones latest selection failed: campaign_id=%d customer_id=%d tags_hash=%s err=%v", c.ID, c.CustomerID, tagsHash, err)
		return nil, nil, nil, 0, err
	}
	if selection != nil {
		s.logger.Printf("fetchBaleAudiencePhones selection hit: campaign_id=%d selection_id=%d prior_ids_length=%d", c.ID, selection.ID, len(selection.IDs))
	} else {
		s.logger.Printf("fetchBaleAudiencePhones selection miss: campaign_id=%d", c.ID)
	}

	selectAudiences := func(exclude map[int64]struct{}) ([]string, []int64, error) {
		phones := make([]string, 0, numAudiences)
		ids := make([]int64, 0, numAudiences)

		filter := models.AudienceProfileFilter{Tags: &tagIDs, Color: utils.ToPtr("white")}
		whites, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
		if err != nil {
			s.logger.Printf("fetchBaleAudiencePhones fetch white failed: campaign_id=%d err=%v", c.ID, err)
			return nil, nil, err
		}
		s.logger.Printf("fetchBaleAudiencePhones white candidates: campaign_id=%d count=%d", c.ID, len(whites))

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
				s.logger.Printf("fetchBaleAudiencePhones fetch pink failed: campaign_id=%d err=%v", c.ID, err)
				return nil, nil, err
			}
			s.logger.Printf("fetchBaleAudiencePhones pink candidates: campaign_id=%d count=%d", c.ID, len(pinks))
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
	s.logger.Printf("fetchBaleAudiencePhones selected (with exclusions): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), c.NumAudiences)

	resetUsed := false
	if int64(len(phones)) < numAudiences {
		// Not enough fresh; retry from scratch without exclusions
		resetUsed = true
		phones, ids, err = selectAudiences(nil)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		s.logger.Printf("fetchBaleAudiencePhones selected (reset): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), c.NumAudiences)
	}

	// Persist selection history with correlation id and merged audience IDs
	var sel *AudienceSelection
	if resetUsed {
		sel, err = s.audienceCache.SaveSnapshot(ctx, c.CustomerID, tagsHash, correlationID, ids)
	} else {
		sel, err = s.audienceCache.SaveWithMerge(ctx, c.CustomerID, tagsHash, correlationID, ids)
	}
	if err != nil {
		s.logger.Printf("fetchBaleAudiencePhones selection save failed: campaign_id=%d err=%v reset=%t", c.ID, err, resetUsed)
		return nil, nil, nil, 0, err
	}
	s.logger.Printf("fetchBaleAudiencePhones selection saved: campaign_id=%d selection_id=%d reset=%t selected=%d", c.ID, sel.ID, resetUsed, len(ids))

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchBaleAudiencePhones skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		s.logger.Printf("fetchBaleAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d ad_link=empty", c.ID, len(phones), len(phones), sel.ID)
		return phones, ids, make([]string, len(phones)), sel.ID, nil
	}

	// Generate sequential UIDs via bot API and persist short links centrally
	codes, err := s.botClient.AllocateShortLinks(ctx, token, c.ID, c.AdLink, phones)
	if err != nil {
		s.logger.Printf("fetchBaleAudiencePhones allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, nil, nil, 0, err
	}
	s.logger.Printf("fetchBaleAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d", c.ID, len(phones), len(codes), sel.ID)
	return phones, ids, codes, sel.ID, nil
}

func baleHashTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	cp := make([]string, len(tags))
	copy(cp, tags)
	sort.Strings(cp)
	h := sha1.Sum([]byte(strings.Join(cp, ",")))
	return hex.EncodeToString(h[:])
}

func (s *BaleCampaignScheduler) buildBaleMessageBody(c dto.BotGetCampaignResponse, code string) string {
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

func (s *BaleCampaignScheduler) notifyAdmin(message string) {
	if s.notifier == nil {
		return
	}
	go func(msg string) {
		for _, mobile := range s.adminCfg.ActiveMobiles() {
			_ = s.notifier.SendSMS(context.Background(), mobile, msg, nil)
		}
	}(message)
}
