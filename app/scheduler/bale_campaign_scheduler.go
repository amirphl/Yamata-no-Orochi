// Package scheduler implements the campaign scheduling and execution logic for Bale campaigns, including audience fetching, message sending with retry logic, and status tracking.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
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

// TODO: Tx management in queries, especially around processed_campaign creation and audience fetching to ensure consistency

const (
	baleSendBatchSize = 100
	statusJobMaxRetry = 3
)

type BaleCampaignScheduler struct {
	audRepo  repository.AudienceProfileRepository
	tagRepo  repository.TagRepository
	sentRepo repository.SentBaleMessageRepository
	pcRepo   repository.ProcessedCampaignRepository
	jobRepo  repository.CampaignStatusJobRepository
	resRepo  repository.BaleStatusResultRepository
	notifier NotificationSender
	logger   *log.Logger
	interval time.Duration

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
	jobRepo repository.CampaignStatusJobRepository,
	resRepo repository.BaleStatusResultRepository,
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
		jobRepo:       jobRepo,
		resRepo:       resRepo,
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
		s.logger.Printf("Bale scheduler: failed to initialize file logger: %v", err)
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
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-parent.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(parent, s.interval*4/5)
				s.runOnce(ctx)
				cancel()
			}
		}
	}()

	go s.startStatusJobWorker(parent)

	return func() {
		if s.logFile != nil {
			_ = s.logFile.Close()
		}
	}
}

func (s *BaleCampaignScheduler) runOnce(ctx context.Context) {
	jazzAccessToken, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("Bale scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Bale scheduler: bot login failed: %v", err))
		return
	}

	ready, err := s.botClient.ListReadyCampaigns(ctx, jazzAccessToken, models.CampaignPlatformBale)
	if err != nil {
		s.logger.Printf("Bale scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Bale scheduler: list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}
	s.logger.Printf("Bale scheduler: listed %d ready campaigns", len(ready))

	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformBale {
			s.logger.Printf("Bale scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform)
			s.notifyAdmin(fmt.Sprintf("Bale scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform))
			continue
		}
		if err := s.validateBaleCampaign(c); err != nil {
			s.logger.Printf("Bale scheduler: validate campaign failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Bale scheduler: validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("Bale scheduler: check processed failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Bale scheduler: check processed failed for id=%d: %v", c.ID, err))
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		} else {
			s.logger.Printf("Bale scheduler: campaign id=%d already processed, skipping", c.ID)
		}
	}
	if len(pending) == 0 {
		return
	}
	s.logger.Printf("Bale scheduler: %d campaigns pending processing...", len(pending))

	for _, camp := range pending {
		go func(c dto.BotGetCampaignResponse) {
			// TODO: Make 4 hours configurable or use a more dynamic approach based on campaign content/size
			ctx2, cancel2 := context.WithTimeout(context.Background(), 4*time.Hour)
			defer cancel2()
			if err := s.processBaleCampaign(ctx2, jazzAccessToken, c); err != nil {
				s.logger.Printf("Bale scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("Bale scheduler: process campaign failed for campaign id=%d: %v", c.ID, err))
			}
		}(camp)
	}
}

func (s *BaleCampaignScheduler) processBaleCampaign(ctx context.Context, jazzAccessToken string, c dto.BotGetCampaignResponse) error {
	botID, err := extractBaleBotID(c)
	if err != nil {
		return fmt.Errorf("resolve Bale bot id: %w", err)
	}
	if c.NumAudiences == nil || *c.NumAudiences <= 0 {
		return fmt.Errorf("campaign has no audiences")
	}

	if err := s.botClient.MoveCampaignToRunning(ctx, jazzAccessToken, c.ID); err != nil {
		return fmt.Errorf("move to running: %w", err)
	}
	s.logger.Printf("Bale scheduler: campaign id=%d moved to running", c.ID)

	// First transaction: create processed_campaign and persist full audience IDs
	var (
		phones       []string
		ids          []int64
		codes        []string
		unmatchedUID []string
		pc           *models.ProcessedCampaign
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
		s.logger.Printf("Bale scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

		if hasTargetAudienceExcelFileUUID(c.TargetAudienceExcelFileUUID) {
			fileUIDs, err := fetchTargetAudienceUIDsFromExcel(txCtx, s.botClient, jazzAccessToken, c.ID)
			if err != nil {
				return err
			}
			audienceResult, err := fetchAudiencePhonesByUIDs(txCtx, s.logger, s.audRepo, s.botClient, c, jazzAccessToken, fileUIDs)
			if err != nil {
				return err
			}
			phones = audienceResult.Phones
			ids = audienceResult.IDs
			codes = audienceResult.Codes
			unmatchedUID = audienceResult.UnmatchedUIDs
			s.logger.Printf("Bale scheduler: fetched %d audience phones via excel for campaign id=%d (unmatched=%d)", len(phones), c.ID, len(unmatchedUID))
			pc.AudienceIDs = pq.Int64Array(ids)
			pc.AudienceCodes = codes
			pc.AudienceSelectionID = nil
		} else {
			// Fetch Bale audiences using tag-based selection with cache-aware exclusion fallback.
			correlationID := uuid.NewString()
			audienceResult, err := s.fetchBaleAudiencePhones(txCtx, c, jazzAccessToken, correlationID)
			if err != nil {
				return err
			}
			phones = audienceResult.Phones
			ids = audienceResult.IDs
			codes = audienceResult.Codes
			s.logger.Printf("Bale scheduler: fetched %d audience phones for campaign id=%d", len(phones), c.ID)
			pc.AudienceIDs = pq.Int64Array(ids)
			pc.AudienceCodes = codes
			pc.AudienceSelectionID = utils.ToPtr(audienceResult.SelectionID)
		}
		pc.UpdatedAt = utils.UTCNow()
		if err := s.pcRepo.Update(txCtx, pc); err != nil {
			return err
		}
		s.logger.Printf("Bale scheduler: updated processed campaign id=%d with audience ids", pc.ID)

		return nil
	}); err != nil {
		return err
	}
	s.logger.Printf("Bale scheduler: persisted processed campaign id=%d num_phones=%d, num_ids=%d, num_codes=%d, num_unmatched=%d", pc.ID, len(phones), len(ids), len(codes), len(unmatchedUID))
	if len(ids) != len(phones) {
		return fmt.Errorf("audience ids mismatch for campaign id=%d: phones=%d ids=%d", c.ID, len(phones), len(ids))
	}
	if len(codes) != len(phones) {
		return fmt.Errorf("audience codes mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}

	if len(unmatchedUID) > 0 {
		if err := s.createUnmatchedSentBaleRows(ctx, pc.ID, unmatchedUID); err != nil {
			return err
		}
	}

	var fileID *string
	if c.MediaUUID != nil {
		id, err := s.uploadCampaignMedia(ctx, jazzAccessToken, c)
		if err != nil {
			return err
		}
		fileID = id
	}

	for start := 0; start < len(phones); start += baleSendBatchSize {
		end := min(start+baleSendBatchSize, len(phones))
		batchPhones := phones[start:end]
		batchIDs := ids[start:end]
		batchCodes := codes[start:end]

		items := make([]BaleSendMessageRequest, 0, len(batchPhones))

		rows := make([]*models.SentBaleMessage, 0, len(batchPhones))
		trackingIDs, err := allocateTrackingIDs(ctx, s.db, len(batchPhones))
		if err != nil {
			return err
		}

		for i, p := range batchPhones {
			body := s.buildBaleMessageBody(c, batchCodes[i])

			trackingID := trackingIDs[i]

			items = append(items, BaleSendMessageRequest{
				RequestID:   trackingIDs[i],
				BotID:       botID,
				PhoneNumber: p,
				MessageData: BaleSendMessageData{
					Message: &BaleMessage{
						Text:   body,
						FileID: fileID,
					},
				},
			})

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

		batchResponses, batchErr := s.baleClient.SendBatch(ctx, items)
		if batchErr != nil {
			s.logger.Printf("Bale scheduler: send batch failed campaign id=%d err=%v", c.ID, batchErr)
		}
		responseByRequestID := make(map[string]*BaleSendMessageResponse, len(batchResponses))
		for i := range batchResponses {
			resp := batchResponses[i]
			reqID := strings.TrimSpace(resp.RequestID)
			if reqID == "" && i < len(items) {
				reqID = strings.TrimSpace(items[i].RequestID)
			}
			if reqID == "" {
				continue
			}
			respCopy := resp
			responseByRequestID[reqID] = &respCopy
		}

		sendUpdates := make([]repository.SentBaleSendResultUpdate, 0, len(items))
		for _, item := range items {
			reqID := strings.TrimSpace(item.RequestID)
			resp := responseByRequestID[reqID]
			sendErr := error(nil)
			if resp == nil {
				if batchErr != nil {
					sendErr = batchErr
				} else {
					sendErr = fmt.Errorf("missing send response for tracking_id=%s", reqID)
				}
			}
			sendUpdates = append(sendUpdates, buildBaleSendResultUpdate(reqID, resp, sendErr))
		}

		if len(sendUpdates) > 0 {
			// TODO: Start tx here if needed?
			if updateErr := s.sentRepo.UpdateSendResultByTrackingIDs(ctx, sendUpdates); updateErr != nil {
				s.logger.Printf("Bale scheduler: failed to batch update sent_bale provider fields for campaign id=%d: %v", c.ID, updateErr)
				// NOTE: Error silent here; not returning to avoid blocking further processing
			}
		}

		if err := s.scheduleStatusCheckJobs(ctx, pc.ID, trackingIDs); err != nil {
			s.logger.Printf("Bale scheduler: failed to schedule status jobs for campaign id=%d: %v", c.ID, err)
			// TODO: How to handle this error? Retry scheduling? Skip status checks for this batch?
		}
		if err := sleepWithContext(ctx, s.messageDelay); err != nil {
			return err
		}
	}

	stats, err := s.updateProcessedCampaignStats(ctx, pc.ID)
	if err != nil {
		return err
	}
	if err := s.botClient.PushCampaignStatistics(ctx, c.ID, stats); err != nil {
		return err
	}

	s.logger.Printf("Bale scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, jazzAccessToken, c.ID); err != nil {
		return err
	}
	s.logger.Printf("Bale scheduler: campaign id=%d moved to executed", c.ID)

	return nil
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
		return fmt.Errorf("Bale api-access-key is not configured")
	}
	if _, err := extractBaleBotID(c); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformBale {
		return fmt.Errorf("campaign platform is not bale")
	}
	return nil
}

func (s *BaleCampaignScheduler) fetchBaleAudiencePhones(
	ctx context.Context,
	c dto.BotGetCampaignResponse,
	jazzAccessToken string,
	correlationID string,
) (*AudiencePhonesResult, error) {
	numAudiences := int64(0)
	if c.NumAudiences != nil {
		numAudiences = int64(*c.NumAudiences)
	}
	s.logger.Printf("fetchBaleAudiencePhones start: campaign_id=%d customer_id=%d num_audiences=%d tags_length=%d correlation_id=%s", c.ID, c.CustomerID, numAudiences, len(c.Tags), correlationID)

	if numAudiences <= 0 {
		return nil, fmt.Errorf("campaign num_audiences must be positive")
	}

	toExtract := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			s.logger.Printf("fetchBaleAudiencePhones tag parse failed: campaign_id=%d tag=%q err=%v", c.ID, tag, err)
			return nil, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		s.logger.Printf("fetchBaleAudiencePhones tags lookup failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}

	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}
	s.logger.Printf("fetchBaleAudiencePhones tags resolved: campaign_id=%d requested=%d resolved=%d", c.ID, len(c.Tags), len(tagIDs))

	// NOTE: len(tagIDs) <= len(c.Tags) because some tags may not be found or are inactive

	const limit = 10000000

	tagsHash := hashTags(c.Tags)
	selection, err := s.audienceCache.Latest(ctx, c.CustomerID, tagsHash)
	if err != nil {
		s.logger.Printf("fetchBaleAudiencePhones latest selection failed: campaign_id=%d customer_id=%d tags_hash=%s err=%v", c.ID, c.CustomerID, tagsHash, err)
		return nil, err
	}
	if selection != nil {
		s.logger.Printf("fetchBaleAudiencePhones selection hit: campaign_id=%d selection_id=%d prior_ids_length=%d", c.ID, selection.ID, len(selection.IDs))
	} else {
		s.logger.Printf("fetchBaleAudiencePhones selection miss: campaign_id=%d", c.ID)
	}

	selectAudiences := func(exclude map[int64]struct{}) ([]string, []int64, error) {
		phones := make([]string, 0, numAudiences)
		ids := make([]int64, 0, numAudiences)

		// Bale campaigns intentionally do not segment audiences by color (white/pink).
		// Color-based routing is SMS-specific, so Bale always queries by tag criteria only.
		filter := models.AudienceProfileFilter{Tags: &tagIDs}
		candidates, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
		if err != nil {
			s.logger.Printf("fetchBaleAudiencePhones fetch candidates failed: campaign_id=%d err=%v", c.ID, err)
			return nil, nil, err
		}
		s.logger.Printf("fetchBaleAudiencePhones candidates: campaign_id=%d count=%d", c.ID, len(candidates))

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

		for _, ap := range candidates {
			if int64(len(phones)) >= numAudiences {
				break
			}
			appendIfFresh(ap)
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
		return nil, err
	}
	s.logger.Printf("fetchBaleAudiencePhones selected (with exclusions): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)

	resetUsed := false
	if int64(len(phones)) < numAudiences {
		// Not enough fresh; retry from scratch without exclusions
		resetUsed = true
		phones, ids, err = selectAudiences(nil)
		if err != nil {
			return nil, err
		}
		s.logger.Printf("fetchBaleAudiencePhones selected (reset): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)
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
		return nil, err
	}
	s.logger.Printf("fetchBaleAudiencePhones selection saved: campaign_id=%d selection_id=%d reset=%t selected=%d", c.ID, sel.ID, resetUsed, len(ids))

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchBaleAudiencePhones skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		s.logger.Printf("fetchBaleAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d ad_link=empty", c.ID, len(phones), len(phones), sel.ID)
		return &AudiencePhonesResult{
			Phones:      phones,
			IDs:         ids,
			Codes:       make([]string, len(phones)),
			SelectionID: sel.ID,
		}, nil
	}

	// Generate sequential UIDs via bot API and persist short links centrally
	codes, err := s.botClient.AllocateShortLinks(ctx, jazzAccessToken, c.ID, c.AdLink, phones)
	if err != nil {
		s.logger.Printf("fetchBaleAudiencePhones allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, err
	}
	if len(codes) != len(phones) {
		return nil, fmt.Errorf("allocate short links length mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}
	s.logger.Printf("fetchBaleAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d", c.ID, len(phones), len(codes), sel.ID)
	return &AudiencePhonesResult{
		Phones:      phones,
		IDs:         ids,
		Codes:       codes,
		SelectionID: sel.ID,
	}, nil
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

func (s *BaleCampaignScheduler) createUnmatchedSentBaleRows(ctx context.Context, processedCampaignID uint, unmatchedUIDs []string) error {
	if len(unmatchedUIDs) == 0 {
		return nil
	}

	const errCode = "AUDIENCE_UID_NOT_FOUND"
	rows := make([]*models.SentBaleMessage, 0, len(unmatchedUIDs))
	for _, uid := range unmatchedUIDs {
		desc := fmt.Sprintf("Audience uid not found or has no phone number: %s", uid)
		code := errCode
		rows = append(rows, &models.SentBaleMessage{
			ProcessedCampaignID: processedCampaignID,
			PhoneNumber:         "",
			PartsDelivered:      0,
			Status:              models.BaleSendStatusUnsuccessful,
			TrackingID:          uuid.NewString(),
			ServerID:            nil,
			ErrorCode:           &code,
			Description:         &desc,
		})
	}
	return s.sentRepo.SaveBatch(ctx, rows)
}

func (s *BaleCampaignScheduler) scheduleStatusCheckJobs(ctx context.Context, processedCampaignID uint, trackingIDs []string) error {
	if len(trackingIDs) == 0 || !s.baleClient.SupportsStatusTracking() || s.jobRepo == nil {
		return nil
	}
	filteredTrackingIDs := make([]string, 0, len(trackingIDs))
	for _, id := range trackingIDs {
		if strings.TrimSpace(id) != "" {
			filteredTrackingIDs = append(filteredTrackingIDs, strings.TrimSpace(id))
		}
	}
	if len(filteredTrackingIDs) == 0 {
		return nil
	}

	corrID := uuid.NewString()
	now := utils.UTCNow()
	offsets := []time.Duration{5 * time.Minute, 15 * time.Minute, 1 * time.Hour, 24 * time.Hour, 48 * time.Hour}
	jobs := make([]*models.CampaignStatusJob, 0, len(offsets))
	for _, off := range offsets {
		jobs = append(jobs, &models.CampaignStatusJob{
			ProcessedCampaignID: processedCampaignID,
			CorrelationID:       corrID,
			TrackingIDs:         pq.StringArray(filteredTrackingIDs),
			RetryCount:          0,
			ScheduledAt:         now.Add(off),
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return s.jobRepo.SaveBatch(ctx, jobs)
}

func (s *BaleCampaignScheduler) startStatusJobWorker(ctx context.Context) {
	ticker := time.NewTicker(statusJobWorkerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctx2, cancel := context.WithTimeout(ctx, 10*time.Minute)
			s.processStatusJobs(ctx2)
			cancel()
		}
	}
}

func (s *BaleCampaignScheduler) processStatusJobs(ctx context.Context) {
	if !s.baleClient.SupportsStatusTracking() || s.jobRepo == nil || s.resRepo == nil {
		return
	}

	now := utils.UTCNow()
	jobs, err := s.jobRepo.ListDue(ctx, now, numJobsPerTick)
	if err != nil {
		s.logger.Printf("Bale scheduler: list status jobs failed: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	// TODO: Consider processing jobs in parallel if they are independent (different campaigns) to speed up status updates, but be mindful of rate limits and database contention.
	for _, job := range jobs {
		jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // TODO: Make this timeout configurable
		if err := s.handleStatusJob(jobCtx, job); err != nil {
			s.logger.Printf("Bale scheduler: handle status job id=%d failed: %v", job.ID, err)
			if job.RetryCount >= statusJobMaxRetry {
				s.notifyAdmin(fmt.Sprintf("Bale scheduler: status job id=%d has failed %d times with error: %v", job.ID, job.RetryCount, err))
			}
			// Note: The job will be retried later based on the retry logic in handleStatusJob, so we don't need to do anything else here for retries.
		} else {
			s.logger.Printf("Bale scheduler: handle status job id=%d succeeded", job.ID)
		}
		cancel()
	}
}

func (s *BaleCampaignScheduler) handleStatusJob(ctx context.Context, job *models.CampaignStatusJob) error {
	rows, err := s.sentRepo.ListByTrackingIDs(ctx, job.ProcessedCampaignID, []string(job.TrackingIDs))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return s.markStatusJobExecuted(ctx, job, nil)
	}

	rowByServerID := make(map[string]*models.SentBaleMessage, len(rows))
	serverIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.ServerID == nil {
			continue
		}
		serverID := strings.TrimSpace(*row.ServerID)
		if serverID == "" {
			continue
		}
		if _, seen := rowByServerID[serverID]; seen {
			continue
		}
		rowByServerID[serverID] = row
		serverIDs = append(serverIDs, serverID)
	}
	if len(serverIDs) == 0 {
		return s.markStatusJobExecuted(ctx, job, nil)
	}

	statusItems, fetchErr := s.baleClient.FetchStatus(ctx, serverIDs)
	if fetchErr != nil {
		now := utils.UTCNow()
		job.RetryCount++
		msg := fetchErr.Error()
		job.Error = &msg
		job.UpdatedAt = now
		// Keep job open for retries until max retry threshold is reached.
		if job.RetryCount >= statusJobMaxRetry {
			job.ExecutedAt = &now
		} else {
			job.ExecutedAt = nil
		}
		if err := s.jobRepo.Update(ctx, job); err != nil {
			return err
		}
		return fetchErr
	}

	var stats map[string]any
	txErr := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		now := utils.UTCNow()

		statusRows := make([]*models.BaleStatusResult, 0, len(statusItems))
		sendUpdates := make([]repository.SentBaleSendResultUpdate, 0, len(statusItems))
		for _, item := range statusItems {
			serverID := strings.TrimSpace(item.MessageID)
			if serverID == "" {
				continue
			}
			row := rowByServerID[serverID]
			if row == nil {
				continue
			}

			totalParts, deliveredParts, undeliveredParts, unknownParts, status := mapBaleProviderStatus(item.Status)
			statusText := strings.TrimSpace(item.StatusText)

			var errorCode *string
			var description *string
			if status == models.BaleSendStatusUnsuccessful {
				code := strconv.Itoa(item.Status)
				errorCode = &code
			}
			if metadataDesc := buildBaleStatusMetadataDescription(row.Description, item.Status, statusText); metadataDesc != "" {
				description = &metadataDesc
			}

			sendUpdates = append(sendUpdates, repository.SentBaleSendResultUpdate{
				TrackingID:     row.TrackingID,
				Status:         status,
				PartsDelivered: int(deliveredParts),
				ServerID:       row.ServerID,
				ErrorCode:      errorCode,
				Description:    description,
			})

			statusValue := strconv.Itoa(item.Status)
			providerStatusCode := int64(item.Status)
			provider := strings.TrimSpace(item.Provider)
			var statusTextPtr *string
			if statusText != "" {
				statusTextPtr = &statusText
			}
			var providerPtr *string
			if provider != "" {
				providerPtr = &provider
			}
			metadata := buildBaleStatusResultMetadata(item, status)

			statusRows = append(statusRows, &models.BaleStatusResult{
				JobID:                 job.ID,
				ProcessedCampaignID:   job.ProcessedCampaignID,
				TrackingID:            row.TrackingID,
				ServerID:              row.ServerID,
				Provider:              providerPtr,
				ProviderStatusCode:    &providerStatusCode,
				ProviderStatusText:    statusTextPtr,
				TotalParts:            &totalParts,
				TotalDeliveredParts:   &deliveredParts,
				TotalUndeliveredParts: &undeliveredParts,
				TotalUnknownParts:     &unknownParts,
				Status:                &statusValue,
				Metadata:              metadata,
			})
		}
		if err := s.sentRepo.UpdateSendResultByTrackingIDs(txCtx, sendUpdates); err != nil {
			return err
		}
		if err := s.resRepo.SaveBatch(txCtx, statusRows); err != nil {
			return err
		}

		job.ExecutedAt = &now
		job.Error = nil
		job.UpdatedAt = now
		return s.jobRepo.Update(txCtx, job)
	})
	if txErr != nil {
		return txErr
	}

	if stats, err = s.updateProcessedCampaignStats(ctx, job.ProcessedCampaignID); err != nil {
		return err
	}

	if stats != nil {
		pc, err := s.pcRepo.ByID(ctx, job.ProcessedCampaignID)
		if err != nil {
			return err
		}
		if pc == nil {
			return fmt.Errorf("processed campaign not found for processed campaign id=%d", job.ProcessedCampaignID)
		}
		if err := s.botClient.PushCampaignStatistics(ctx, pc.CampaignID, stats); err != nil {
			return err
		}
	}
	return nil
}

func (s *BaleCampaignScheduler) markStatusJobExecuted(ctx context.Context, job *models.CampaignStatusJob, errText *string) error {
	now := utils.UTCNow()
	job.ExecutedAt = &now
	job.UpdatedAt = now
	job.Error = errText
	return s.jobRepo.Update(ctx, job)
}

func mapBaleProviderStatus(statusCode int) (totalParts int64, deliveredParts int64, undeliveredParts int64, unknownParts int64, status models.BaleSendStatus) {
	totalParts = 1
	switch statusCode {
	case 10:
		return 1, 1, 0, 0, models.BaleSendStatusSuccessful
	case 6, 11, 13, 14, 100:
		return 1, 0, 1, 0, models.BaleSendStatusUnsuccessful
	case 1, 2, 4:
		return 1, 0, 0, 1, models.BaleSendStatusPending
	default:
		return 1, 0, 0, 1, models.BaleSendStatusPending
	}
}

func (s *BaleCampaignScheduler) uploadCampaignMedia(ctx context.Context, jazzAccessToken string, c dto.BotGetCampaignResponse) (*string, error) {
	if c.MediaUUID == nil || *c.MediaUUID == uuid.Nil {
		return nil, nil
	}

	path, err := s.botClient.DownloadCampaignMedia(ctx, jazzAccessToken, c.MediaUUID.String())
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

func buildBaleSendResultUpdate(trackingID string, resp *BaleSendMessageResponse, sendErr error) repository.SentBaleSendResultUpdate {
	update := repository.SentBaleSendResultUpdate{
		TrackingID:     trackingID,
		PartsDelivered: 0,
		Status:         models.BaleSendStatusUnsuccessful,
	}

	if sendErr != nil {
		code := "SEND_FAILED"
		desc := sendErr.Error()
		update.ErrorCode = &code
		update.Description = &desc
		return update
	}

	if resp != nil && len(resp.ErrorData) > 0 {
		first := resp.ErrorData[0]
		code := first.CodeString()
		desc := buildBaleSendMetadataDescription(resp, marshalBaleErrorSpec(resp.ErrorData))
		update.ErrorCode = &code
		update.Description = &desc
		return update
	}

	update.Status = models.BaleSendStatusSuccessful
	update.PartsDelivered = 1
	if resp != nil && strings.TrimSpace(resp.MessageID) != "" {
		id := strings.TrimSpace(resp.MessageID)
		update.ServerID = &id
	}
	desc := buildBaleSendMetadataDescription(resp, "")
	if strings.TrimSpace(desc) != "" {
		update.Description = &desc
	}
	return update
}

func buildBaleSendMetadataDescription(resp *BaleSendMessageResponse, fallback string) string {
	if resp == nil {
		return strings.TrimSpace(fallback)
	}

	metadata := map[string]any{
		"provider": resp.Provider,
	}
	if strings.TrimSpace(resp.MessageID) != "" {
		metadata["messageID"] = strings.TrimSpace(resp.MessageID)
	}
	if len(resp.ErrorData) > 0 {
		metadata["errorData"] = resp.ErrorData
	}
	if trimmedFallback := strings.TrimSpace(fallback); trimmedFallback != "" {
		metadata["error"] = trimmedFallback
	}
	if len(resp.RawBody) > 0 {
		metadata["raw"] = json.RawMessage(resp.RawBody)
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		if strings.TrimSpace(fallback) != "" {
			return strings.TrimSpace(fallback)
		}
		if len(resp.RawBody) > 0 {
			return strings.TrimSpace(string(resp.RawBody))
		}
		return ""
	}
	return string(data)
}

func buildBaleStatusMetadataDescription(existing *string, statusCode int, statusText string) string {
	metadata := map[string]any{}

	if existing != nil && strings.TrimSpace(*existing) != "" {
		var existingJSON map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(*existing)), &existingJSON); err == nil {
			for k, v := range existingJSON {
				metadata[k] = v
			}
		} else {
			metadata["previousDescription"] = strings.TrimSpace(*existing)
		}
	}

	metadata["lastStatus"] = map[string]any{
		"code": statusCode,
		"text": strings.TrimSpace(statusText),
		"at":   utils.UTCNow().Format(time.RFC3339),
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return strings.TrimSpace(statusText)
	}
	return string(data)
}

func buildBaleStatusResultMetadata(item BaleStatusResponse, normalizedStatus models.BaleSendStatus) json.RawMessage {
	metadata := map[string]any{
		"provider":         strings.TrimSpace(item.Provider),
		"messageID":        strings.TrimSpace(item.MessageID),
		"statusCode":       item.Status,
		"statusText":       strings.TrimSpace(item.StatusText),
		"normalizedStatus": string(normalizedStatus),
	}
	if len(item.RawBody) > 0 {
		metadata["raw"] = json.RawMessage(item.RawBody)
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
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

func (s *BaleCampaignScheduler) updateProcessedCampaignStats(ctx context.Context, processedCampaignID uint) (map[string]any, error) {
	pc, err := s.pcRepo.ByID(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, fmt.Errorf("processed campaign not found for processed_campaign_id=%d", processedCampaignID)
	}
	if s.resRepo == nil {
		return s.updateProcessedCampaignStatsFromSentRows(ctx, pc)
	}

	agg, err := s.resRepo.AggregateByCampaign(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}

	trackingResults, err := s.resRepo.TrackingResultsByCampaign(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}

	// Fallback before any status jobs land.
	if agg.AggregatedTotalRecords == 0 && len(trackingResults) == 0 {
		return s.updateProcessedCampaignStatsFromSentRows(ctx, pc)
	}

	stats := map[string]any{
		"aggregatedTotalRecords":          agg.AggregatedTotalRecords,
		"aggregatedTotalSent":             agg.AggregatedTotalSent,
		"aggregatedTotalParts":            agg.AggregatedTotalParts,
		"aggregatedTotalDeliveredParts":   agg.AggregatedDeliveredParts,
		"aggregatedTotalUnDeliveredParts": agg.AggregatedUndelivered,
		"aggregatedTotalUnKnownParts":     agg.AggregatedUnknown,
		"trackingResults":                 trackingResults,
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

func (s *BaleCampaignScheduler) updateProcessedCampaignStatsFromSentRows(ctx context.Context, pc *models.ProcessedCampaign) (map[string]any, error) {
	type row struct {
		Total      int64
		Successful int64
	}
	var agg row
	if err := s.db.WithContext(ctx).Table("sent_bale_messages").
		Select(`
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN LOWER(BTRIM(status::text)) = 'successful' THEN 1 ELSE 0 END), 0) AS successful`).
		Where("processed_campaign_id = ?", pc.ID).
		Scan(&agg).Error; err != nil {
		return nil, err
	}

	trackingResults, err := s.sentRepo.TrackingResultsFromSentRows(ctx, pc.ID)
	if err != nil {
		return nil, err
	}

	stats := map[string]any{
		"aggregatedTotalRecords":          agg.Total,
		"aggregatedTotalSent":             agg.Successful,
		"aggregatedTotalParts":            agg.Total,
		"aggregatedTotalDeliveredParts":   agg.Successful,
		"aggregatedTotalUnDeliveredParts": agg.Total - agg.Successful,
		"aggregatedTotalUnKnownParts":     int64(0),
		"trackingResults":                 trackingResults,
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
