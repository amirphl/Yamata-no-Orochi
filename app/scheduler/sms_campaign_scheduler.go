// Package scheduler
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
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
	smsSendBatchSize     = 200 // NOTE: MUST BE LESS THAN 250
	smsStatusJobMaxRetry = 3
)

type SMSCampaignScheduler struct {
	audRepo   repository.AudienceProfileRepository
	tagRepo   repository.TagRepository
	sentRepo  repository.SentSMSRepository
	pcRepo    repository.ProcessedCampaignRepository
	jobRepo   repository.CampaignStatusJobRepository
	resRepo   repository.SMSStatusResultRepository
	statsRepo repository.SrcLayerAllStatsRepository
	notifier  NotificationSender
	logger    *log.Logger
	interval  time.Duration

	db       *gorm.DB
	adminCfg config.AdminConfig
	botCfg   config.BotConfig

	botClient BotClient
	smsClient PayamSMSClient

	logFile *os.File

	schedulerName string

	bundleAudienceCache *BundleAudienceCache
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
	statsRepo repository.SrcLayerAllStatsRepository,
	notifier NotificationSender,
	db *gorm.DB,
	logger *log.Logger,
	interval time.Duration,
	payamSMSCfg config.PayamSMSConfig,
	botCfg config.BotConfig,
	adminCfg config.AdminConfig,
	messageSendMockEnabled bool,
) *SMSCampaignScheduler {
	if interval <= 0 {
		interval = time.Minute
	}

	if botCfg.APIDomain == "" {
		botCfg.APIDomain = defaultBotAPIDomain
	}

	s := &SMSCampaignScheduler{
		audRepo:             audRepo,
		tagRepo:             tagRepo,
		sentRepo:            sentRepo,
		pcRepo:              pcRepo,
		jobRepo:             jobRepo,
		resRepo:             resRepo,
		statsRepo:           statsRepo,
		notifier:            notifier,
		logger:              logger,
		db:                  db,
		interval:            interval,
		adminCfg:            adminCfg,
		botCfg:              botCfg,
		botClient:           newHTTPBotClient(botCfg),
		smsClient:           maybeMockPayamSMSClient(newHTTPPayamSMSClient(payamSMSCfg), messageSendMockEnabled),
		bundleAudienceCache: NewBundleAudienceCache(repository.NewBundleAudienceSelectionRepository(db)),
		schedulerName:       "sms",
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
				func() {
					ctx, cancel := context.WithTimeout(parent, 20*time.Minute) // TODO:
					defer cancel()
					s.runOnce(ctx, parent)
				}()
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
	// recoverStaleUnpreparedCampaigns(ctx, s.db, s.logger, "SMS")
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

	s.dispatchPendingSMSCampaigns(parent, jazzAccessToken, pending, s.processSMSCampaign)
}

type smsCampaignDispatchGroup struct {
	campaigns []dto.BotGetCampaignResponse
}

func groupSMSCampaignsForDispatch(pending []dto.BotGetCampaignResponse) []smsCampaignDispatchGroup {
	groups := make([]smsCampaignDispatchGroup, 0, len(pending))
	groupByKey := make(map[string]int, len(pending))

	for _, camp := range pending {
		key := fmt.Sprintf("campaign:%d", camp.ID)
		if camp.BundleID != nil {
			key = fmt.Sprintf("bundle:%d:%d", camp.CustomerID, *camp.BundleID)
		}

		if idx, ok := groupByKey[key]; ok {
			groups[idx].campaigns = append(groups[idx].campaigns, camp)
			continue
		}

		groupByKey[key] = len(groups)
		groups = append(groups, smsCampaignDispatchGroup{
			campaigns: []dto.BotGetCampaignResponse{camp},
		})
	}

	return groups
}

func (s *SMSCampaignScheduler) dispatchPendingSMSCampaigns(
	parent context.Context,
	jazzAccessToken string,
	pending []dto.BotGetCampaignResponse,
	process func(context.Context, string, dto.BotGetCampaignResponse) error,
) {
	for _, group := range groupSMSCampaignsForDispatch(pending) {
		go func(g smsCampaignDispatchGroup) {
			for _, camp := range g.campaigns {
				if parent.Err() != nil {
					return
				}

				ctx2, cancel2 := context.WithTimeout(parent, campaignExecutionTimeout)
				if err := process(ctx2, jazzAccessToken, camp); err != nil {
					s.logger.Printf("SMS scheduler: process campaign id=%d failed: %v", camp.ID, err)
					s.notifyAdmin(fmt.Sprintf("SMS Scheduler: process campaign failed for campaign id=%d: %v", camp.ID, err))
				}
				cancel2()
			}
		}(group)
	}
}

func (s *SMSCampaignScheduler) processSMSCampaign(ctx context.Context, jazzAccessToken string, c dto.BotGetCampaignResponse) (err error) {
	// Sender from campaign line number
	if c.LineNumber == nil {
		return fmt.Errorf("resolve SMS sender for campaign id=%d: sender is nil", c.ID)
	}
	if _, err := schedulerConfiguredAudienceCount(c); err != nil {
		return err
	}
	sender := *c.LineNumber

	if err := s.botClient.MoveCampaignToRunning(ctx, jazzAccessToken, c.ID); err != nil {
		return fmt.Errorf("move campaign id=%d to running: %w", c.ID, err)
	}
	// defer releaseUnpreparedCampaignOnFailure(s.db, s.logger, "SMS", c.ID, &err)
	s.logger.Printf("SMS scheduler: campaign id=%d moved to running", c.ID)

	// Fetch audience data OUTSIDE any DB transaction.
	// AllocateShortLinks and DownloadTargetAudienceExcelFile are external HTTP calls that can
	// take 60+ seconds for large audiences. Holding a Postgres transaction open during these
	// calls triggers idle_in_transaction_session_timeout, killing the connection with
	// "driver: bad connection" on the next SQL statement.
	var (
		phones                    []string
		ids                       []int64
		uids                      []string
		codes                     []string
		unmatchedUID              []string
		bundleAudienceSelectionID *uint
	)
	if usesExcelAudienceTargeting(c) {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before fetching excel UIDs for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("SMS scheduler: campaign id=%d fetching audience UIDs from excel", c.ID)
		fileUIDs, err := fetchTargetAudienceUIDsFromExcel(ctx, s.botClient, jazzAccessToken, c.ID)
		if err != nil {
			return fmt.Errorf("fetch excel UIDs for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("SMS scheduler: campaign id=%d resolving %d UIDs to phones", c.ID, len(fileUIDs))
		excelShortLinkDomain := ""
		if c.ShortLinkDomain != nil {
			excelShortLinkDomain = *c.ShortLinkDomain
		}
		audienceResult, err := fetchAudiencePhonesByUIDs(ctx, s.logger, s.audRepo, s.botClient, c, jazzAccessToken, fileUIDs, excelShortLinkDomain)
		if err != nil {
			return fmt.Errorf("fetch audience phones by UIDs for campaign id=%d: %w", c.ID, err)
		}
		phones = audienceResult.Phones
		ids = audienceResult.IDs
		uids = audienceResult.UIDs
		codes = audienceResult.Codes
		unmatchedUID = audienceResult.UnmatchedUIDs
		s.logger.Printf("SMS scheduler: campaign id=%d fetched %d phones via excel (unmatched=%d)", c.ID, len(phones), len(unmatchedUID))
	} else {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before fetching audiences for campaign id=%d: %w", c.ID, err)
		}
		// Audience eligibility is platform-neutral; deterministic ordering is
		// enforced by the repository.
		correlationID := uuid.NewString()
		s.logger.Printf("SMS scheduler: campaign id=%d fetching audience phones (correlation_id=%s)", c.ID, correlationID)
		var (
			audienceResult *AudiencePhonesResult
			err            error
		)
		if c.BundleID == nil || *c.BundleID == 0 {
			return fmt.Errorf("campaign id=%d has no bundle", c.ID)
		}
		audienceResult, err = s.fetchSMSAudiencePhonesByBundle(ctx, c, jazzAccessToken, correlationID)
		if err != nil {
			return fmt.Errorf("fetch audience phones for campaign id=%d: %w", c.ID, err)
		}
		phones = audienceResult.Phones
		ids = audienceResult.IDs
		uids = audienceResult.UIDs
		codes = audienceResult.Codes
		bundleAudienceSelectionID, err = bundleSelectionIDFromAudienceResult(audienceResult)
		if err != nil {
			return fmt.Errorf("resolve selection id for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("SMS scheduler: campaign id=%d fetched %d phones (bundle_audience_selection_id=%d)", c.ID, len(phones), *bundleAudienceSelectionID)
	}

	if len(ids) != len(phones) {
		return fmt.Errorf("audience ids mismatch for campaign id=%d: phones=%d ids=%d", c.ID, len(phones), len(ids))
	}
	if len(codes) != len(phones) {
		return fmt.Errorf("audience codes mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}
	s.logger.Printf("SMS scheduler: campaign id=%d audience ready: phones=%d unmatched=%d", c.ID, len(phones), len(unmatchedUID))

	campaignJSON, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal campaign id=%d: %w", c.ID, err)
	}

	// Persist ProcessedCampaign and all audience data in one focused transaction.
	// No external calls here — the transaction stays short and the connection stays active.
	var pc *models.ProcessedCampaign
	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		pc = &models.ProcessedCampaign{
			CampaignID:                c.ID,
			CampaignJSON:              json.RawMessage(campaignJSON),
			AudienceIDs:               pq.Int64Array{},
			AudienceCodes:             []string{},
			LastAudienceID:            nil,
			BundleAudienceSelectionID: bundleAudienceSelectionID,
			Statistics:                nil,
		}
		if err := s.pcRepo.Save(txCtx, pc); err != nil {
			return fmt.Errorf("save processed campaign: %w", err)
		}
		s.logger.Printf("SMS scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

		for start := 0; start < len(ids); start += audienceAppendBatchSize {
			end := min(start+audienceAppendBatchSize, len(ids))
			if err := s.pcRepo.AppendAudienceData(txCtx, pc.ID, ids[start:end], codes[start:end]); err != nil {
				return fmt.Errorf("append audience batch [%d,%d): %w", start, end, err)
			}
		}
		pc.UpdatedAt = utils.UTCNow()
		if err := s.pcRepo.UpdateMeta(txCtx, pc); err != nil {
			return fmt.Errorf("update processed campaign meta: %w", err)
		}
		s.logger.Printf("SMS scheduler: updated processed campaign id=%d with %d audience ids", pc.ID, len(ids))
		return nil
	}); err != nil {
		return fmt.Errorf("persist campaign data for campaign id=%d: %w", c.ID, err)
	}
	s.logger.Printf("SMS scheduler: persisted processed campaign id=%d num_phones=%d, num_ids=%d, num_codes=%d, num_unmatched=%d", pc.ID, len(phones), len(ids), len(codes), len(unmatchedUID))

	if len(unmatchedUID) > 0 {
		s.logger.Printf("SMS scheduler: campaign id=%d creating %d unmatched sent rows for processed_campaign_id=%d", c.ID, len(unmatchedUID), pc.ID)
		if err := s.createUnmatchedSentSMSRows(ctx, pc.ID, unmatchedUID); err != nil {
			return fmt.Errorf("create unmatched sent rows for campaign id=%d: %w", c.ID, err)
		}
	}

	for start := 0; start < len(phones); start += smsSendBatchSize {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired at batch start=%d for campaign id=%d: %w", start, c.ID, err)
		}

		end := min(start+smsSendBatchSize, len(phones))
		batchPhones := phones[start:end]
		batchIDs := ids[start:end]
		batchUIDs := uids[start:end]
		batchCodes := codes[start:end]

		items := make([]PayamSMSItem, 0, len(batchPhones))
		rows := make([]*models.SentSMS, 0, len(batchPhones))

		s.logger.Printf("SMS scheduler: campaign id=%d allocating tracking ids for batch [%d,%d)", c.ID, start, end)
		trackingIDs, err := allocateTrackingIDs(ctx, s.db, len(batchPhones))
		if err != nil {
			return fmt.Errorf("allocate tracking ids for batch [%d,%d) campaign id=%d: %w", start, end, c.ID, err)
		}

		for i, p := range batchPhones {
			body := s.buildSMSBody(c, batchCodes[i], batchUIDs[i])
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

		lastBatchID := batchIDs[len(batchIDs)-1]
		if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
			if len(rows) > 0 {
				if err := s.sentRepo.SaveBatch(txCtx, rows); err != nil {
					return fmt.Errorf("save batch rows: %w", err)
				}
			}
			pc.LastAudienceID = utils.ToPtr(lastBatchID)
			pc.UpdatedAt = utils.UTCNow()
			if err := s.pcRepo.UpdateMeta(txCtx, pc); err != nil {
				return fmt.Errorf("update meta: %w", err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("save batch [%d,%d) for campaign id=%d: %w", start, end, c.ID, err)
		}
		s.logger.Printf("SMS scheduler: campaign id=%d batch [%d,%d) saved, sending to SMS provider", c.ID, start, end)

		batchResult, batchErr := s.smsClient.SendBatch(ctx, sender, items)
		if batchErr != nil {
			s.logger.Printf("SMS scheduler: send batch [%d,%d) failed for campaign id=%d: %v", start, end, c.ID, batchErr)
			// TODO: How to handle this error? Retry sending? Skip to next batch?
		}
		if auditErr := s.persistPayamSMSSendResponse(ctx, pc.ID, items, batchResult, batchErr); auditErr != nil {
			s.logger.Printf("SMS scheduler: failed to persist PayamSMS send response for campaign id=%d batch [%d,%d): %v", c.ID, start, end, auditErr)
			s.notifyAdmin(fmt.Sprintf("SMS Scheduler: failed to persist PayamSMS send response for campaign id=%d: %v", c.ID, auditErr))
		}

		responseByTrackingID := make(map[string]*PayamSMSResponseItem, len(batchResult.Items))
		for i := range batchResult.Items {
			resp := batchResult.Items[i]
			trackingID := strings.TrimSpace(resp.TrackingID)
			if trackingID == "" {
				continue
			}
			respCopy := resp
			responseByTrackingID[trackingID] = &respCopy
		}
		s.logger.Printf("SMS scheduler: campaign id=%d batch [%d,%d) SMS provider responded: sent=%d responses=%d", c.ID, start, end, len(items), len(batchResult.Items))

		sendUpdates := make([]repository.SentSMSProviderUpdate, 0, len(items))
		for _, item := range items {
			trackingID := strings.TrimSpace(item.TrackingID)
			if trackingID == "" {
				continue
			}
			sendUpdates = append(sendUpdates, buildSMSProviderUpdate(trackingID, responseByTrackingID[trackingID], batchErr))
		}
		if len(sendUpdates) > 0 {
			if updateErr := s.sentRepo.UpdateProviderFieldsByTrackingIDs(ctx, sendUpdates); updateErr != nil {
				s.logger.Printf("SMS scheduler: failed to batch update sent_sms provider fields for campaign id=%d: %v", c.ID, updateErr)
				// NOTE: Error silent here; not returning to avoid blocking further processing
			}
		}

		if err := s.scheduleStatusCheckJobs(ctx, pc.ID, trackingIDs); err != nil {
			s.logger.Printf("SMS scheduler: failed to schedule status jobs for campaign id=%d: %v", c.ID, err)
			// NOTE: Error silent here; not returning to avoid blocking further processing
		}
		s.logger.Printf("SMS scheduler: campaign id=%d batch [%d,%d) done", c.ID, start, end)
	}

	stats, err := preparedCampaignStatistics(ctx, s.pcRepo, pc, len(phones), s.updateProcessedCampaignStats)
	if err != nil {
		return fmt.Errorf("update stats for campaign id=%d: %w", c.ID, err)
	}
	if shouldPushPreparedCampaignStatistics(stats, len(phones)) {
		if err := s.botClient.PushCampaignStatistics(ctx, c.ID, stats); err != nil {
			return fmt.Errorf("push statistics for campaign id=%d: %w", c.ID, err)
		}
	}

	s.logger.Printf("SMS scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, jazzAccessToken, c.ID); err != nil {
		return fmt.Errorf("move campaign id=%d to executed: %w", c.ID, err)
	}
	s.logger.Printf("SMS scheduler: campaign id=%d moved to executed", c.ID)

	if len(uids) > 0 {
		go func(campaignID uint, uids, codes []string) {
			pushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if err := s.botClient.PushCampaignAudienceUIDs(pushCtx, campaignID, uids, codes); err != nil {
				s.logger.Printf("SMS scheduler: push audience UIDs failed for campaign id=%d: %v", campaignID, err)
				s.notifyAdmin(fmt.Sprintf("SMS Scheduler: push audience UIDs failed for campaign id=%d: %v", campaignID, err))
			}
		}(c.ID, uids, codes)
	}

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

// resolveScoreConstraint looks up the percentile thresholds from src_layer_all_stats
// for the campaign's levels and converts AudienceGrades into a score filter.
// Returns nil when no filter is needed (grades are empty or [A,B,C]). A
// restricted grade set fails closed when percentile statistics are missing.
func (s *SMSCampaignScheduler) resolveScoreConstraint(ctx context.Context, c dto.BotGetCampaignResponse) (*models.NormalizedScoreConstraint, error) {
	if usesSmartAudienceTargeting(c) {
		return nil, nil
	}
	if campaignIgnoresAudienceGrades(c) {
		s.logger.Printf("resolveScoreConstraint: campaign id=%d tag_id=%d bypasses audience grade filter", c.ID, audienceGradeExemptTagID)
		return nil, nil
	}
	if !gradesNeedScoreFilter(c.AudienceGrades) {
		return nil, nil
	}
	if s.statsRepo == nil {
		return nil, fmt.Errorf("audience score statistics repository is unavailable for campaign id=%d with grades=%v", c.ID, c.AudienceGrades)
	}
	percentiles, err := s.statsRepo.FetchPercentiles(ctx, c.Level1, c.Level2s, c.Level3s)
	if err != nil {
		return nil, fmt.Errorf("fetch percentiles for campaign id=%d: %w", c.ID, err)
	}
	if percentiles == nil {
		return nil, fmt.Errorf("audience score statistics are missing for campaign id=%d levels and grades=%v", c.ID, c.AudienceGrades)
	}
	s.logger.Printf("resolveScoreConstraint: campaign id=%d grades=%v p33=%.4f p66=%.4f", c.ID, c.AudienceGrades, percentiles.P33, percentiles.P66)
	return gradesToScoreConstraint(c.AudienceGrades, percentiles.P33, percentiles.P66), nil
}

// selectTagAudiences preserves SMS eligibility and priority: white recipients
// are selected first, then pink recipients fill any remaining capacity. Black
// recipients are intentionally excluded from standard SMS campaigns.
func (s *SMSCampaignScheduler) selectTagAudiences(
	ctx context.Context,
	campaignID uint,
	tagIDs pq.Int32Array,
	numAudiences int64,
	exclude map[int64]struct{},
	excludeBundleID *uint,
	scoreConstraint *models.NormalizedScoreConstraint,
) (phones []string, ids []int64, uids []string, err error) {
	if numAudiences <= 0 {
		return []string{}, []int64{}, []string{}, nil
	}
	limit, err := checkedAudienceQueryLimit(numAudiences)
	if err != nil {
		return nil, nil, nil, err
	}

	phones = make([]string, 0, numAudiences)
	ids = make([]int64, 0, numAudiences)
	uids = make([]string, 0, numAudiences)
	excludeIDs := audienceIDsFromSet(exclude)

	appendCandidate := func(ap *models.AudienceProfile) {
		if ap == nil || ap.PhoneNumber == nil || strings.TrimSpace(*ap.PhoneNumber) == "" {
			return
		}
		phones = append(phones, strings.TrimSpace(*ap.PhoneNumber))
		ids = append(ids, int64(ap.ID))
		uids = append(uids, ap.UID)
	}

	selectColor := func(color string, colorLimit int) error {
		if colorLimit <= 0 {
			return nil
		}
		candidates, err := s.audRepo.SelectCampaignCandidates(ctx, models.AudienceProfileFilter{
			Tags:            &tagIDs,
			Color:           utils.ToPtr(color),
			NormalizedScore: scoreConstraint,
			ExcludeBundleID: excludeBundleID,
		}, excludeIDs, colorLimit)
		if err != nil {
			return err
		}
		s.logger.Printf("selectTagAudiences %s candidates: campaign_id=%d count=%d limit=%d excluded=%d", color, campaignID, len(candidates), colorLimit, len(excludeIDs))
		for _, ap := range candidates {
			appendCandidate(ap)
		}
		return nil
	}

	if err := selectColor("white", limit); err != nil {
		s.logger.Printf("selectTagAudiences fetch white failed: campaign_id=%d err=%v", campaignID, err)
		return nil, nil, nil, err
	}
	remaining := limit - len(ids)
	if err := selectColor("pink", remaining); err != nil {
		s.logger.Printf("selectTagAudiences fetch pink failed: campaign_id=%d err=%v", campaignID, err)
		return nil, nil, nil, err
	}

	return phones, ids, uids, nil
}

// fetchSMSAudiencePhonesByBundle selects audiences for a campaign that belongs to a bundle.
// Uniqueness is enforced across all campaigns in the bundle:
// audiences already selected by earlier campaigns in the same bundle are excluded.
// Unlike the tag-hash path there is no rolling-window reset. Selection and
// persistence run under the Bundle lock and require the exact requested count.
func (s *SMSCampaignScheduler) fetchSMSAudiencePhonesByBundle(
	ctx context.Context,
	c dto.BotGetCampaignResponse,
	jazzAccessToken string,
	correlationID string,
) (*AudiencePhonesResult, error) {
	bundleID := *c.BundleID
	numAudiences, err := schedulerConfiguredAudienceCount(c)
	if err != nil {
		return nil, err
	}
	s.logger.Printf("fetchSMSAudiencePhonesByBundle start: campaign_id=%d customer_id=%d bundle_id=%d num_audiences=%d correlation_id=%s",
		c.ID, c.CustomerID, bundleID, numAudiences, correlationID)

	executionTags, tagIDs, err := resolveActiveCampaignTagIDs(ctx, s.tagRepo, c)
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhonesByBundle tags resolution failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}
	s.logger.Printf("fetchSMSAudiencePhonesByBundle tags resolved: campaign_id=%d requested=%d resolved=%d", c.ID, len(executionTags), len(tagIDs))

	scoreConstraint, err := s.resolveScoreConstraint(ctx, c)
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhonesByBundle resolve score constraint failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}

	var phones []string
	var ids []int64
	var uids []string
	var selectionID uint
	if usesSmartAudienceTargeting(c) {
		phones, ids, uids, selectionID, err = selectAndReserveExactSmartTargetingCandidates(ctx, s.db, c, numAudiences, correlationID)
	} else {
		phones, ids, uids, selectionID, err = selectAndReserveStandardBundleCandidates(
			ctx, s.db, s.bundleAudienceCache, c.ID, c.CustomerID, bundleID, numAudiences, correlationID,
			func(selectionCtx context.Context, exclude map[int64]struct{}) ([]string, []int64, []string, error) {
				return s.selectTagAudiences(selectionCtx, c.ID, tagIDs, numAudiences, exclude, &bundleID, scoreConstraint)
			},
			func(selectionCtx context.Context, ids []int64) ([]string, []int64, []string, error) {
				return loadReservedBundleAudience(selectionCtx, s.audRepo, ids)
			},
		)
	}
	if err != nil {
		return nil, err
	}
	if selectionID == 0 {
		return nil, fmt.Errorf("bundle audience selection was not persisted for campaign %d", c.ID)
	}
	s.logger.Printf("fetchSMSAudiencePhonesByBundle selected: campaign_id=%d bundle_id=%d selected=%d requested=%d",
		c.ID, bundleID, len(phones), numAudiences)
	if err := validateSchedulerSelectedAudienceCount(c, numAudiences, len(ids)); err != nil {
		return nil, err
	}

	s.logger.Printf("fetchSMSAudiencePhonesByBundle selection saved: campaign_id=%d bundle_id=%d selection_id=%d selected=%d",
		c.ID, bundleID, selectionID, len(ids))
	if len(phones) == 0 {
		return &AudiencePhonesResult{
			Phones:                    phones,
			IDs:                       ids,
			UIDs:                      uids,
			Codes:                     []string{},
			BundleAudienceSelectionID: utils.ToPtr(selectionID),
		}, nil
	}

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchSMSAudiencePhonesByBundle skipped short links: campaign_id=%d ad_link=empty", c.ID)
		return &AudiencePhonesResult{
			Phones:                    phones,
			IDs:                       ids,
			UIDs:                      uids,
			Codes:                     make([]string, len(phones)),
			BundleAudienceSelectionID: utils.ToPtr(selectionID),
		}, nil
	}

	if c.ShortLinkDomain == nil || strings.TrimSpace(*c.ShortLinkDomain) == "" {
		s.logger.Printf("fetchSMSAudiencePhonesByBundle skipped short links: campaign_id=%d short_link_domain=empty", c.ID)
		return &AudiencePhonesResult{
			Phones:                    phones,
			IDs:                       ids,
			UIDs:                      uids,
			Codes:                     make([]string, len(phones)),
			BundleAudienceSelectionID: utils.ToPtr(selectionID),
		}, nil
	}

	items := make([]dto.PhoneWithAdLink, len(phones))
	for i, p := range phones {
		adLink := c.AdLink
		if adLink != nil && strings.Contains(*adLink, "{uid}") {
			resolved := strings.ReplaceAll(*adLink, "{uid}", uids[i])
			adLink = &resolved
		}
		items[i] = dto.PhoneWithAdLink{Phone: p, AdLink: adLink}
	}
	codes, err := s.botClient.AllocateShortLinks(ctx, jazzAccessToken, &dto.BotAllocateShortLinksRequest{
		CampaignID:      c.ID,
		Items:           items,
		ShortLinkDomain: *c.ShortLinkDomain,
	})
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhonesByBundle allocate short links failed: campaign_id=%d bundle_id=%d err=%v", c.ID, bundleID, err)
		return nil, err
	}
	if len(codes) != len(phones) {
		return nil, fmt.Errorf("allocate short links length mismatch for campaign id=%d bundle_id=%d: phones=%d codes=%d", c.ID, bundleID, len(phones), len(codes))
	}
	s.logger.Printf("fetchSMSAudiencePhonesByBundle success: campaign_id=%d bundle_id=%d selected=%d codes=%d selection_id=%d",
		c.ID, bundleID, len(phones), len(codes), selectionID)
	return &AudiencePhonesResult{
		Phones:                    phones,
		IDs:                       ids,
		UIDs:                      uids,
		Codes:                     codes,
		BundleAudienceSelectionID: utils.ToPtr(selectionID),
	}, nil
}

func (s *SMSCampaignScheduler) buildSMSBody(c dto.BotGetCampaignResponse, code string, uid string) string {
	content := ""
	if c.Content != nil {
		content = *c.Content
	}
	if hasCampaignAdLink(c.AdLink) {
		if c.ShortLinkDomain != nil && *c.ShortLinkDomain != "" {
			domain := *c.ShortLinkDomain
			if !strings.HasSuffix(domain, "/") {
				domain += "/"
			}
			shortened := domain + code
			return strings.ReplaceAll(content, "{YOUR_LINK}", shortened) + "\n" + "لغو۱۱"
		}
		injected := strings.ReplaceAll(*c.AdLink, "{uid}", uid)
		return strings.ReplaceAll(content, "{YOUR_LINK}", injected) + "\n" + "لغو۱۱"
	}
	return strings.ReplaceAll(content, "{YOUR_LINK}", "") + "\n" + "لغو۱۱"
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
			Platform:            models.CampaignPlatformSMS,
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
		if stats["aggregatedTotalSent"] != nil && stats["aggregatedTotalSent"].(int64) > 0 {
			if err := s.botClient.PushCampaignStatistics(ctx, pc.CampaignID, stats); err != nil {
				return err
			}
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
	offsets := []time.Duration{1 * time.Minute, 5 * time.Minute, 15 * time.Minute, 24 * time.Hour, 48 * time.Hour}
	jobs := make([]*models.CampaignStatusJob, 0, len(offsets))
	for _, off := range offsets {
		jobs = append(jobs, &models.CampaignStatusJob{
			ProcessedCampaignID: processedCampaignID,
			CorrelationID:       corrID,
			Platform:            models.CampaignPlatformSMS,
			TrackingIDs:         pq.StringArray(filteredTrackingIDs),
			RetryCount:          0,
			ScheduledAt:         now.Add(off),
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return s.jobRepo.SaveBatch(ctx, jobs)
}

func (s *SMSCampaignScheduler) startStatusJobWorker(parent context.Context) {
	ticker := time.NewTicker(statusJobWorkerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-parent.Done():
			return
		case <-ticker.C:
			if s.jobRepo == nil || s.resRepo == nil {
				continue
			}

			listCtx, listCancel := context.WithTimeout(parent, 30*time.Second)
			jobs, err := s.jobRepo.ListDue(listCtx, models.CampaignPlatformSMS, utils.UTCNow(), numJobsPerTick)
			listCancel()
			if err != nil {
				s.logger.Printf("SMS scheduler: list status jobs failed: %v", err)
				continue
			}
			if len(jobs) == 0 {
				continue
			}

			tokenCtx, tokenCancel := context.WithTimeout(parent, 30*time.Second)
			atiehAccessToken, err := s.smsClient.GetToken(tokenCtx)
			tokenCancel()
			if err != nil {
				s.logger.Printf("SMS scheduler: payamsms token for status jobs failed: %v", err)
				continue
			}

			for i, job := range jobs {
				if parent.Err() != nil {
					return
				}

				jobCtx, jobCancel := context.WithTimeout(parent, 2*time.Minute)
				err := s.handleStatusJob(jobCtx, job, atiehAccessToken)
				jobCancel()

				if err != nil {
					s.logger.Printf("SMS scheduler: handle status job id=%d failed: %v", job.ID, err)
					if job.RetryCount >= smsStatusJobMaxRetry {
						s.notifyAdmin(fmt.Sprintf("SMS scheduler: status job id=%d has failed %d times with error: %v", job.ID, job.RetryCount, err))
					}
				} else {
					s.logger.Printf("SMS scheduler: handle status job id=%d succeeded", job.ID)
				}

				if i < len(jobs)-1 {
					if err := sleepWithContext(parent, time.Second); err != nil {
						return
					}
				}
			}
		}
	}
}

func (s *SMSCampaignScheduler) handleStatusJob(ctx context.Context, job *models.CampaignStatusJob, jazzAccessToken string) error {
	statusResult, fetchErr := s.smsClient.FetchStatus(ctx, jazzAccessToken, []string(job.TrackingIDs))
	job.RawProviderResponse = statusResult.RawResponse
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
	statusItems := statusResult.Items

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
		if shouldPushCurrentProcessedCampaignStatistics(pc, stats) {
			if err := s.botClient.PushCampaignStatistics(ctx, pc.CampaignID, stats); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SMSCampaignScheduler) updateProcessedCampaignStats(ctx context.Context, processedCampaignID uint) (map[string]any, error) {
	pc, err := s.pcRepo.ByID(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, fmt.Errorf("processed campaign not found for processed_campaign_id=%d", processedCampaignID)
	}

	agg, err := s.resRepo.AggregateByCampaign(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}

	// trackingResults, err := s.resRepo.TrackingResultsByCampaign(ctx, processedCampaignID)
	// if err != nil {
	// 	return nil, err
	// }

	// Fallback before any status jobs land.
	// if agg.AggregatedTotalRecords == 0 && len(trackingResults) == 0 {
	if agg.AggregatedTotalRecords == 0 {
		// s.logger.Printf("updateProcessedCampaignStats: no status results yet for processed_campaign_id=%d, falling back to sent rows", processedCampaignID)
		// return s.updateProcessedCampaignStatsFromSentRows(ctx, pc)
		return nil, nil
	}

	stats := map[string]any{
		"aggregatedTotalRecords":          agg.AggregatedTotalRecords,
		"aggregatedTotalSent":             agg.AggregatedTotalSent,
		"aggregatedTotalParts":            agg.AggregatedTotalParts,
		"aggregatedTotalDeliveredParts":   agg.AggregatedDeliveredParts,
		"aggregatedTotalUnDeliveredParts": agg.AggregatedUndelivered,
		"aggregatedTotalUnKnownParts":     agg.AggregatedUnknown,
		// "trackingResults":                 trackingResults,
		"updatedAt": utils.UTCNow().Format(time.RFC3339),
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}
	pc.Statistics = data
	pc.UpdatedAt = utils.UTCNow()
	if err := s.pcRepo.UpdateMeta(ctx, pc); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *SMSCampaignScheduler) updateProcessedCampaignStatsFromSentRows(ctx context.Context, pc *models.ProcessedCampaign) (map[string]any, error) {
	s.logger.Printf("updateProcessedCampaignStatsFromSentRows: computing stats from sent rows for processed_campaign_id=%d", pc.ID)
	type row struct {
		Total      int64
		Successful int64
	}
	var agg row
	if err := s.db.WithContext(ctx).Table("sent_sms").
		Select(`
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN LOWER(BTRIM(status::text)) = 'successful' THEN 1 ELSE 0 END), 0) AS successful`).
		Where("processed_campaign_id = ?", pc.ID).
		Scan(&agg).Error; err != nil {
		return nil, err
	}

	// trackingResults, err := s.sentRepo.TrackingResultsFromSentRows(ctx, pc.ID)
	// if err != nil {
	// 	return nil, err
	// }

	stats := map[string]any{
		"aggregatedTotalRecords":          agg.Total,
		"aggregatedTotalSent":             agg.Successful,
		"aggregatedTotalParts":            agg.Total,
		"aggregatedTotalDeliveredParts":   agg.Successful,
		"aggregatedTotalUnDeliveredParts": agg.Total - agg.Successful,
		"aggregatedTotalUnKnownParts":     int64(0),
		// "trackingResults":                 trackingResults,
		"updatedAt": utils.UTCNow().Format(time.RFC3339),
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}
	pc.Statistics = data
	pc.UpdatedAt = utils.UTCNow()
	if err := s.pcRepo.UpdateMeta(ctx, pc); err != nil {
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

func buildPayamSMSSendResponse(
	processedCampaignID uint,
	items []PayamSMSItem,
	result PayamSMSSendResult,
	sendErr error,
) (*models.PayamSMSSendResponse, error) {
	trackingIDs := make(pq.StringArray, 0, len(items))
	for _, item := range items {
		if trackingID := strings.TrimSpace(item.TrackingID); trackingID != "" {
			trackingIDs = append(trackingIDs, trackingID)
		}
	}

	headers := result.ResponseHeaders
	if headers == nil {
		headers = make(map[string][]string)
	}
	responseHeaders, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("marshal PayamSMS response headers: %w", err)
	}

	var errorMessage *string
	if sendErr != nil {
		message := sendErr.Error()
		errorMessage = &message
	}

	return &models.PayamSMSSendResponse{
		ProcessedCampaignID: processedCampaignID,
		TrackingIDs:         trackingIDs,
		HTTPStatusCode:      result.HTTPStatusCode,
		ResponseHeaders:     responseHeaders,
		ResponseBody:        result.RawResponse,
		Error:               errorMessage,
		AttemptCount:        result.AttemptCount,
	}, nil
}

// persistPayamSMSSendResponse deliberately detaches the audit write from a
// canceled campaign context. A timeout/transport cancellation is itself one of
// the failures this table is intended to preserve.
func (s *SMSCampaignScheduler) persistPayamSMSSendResponse(
	ctx context.Context,
	processedCampaignID uint,
	items []PayamSMSItem,
	result PayamSMSSendResult,
	sendErr error,
) error {
	row, err := buildPayamSMSSendResponse(processedCampaignID, items, result, sendErr)
	if err != nil {
		return err
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	return s.db.WithContext(persistCtx).Create(row).Error
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
