// Package scheduler
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
	smsSendBatchSize     = 100 // NOTE: MUST BE LESS THAN 250
	smsStatusJobMaxRetry = 3
)

type SMSCampaignScheduler struct {
	audRepo  repository.AudienceProfileRepository
	tagRepo  repository.TagRepository
	sentRepo repository.SentSMSRepository
	pcRepo   repository.ProcessedCampaignRepository
	jobRepo  repository.CampaignStatusJobRepository
	resRepo  repository.SMSStatusResultRepository
	notifier NotificationSender
	logger   *log.Logger
	interval time.Duration

	db       *gorm.DB
	adminCfg config.AdminConfig
	botCfg   config.BotConfig

	botClient BotClient
	smsClient PayamSMSClient

	logFile *os.File

	schedulerName string

	audienceCache *AudienceCache
}

// NotificationSender is a minimal interface extracted from NotificationService for SMS
// This keeps the scheduler independent and easy to test
type NotificationSender interface {
	SendSMS(ctx context.Context, to string, message string, trackingID *int64) error
	SendSMSBulk(ctx context.Context, mobiles []string, message string, trackingID *int64) error
}

func NewCampaignScheduler(
	audRepo repository.AudienceProfileRepository,
	tagRepo repository.TagRepository,
	sentRepo repository.SentSMSRepository,
	pcRepo repository.ProcessedCampaignRepository,
	jobRepo repository.CampaignStatusJobRepository,
	resRepo repository.SMSStatusResultRepository,
	notifier NotificationSender,
	db *gorm.DB,
	logger *log.Logger,
	interval time.Duration,
	payamSMSCfg config.PayamSMSConfig,
	botCfg config.BotConfig,
	adminCfg config.AdminConfig,
) *SMSCampaignScheduler {
	if interval <= 0 {
		interval = time.Minute
	}

	if botCfg.APIDomain == "" {
		botCfg.APIDomain = defaultBotAPIDomain
	}

	s := &SMSCampaignScheduler{
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
		adminCfg:      adminCfg,
		botCfg:        botCfg,
		botClient:     newHTTPBotClient(botCfg),
		smsClient:     newHTTPPayamSMSClient(payamSMSCfg),
		audienceCache: NewAudienceCache(repository.NewAudienceSelectionRepository(db)),
		schedulerName: "sms",
	}

	if err := s.initSchedulerLogger(); err != nil {
		s.logger = log.New(io.Discard, "sms_scheduler ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		s.logger.Printf("SMS scheduler: failed to initialize file logger: %v", err)
	}

	return s
}

func (s *SMSCampaignScheduler) initSchedulerLogger() error {
	l, f, err := initSchedulerLogger(s.schedulerName + "_scheduler")
	if err != nil {
		return err
	}
	s.logFile = f
	s.logger = l
	return nil
}

func (s *SMSCampaignScheduler) Start(parent context.Context) func() {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-parent.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(parent, s.interval*4/5)
				s.runOnce(ctx, parent)
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

func (s *SMSCampaignScheduler) runOnce(ctx context.Context, parent context.Context) {
	jazzAccessToken, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("SMS scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("SMS Scheduler: bot login failed: %v", err))
		return
	}

	ready, err := s.botClient.ListReadyCampaigns(ctx, jazzAccessToken, models.CampaignPlatformSMS)
	if err != nil {
		s.logger.Printf("SMS scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("SMS Scheduler: list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}
	s.logger.Printf("SMS scheduler: listed %d ready campaigns", len(ready))

	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformSMS {
			s.logger.Printf("SMS scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform)
			s.notifyAdmin(fmt.Sprintf("SMS Scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform))
			continue
		}
		if err := s.validateSMSCampaign(c); err != nil {
			s.logger.Printf("SMS scheduler: validate campaign failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("SMS Scheduler: validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("SMS scheduler: check processed failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("SMS Scheduler: check processed failed for id=%d: %v", c.ID, err))
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		} else {
			s.logger.Printf("SMS scheduler: campaign id=%d already processed, skipping", c.ID)
		}
	}
	if len(pending) == 0 {
		return
	}
	s.logger.Printf("SMS scheduler: %d campaigns pending processing...", len(pending))

	for _, camp := range pending {
		go func(c dto.BotGetCampaignResponse) {
			// TODO: Make 4 hours configurable or use a more dynamic approach based on campaign content/size
			ctx2, cancel2 := context.WithTimeout(parent, 4*time.Hour)
			defer cancel2()
			if err := s.processSMSCampaign(ctx2, jazzAccessToken, c); err != nil {
				s.logger.Printf("SMS scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("SMS Scheduler: process campaign failed for campaign id=%d: %v", c.ID, err))
			}
		}(camp)
	}
}

func (s *SMSCampaignScheduler) processSMSCampaign(ctx context.Context, jazzAccessToken string, c dto.BotGetCampaignResponse) error {
	// Sender from campaign line number
	if c.LineNumber == nil {
		return fmt.Errorf("sender is nil")
	}
	if c.NumAudiences == nil || *c.NumAudiences <= 0 {
		return fmt.Errorf("campaign has no audiences")
	}
	sender := *c.LineNumber

	if err := s.botClient.MoveCampaignToRunning(ctx, jazzAccessToken, c.ID); err != nil {
		return fmt.Errorf("move to running: %w", err)
	}
	s.logger.Printf("SMS scheduler: campaign id=%d moved to running", c.ID)

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
		s.logger.Printf("SMS scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

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
			s.logger.Printf("SMS scheduler: fetched %d audience phones via excel for campaign id=%d (unmatched=%d)", len(phones), c.ID, len(unmatchedUID))
			pc.AudienceIDs = pq.Int64Array(ids)
			pc.AudienceCodes = codes
			pc.AudienceSelectionID = nil
		} else {
			// Fetch audiences (white then pink, DB-shuffled), and sort order is enforced inside repo
			var err error
			correlationID := uuid.NewString()
			audienceResult, err := s.fetchSMSAudiencePhones(txCtx, c, jazzAccessToken, correlationID)
			if err != nil {
				return err
			}
			phones = audienceResult.Phones
			ids = audienceResult.IDs
			codes = audienceResult.Codes
			s.logger.Printf("SMS scheduler: fetched %d audience phones for campaign id=%d", len(phones), c.ID)
			pc.AudienceIDs = pq.Int64Array(ids)
			pc.AudienceCodes = codes
			pc.AudienceSelectionID = utils.ToPtr(audienceResult.SelectionID)
		}

		pc.UpdatedAt = utils.UTCNow()
		if err := s.pcRepo.Update(txCtx, pc); err != nil {
			return err
		}
		s.logger.Printf("SMS scheduler: updated processed campaign id=%d with audience ids", pc.ID)

		return nil
	}); err != nil {
		return err
	}
	s.logger.Printf("SMS scheduler: persisted processed campaign id=%d num_phones=%d, num_ids=%d, num_codes=%d, num_unmatched=%d", pc.ID, len(phones), len(ids), len(codes), len(unmatchedUID))
	if len(ids) != len(phones) {
		return fmt.Errorf("audience ids mismatch for campaign id=%d: phones=%d ids=%d", c.ID, len(phones), len(ids))
	}
	if len(codes) != len(phones) {
		return fmt.Errorf("audience codes mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}

	if len(unmatchedUID) > 0 {
		if err := s.createUnmatchedSentSMSRows(ctx, pc.ID, unmatchedUID); err != nil {
			return err
		}
	}

	for start := 0; start < len(phones); start += smsSendBatchSize {
		end := min(start+smsSendBatchSize, len(phones))
		batchPhones := phones[start:end]
		batchIDs := ids[start:end]
		batchCodes := codes[start:end]

		items := make([]PayamSMSItem, 0, len(batchPhones))

		rows := make([]*models.SentSMS, 0, len(batchPhones))
		trackingIDs, err := allocateTrackingIDs(ctx, s.db, len(batchPhones))
		if err != nil {
			return err
		}

		for i, p := range batchPhones {
			body := s.buildSMSBody(c, batchCodes[i])

			trackingID := trackingIDs[i]

			items = append(items, PayamSMSItem{
				Recipient:  p,
				Body:       body,
				TrackingID: trackingID,
			})

			rows = append(rows, &models.SentSMS{
				ProcessedCampaignID: pc.ID,
				PhoneNumber:         p,
				PartsDelivered:      0,
				Status:              models.SMSSendStatusPending,
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

		batchResponses, batchErr := s.smsClient.SendBatch(ctx, sender, items)
		if batchErr != nil {
			s.logger.Printf("SMS scheduler: payamsms send batch failed for campaign id=%d: %v", c.ID, batchErr)
			// TODO: How to handle this error? Retry sending? Skip to next batch?
		}

		responseByTrackingID := make(map[string]*PayamSMSResponseItem, len(batchResponses))
		for i := range batchResponses {
			resp := batchResponses[i]
			trackingID := strings.TrimSpace(resp.TrackingID)
			if trackingID == "" {
				continue
			}
			respCopy := resp
			responseByTrackingID[trackingID] = &respCopy
		}

		sendUpdates := make([]repository.SentSMSProviderUpdate, 0, len(items))
		for _, item := range items {
			trackingID := strings.TrimSpace(item.TrackingID)
			if trackingID == "" {
				continue
			}
			sendUpdates = append(sendUpdates, buildSMSProviderUpdate(trackingID, responseByTrackingID[trackingID], batchErr))
		}
		if len(sendUpdates) > 0 {
			// TODO: Start tx here if needed?
			if updateErr := s.sentRepo.UpdateProviderFieldsByTrackingIDs(ctx, sendUpdates); updateErr != nil {
				s.logger.Printf("SMS scheduler: failed to batch update sent_sms provider fields for campaign id=%d: %v", c.ID, updateErr)
				// NOTE: Error silent here; not returning to avoid blocking further processing
			}
		}

		// Schedule status check jobs for this batch
		batchTrackingIDs := make([]string, 0, len(rows))
		for _, r := range rows {
			if r.TrackingID != "" {
				batchTrackingIDs = append(batchTrackingIDs, r.TrackingID)
			}
		}
		if err := s.scheduleStatusCheckJobs(ctx, pc.ID, batchTrackingIDs); err != nil {
			s.logger.Printf("SMS scheduler: failed to schedule status jobs for campaign id=%d: %v", c.ID, err)
			// TODO: How to handle this error? Retry scheduling? Skip and rely on next batch's jobs to eventually cover these?
		}
	}

	stats, err := s.updateProcessedCampaignStats(ctx, pc.ID)
	if err != nil {
		return err
	}
	if err := s.botClient.PushCampaignStatistics(ctx, c.ID, stats); err != nil {
		return err
	}

	s.logger.Printf("SMS scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, jazzAccessToken, c.ID); err != nil {
		return err
	}
	s.logger.Printf("SMS scheduler: campaign id=%d moved to executed", c.ID)

	return nil
}

func (s *SMSCampaignScheduler) validateSMSCampaign(c dto.BotGetCampaignResponse) error {
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
	if c.LineNumber == nil || *c.LineNumber == "" {
		return fmt.Errorf("campaign line number (sender) is empty")
	}
	if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformSMS {
		return fmt.Errorf("campaign platform is not sms")
	}
	return nil
}

func (s *SMSCampaignScheduler) fetchSMSAudiencePhones(
	ctx context.Context,
	c dto.BotGetCampaignResponse,
	jazzAccessToken string,
	correlationID string,
) (*AudiencePhonesResult, error) {
	numAudiences := int64(0)
	if c.NumAudiences != nil {
		numAudiences = int64(*c.NumAudiences)
	}
	s.logger.Printf("fetchSMSAudiencePhones start: campaign_id=%d customer_id=%d num_audiences=%d tags_length=%d correlation_id=%s", c.ID, c.CustomerID, numAudiences, len(c.Tags), correlationID)

	if numAudiences <= 0 {
		return nil, fmt.Errorf("campaign num_audiences must be positive")
	}

	toExtract := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			s.logger.Printf("fetchSMSAudiencePhones tag parse failed: campaign_id=%d tag=%q err=%v", c.ID, tag, err)
			return nil, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhones tags lookup failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}

	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}
	s.logger.Printf("fetchSMSAudiencePhones tags resolved: campaign_id=%d requested=%d resolved=%d", c.ID, len(c.Tags), len(tagIDs))

	// NOTE: len(tagIDs) <= len(c.Tags) because some tags may not be found or are inactive

	const limit = 10000000

	tagsHash := hashTags(c.Tags)
	selection, err := s.audienceCache.Latest(ctx, c.CustomerID, tagsHash)
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhones latest selection failed: campaign_id=%d customer_id=%d tags_hash=%s err=%v", c.ID, c.CustomerID, tagsHash, err)
		return nil, err
	}
	if selection != nil {
		s.logger.Printf("fetchSMSAudiencePhones selection hit: campaign_id=%d selection_id=%d prior_ids_length=%d", c.ID, selection.ID, len(selection.IDs))
	} else {
		s.logger.Printf("fetchSMSAudiencePhones selection miss: campaign_id=%d", c.ID)
	}

	selectAudiences := func(exclude map[int64]struct{}) ([]string, []int64, error) {
		phones := make([]string, 0, numAudiences)
		ids := make([]int64, 0, numAudiences)

		filter := models.AudienceProfileFilter{
			Tags:  &tagIDs,
			Color: utils.ToPtr("white"),
		}
		whites, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
		if err != nil {
			s.logger.Printf("fetchSMSAudiencePhones fetch white failed: campaign_id=%d err=%v", c.ID, err)
			return nil, nil, err
		}
		s.logger.Printf("fetchSMSAudiencePhones white candidates: campaign_id=%d count=%d", c.ID, len(whites))

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
			filter := models.AudienceProfileFilter{
				Tags:  &tagIDs,
				Color: utils.ToPtr("pink"),
			}
			pinks, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
			if err != nil {
				s.logger.Printf("fetchSMSAudiencePhones fetch pink failed: campaign_id=%d err=%v", c.ID, err)
				return nil, nil, err
			}
			s.logger.Printf("fetchSMSAudiencePhones pink candidates: campaign_id=%d count=%d", c.ID, len(pinks))
			for _, ap := range pinks {
				if len(phones) >= int(numAudiences) {
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
		return nil, err
	}
	s.logger.Printf("fetchSMSAudiencePhones selected (with exclusions): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)

	resetUsed := false
	if int64(len(phones)) < numAudiences {
		// Not enough fresh; retry from scratch without exclusions
		resetUsed = true
		phones, ids, err = selectAudiences(nil)
		if err != nil {
			return nil, err
		}
		s.logger.Printf("fetchSMSAudiencePhones selected (reset): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)
	}

	// Persist selection history with correlation id and merged audience IDs
	var sel *AudienceSelection
	if resetUsed {
		sel, err = s.audienceCache.SaveSnapshot(ctx, c.CustomerID, tagsHash, correlationID, ids)
	} else {
		sel, err = s.audienceCache.SaveWithMerge(ctx, c.CustomerID, tagsHash, correlationID, ids)
	}
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhones selection save failed: campaign_id=%d err=%v reset=%t", c.ID, err, resetUsed)
		return nil, err
	}
	s.logger.Printf("fetchSMSAudiencePhones selection saved: campaign_id=%d selection_id=%d reset=%t selected=%d", c.ID, sel.ID, resetUsed, len(ids))

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchSMSAudiencePhones skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		s.logger.Printf("fetchSMSAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d ad_link=empty", c.ID, len(phones), len(phones), sel.ID)
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
		s.logger.Printf("fetchSMSAudiencePhones allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, err
	}
	if len(codes) != len(phones) {
		return nil, fmt.Errorf("allocate short links length mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}
	s.logger.Printf("fetchSMSAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d", c.ID, len(phones), len(codes), sel.ID)
	return &AudiencePhonesResult{
		Phones:      phones,
		IDs:         ids,
		Codes:       codes,
		SelectionID: sel.ID,
	}, nil
}

func (s *SMSCampaignScheduler) buildSMSBody(c dto.BotGetCampaignResponse, code string) string {
	content := ""
	if c.Content != nil {
		content = *c.Content
	}
	if hasCampaignAdLink(c.AdLink) {
		shortened := "jo1n.ir/" + code
		return strings.ReplaceAll(content, "🔗", shortened) + "\n" + "لغو۱۱"

	}
	return strings.ReplaceAll(content, "🔗", "") + "\n" + "لغو۱۱"
}

func (s *SMSCampaignScheduler) createUnmatchedSentSMSRows(ctx context.Context, processedCampaignID uint, unmatchedUIDs []string) error {
	pc, err := s.pcRepo.ByID(ctx, processedCampaignID)
	if err != nil {
		return err
	}
	if pc == nil {
		return fmt.Errorf("processed campaign not found for processed campaign id=%d", processedCampaignID)
	}

	trackingIDs, err := allocateTrackingIDs(ctx, s.db, len(unmatchedUIDs))
	if err != nil {
		return err
	}

	const errCode = "AUDIENCE_UID_NOT_FOUND"

	fakeSentSMSs := make([]*models.SentSMS, 0, len(unmatchedUIDs))
	for i, uid := range unmatchedUIDs {
		desc := fmt.Sprintf("Audience uid not found or has no phone number: %s", uid)
		code := errCode
		fakeSentSMSs = append(fakeSentSMSs, &models.SentSMS{
			ProcessedCampaignID: processedCampaignID,
			PhoneNumber:         "",
			TrackingID:          trackingIDs[i],
			PartsDelivered:      0,
			Status:              models.SMSSendStatusUnsuccessful,
			ServerID:            nil,
			ErrorCode:           &code,
			Description:         &desc,
		})
	}
	if len(fakeSentSMSs) == 0 {
		return nil
	}

	var stats map[string]any
	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		if err := s.sentRepo.SaveBatch(txCtx, fakeSentSMSs); err != nil {
			return err
		}

		now := utils.UTCNow()
		executedAt := now.Add(time.Second)
		fakeJob := &models.CampaignStatusJob{
			ProcessedCampaignID: processedCampaignID,
			CorrelationID:       uuid.NewString(),
			TrackingIDs:         pq.StringArray(trackingIDs),
			RetryCount:          0,
			ScheduledAt:         now,
			ExecutedAt:          &executedAt,
			CreatedAt:           now,
			UpdatedAt:           now.Add(time.Second),
		}
		if err := s.jobRepo.Save(txCtx, fakeJob); err != nil {
			return err
		}

		zeroVal := int64(0)
		zero := &zeroVal
		fakeSMSStatusResults := make([]*models.SMSStatusResult, 0, len(unmatchedUIDs))
		for _, trackingID := range trackingIDs {
			status := errCode
			fakeSMSStatusResults = append(fakeSMSStatusResults, &models.SMSStatusResult{
				JobID:                 fakeJob.ID,
				ProcessedCampaignID:   fakeJob.ProcessedCampaignID,
				TrackingID:            trackingID,
				ServerID:              nil,
				TotalParts:            zero,
				TotalDeliveredParts:   zero,
				TotalUndeliveredParts: zero,
				TotalUnknownParts:     zero,
				Status:                &status,
			})
		}
		if err := s.resRepo.SaveBatch(txCtx, fakeSMSStatusResults); err != nil {
			return err
		}

		var err error
		stats, err = s.updateProcessedCampaignStats(txCtx, processedCampaignID)
		return err
	}); err != nil {
		return err
	}

	if stats != nil {
		if err := s.botClient.PushCampaignStatistics(ctx, pc.CampaignID, stats); err != nil {
			return err
		}
	}

	return nil
}

func (s *SMSCampaignScheduler) scheduleStatusCheckJobs(ctx context.Context, processedCampaignID uint, trackingIDs []string) error {
	if len(trackingIDs) == 0 || s.jobRepo == nil {
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

func (s *SMSCampaignScheduler) startStatusJobWorker(ctx context.Context) {
	ticker := time.NewTicker(statusJobWorkerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Minute)
			s.processStatusJobs(ctx2)
			cancel2()
		}
	}
}

func (s *SMSCampaignScheduler) processStatusJobs(ctx context.Context) {
	if s.jobRepo == nil || s.resRepo == nil {
		return
	}

	now := utils.UTCNow()
	jobs, err := s.jobRepo.ListDue(ctx, now, numJobsPerTick)
	if err != nil {
		s.logger.Printf("SMS scheduler: list status jobs failed: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	atiehAccessToken, err := s.smsClient.GetToken(ctx)
	if err != nil {
		s.logger.Printf("SMS scheduler: payamsms token for status jobs failed: %v", err)
		return
	}

	// TODO: Consider processing jobs in parallel if they are independent (different campaigns) to speed up status updates, but be mindful of rate limits and database contention.
	for _, j := range jobs {
		func(job *models.CampaignStatusJob) {
			jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // TODO: adjust timeout as needed
			defer cancel()
			if err := s.handleStatusJob(jobCtx, job, atiehAccessToken); err != nil {
				s.logger.Printf("SMS scheduler: handle status job id=%d failed: %v", job.ID, err)
				if job.RetryCount >= smsStatusJobMaxRetry {
					s.logger.Printf("SMS scheduler: status job id=%d reached max retries, marking as failed", job.ID)
					// Optionally, update job status to failed in DB here
				}
				// Note: The job will be retried in the next tick based on its ScheduledAt and RetryCount
			} else {
				s.logger.Printf("SMS scheduler: handle status job id=%d succeeded", job.ID)
			}
		}(j)
	}
}

func (s *SMSCampaignScheduler) handleStatusJob(ctx context.Context, job *models.CampaignStatusJob, jazzAccessToken string) error {
	statusItems, fetchErr := s.smsClient.FetchStatus(ctx, jazzAccessToken, []string(job.TrackingIDs))
	if fetchErr != nil {
		now := utils.UTCNow()
		job.RetryCount++
		msg := fetchErr.Error()
		job.Error = &msg
		job.UpdatedAt = now
		if job.RetryCount >= smsStatusJobMaxRetry {
			job.ExecutedAt = &now
		} else {
			job.ExecutedAt = nil
		}
		if err := s.jobRepo.Update(ctx, job); err != nil {
			return err
		}
		return fetchErr
	}

	txErr := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		now := utils.UTCNow()

		statusRows := make([]*models.SMSStatusResult, 0, len(statusItems))
		for _, item := range statusItems {
			// BUG FIX 7: was `job.TrackingIDs[idx]` (positional array index). The provider
			// API is not guaranteed to return results in the same order as the request, so
			// correlating by position silently maps status results to the wrong tracking IDs.
			// Use the TrackingID that the provider echoes back in each response item instead.
			trackingID := strings.TrimSpace(item.TrackingID)
			if trackingID == "" {
				continue
			}
			statusRows = append(statusRows, &models.SMSStatusResult{
				JobID:                 job.ID,
				ProcessedCampaignID:   job.ProcessedCampaignID,
				TrackingID:            trackingID,
				ServerID:              item.ServerID,
				TotalParts:            &item.TotalParts,
				TotalDeliveredParts:   &item.TotalDeliveredParts,
				TotalUndeliveredParts: &item.TotalUndeliveredParts,
				TotalUnknownParts:     &item.TotalUnknownParts,
				Status:                &item.Status,
			})
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

	stats, err := s.updateProcessedCampaignStats(ctx, job.ProcessedCampaignID)
	if err != nil {
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

func (s *SMSCampaignScheduler) updateProcessedCampaignStats(txCtx context.Context, processedCampaignID uint) (map[string]any, error) {
	pc, err := s.pcRepo.ByID(txCtx, processedCampaignID)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, fmt.Errorf("processed campaign not found for processed_campaign_id=%d", processedCampaignID)
	}

	agg, err := s.resRepo.AggregateByCampaign(txCtx, processedCampaignID)
	if err != nil {
		return nil, err
	}

	trackingResults, err := s.resRepo.TrackingResultsByCampaign(txCtx, processedCampaignID)
	if err != nil {
		return nil, err
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
	if err := s.pcRepo.Update(txCtx, pc); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *SMSCampaignScheduler) notifyAdmin(message string) {
	if s.notifier == nil {
		return
	}
	go func(msg string) {
		for _, mobile := range s.adminCfg.ActiveMobiles() {
			_ = s.notifier.SendSMS(context.Background(), mobile, msg, nil)
		}
	}(message)
}

func buildSMSProviderUpdate(trackingID string, resp *PayamSMSResponseItem, sendErr error) repository.SentSMSProviderUpdate {
	update := repository.SentSMSProviderUpdate{
		TrackingID: trackingID,
	}
	if sendErr != nil {
		code := "SEND_BATCH_FAILED"
		desc := sendErr.Error()
		update.ErrorCode = &code
		update.Description = &desc
		return update
	}

	if resp == nil {
		code := "MISSING_SEND_RESPONSE"
		desc := fmt.Sprintf("missing send response for tracking_id=%s", trackingID)
		update.ErrorCode = &code
		update.Description = &desc
		return update
	}

	update.ServerID = resp.ServerID
	update.ErrorCode = resp.ErrorCode
	update.Description = resp.Desc
	return update
}
