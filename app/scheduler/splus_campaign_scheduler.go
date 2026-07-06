// Package scheduler implements campaign scheduling and execution for Splus campaigns,
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
	defaultSplusBaseURL = "https://bui.splus.ir"
	splusSendMaxRetries = 5
	splusMediaMaxSize   = int64(8 * 1024 * 1024)
	splusSendBatchSize  = 200
)

type SplusCampaignScheduler struct {
	audRepo   repository.AudienceProfileRepository
	tagRepo   repository.TagRepository
	sentRepo  repository.SentSplusMessageRepository
	pcRepo    repository.ProcessedCampaignRepository
	jobRepo   repository.CampaignStatusJobRepository
	resRepo   repository.SplusStatusResultRepository
	statsRepo repository.SrcLayerAllStatsRepository
	notifier  NotificationSender
	logger    *log.Logger
	interval  time.Duration

	messageDelay time.Duration

	db       *gorm.DB
	adminCfg config.AdminConfig
	splusCfg config.SplusConfig
	botCfg   config.BotConfig

	botClient   BotClient
	splusClient SplusClient

	logFile *os.File

	schedulerName string

	audienceCache       *AudienceCache
	bundleAudienceCache *BundleAudienceCache
}

func NewSplusCampaignScheduler(
	audRepo repository.AudienceProfileRepository,
	tagRepo repository.TagRepository,
	sentRepo repository.SentSplusMessageRepository,
	pcRepo repository.ProcessedCampaignRepository,
	jobRepo repository.CampaignStatusJobRepository,
	resRepo repository.SplusStatusResultRepository,
	statsRepo repository.SrcLayerAllStatsRepository,
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
		messageDelay:        messageDelay,
		adminCfg:            adminCfg,
		splusCfg:            splusCfg,
		botCfg:              botCfg,
		botClient:           newHTTPBotClient(botCfg),
		splusClient:         newHTTPSplusClient(splusCfg),
		audienceCache:       NewAudienceCache(repository.NewAudienceSelectionRepository(db)),
		bundleAudienceCache: NewBundleAudienceCache(repository.NewBundleAudienceSelectionRepository(db)),
		schedulerName:       "splus",
	}

	if err := s.initSchedulerLogger(); err != nil {
		s.logger = log.New(io.Discard, "splus_scheduler ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		s.logger.Printf("Splus scheduler: failed to initialize file logger: %v", err)
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

func (s *SplusCampaignScheduler) runOnce(ctx context.Context, parent context.Context) {
	jazzAccessToken, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("Splus scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Splus scheduler: bot login failed: %v", err))
		return
	}

	ready, err := s.botClient.ListReadyCampaigns(ctx, jazzAccessToken, models.CampaignPlatformSPlus)
	if err != nil {
		s.logger.Printf("Splus scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Splus scheduler: list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}
	s.logger.Printf("Splus scheduler: listed %d ready campaigns", len(ready))

	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformSPlus {
			s.logger.Printf("Splus scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform)
			s.notifyAdmin(fmt.Sprintf("Splus scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform))
			continue
		}
		if err := s.validateSplusCampaign(c); err != nil {
			s.logger.Printf("Splus scheduler: validate campaign failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Splus scheduler: validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("Splus scheduler: check processed failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Splus scheduler: check processed failed for id=%d: %v", c.ID, err))
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		} else {
			s.logger.Printf("Splus scheduler: campaign id=%d already processed, skipping", c.ID)
		}
	}
	if len(pending) == 0 {
		return
	}
	s.logger.Printf("Splus scheduler: %d campaigns pending processing...", len(pending))

	for _, camp := range pending {
		go func(c dto.BotGetCampaignResponse) {
			// TODO: Make 4 hours configurable or use a more dynamic approach based on campaign content/size
			ctx2, cancel2 := context.WithTimeout(parent, 4*time.Hour)
			defer cancel2()
			if err := s.processSplusCampaign(ctx2, jazzAccessToken, c); err != nil {
				s.logger.Printf("Splus scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("Splus scheduler: process campaign failed for campaign id=%d: %v", c.ID, err))
			}
		}(camp)
	}
}

func (s *SplusCampaignScheduler) processSplusCampaign(ctx context.Context, jazzAccessToken string, c dto.BotGetCampaignResponse) error {
	botID, err := extractSplusBotID(c)
	if err != nil {
		return fmt.Errorf("resolve Splus bot id for campaign id=%d: %w", c.ID, err)
	}
	if c.NumAudiences == nil || *c.NumAudiences <= 0 {
		return fmt.Errorf("campaign id=%d has no audiences", c.ID)
	}

	if err := s.botClient.MoveCampaignToRunning(ctx, jazzAccessToken, c.ID); err != nil {
		return fmt.Errorf("move campaign id=%d to running: %w", c.ID, err)
	}
	s.logger.Printf("Splus scheduler: campaign id=%d moved to running", c.ID)

	// Fetch audience data OUTSIDE any DB transaction.
	// AllocateShortLinks and DownloadTargetAudienceExcelFile are external HTTP calls that can
	// take 60+ seconds for large audiences. Holding a Postgres transaction open during these
	// calls triggers idle_in_transaction_session_timeout, killing the connection with
	// "driver: bad connection" on the next SQL statement.
	var (
		phones       []string
		ids          []int64
		uids         []string
		codes        []string
		unmatchedUID []string
		selectionID  *uint
	)
	if hasTargetAudienceExcelFileUUID(c.TargetAudienceExcelFileUUID) {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before fetching excel UIDs for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("Splus scheduler: campaign id=%d fetching audience UIDs from excel", c.ID)
		fileUIDs, err := fetchTargetAudienceUIDsFromExcel(ctx, s.botClient, jazzAccessToken, c.ID)
		if err != nil {
			return fmt.Errorf("fetch excel UIDs for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("Splus scheduler: campaign id=%d resolving %d UIDs to phones", c.ID, len(fileUIDs))
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
		selectionID = nil
		s.logger.Printf("Splus scheduler: campaign id=%d fetched %d phones via excel (unmatched=%d)", c.ID, len(phones), len(unmatchedUID))
	} else {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before fetching audiences for campaign id=%d: %w", c.ID, err)
		}
		correlationID := uuid.NewString()
		s.logger.Printf("Splus scheduler: campaign id=%d fetching audience phones (correlation_id=%s)", c.ID, correlationID)
		var (
			audienceResult *AudiencePhonesResult
			err            error
		)
		if c.BundleID != nil {
			audienceResult, err = s.fetchSplusAudiencePhonesByBundle(ctx, c, jazzAccessToken, correlationID)
		} else {
			audienceResult, err = s.fetchSplusAudiencePhones(ctx, c, jazzAccessToken, correlationID)
		}
		if err != nil {
			return fmt.Errorf("fetch audience phones for campaign id=%d: %w", c.ID, err)
		}
		phones = audienceResult.Phones
		ids = audienceResult.IDs
		uids = audienceResult.UIDs
		codes = audienceResult.Codes
		selectionID = utils.ToPtr(audienceResult.SelectionID)
		s.logger.Printf("Splus scheduler: campaign id=%d fetched %d phones (selection_id=%d)", c.ID, len(phones), audienceResult.SelectionID)
	}

	if len(ids) != len(phones) {
		return fmt.Errorf("audience ids mismatch for campaign id=%d: phones=%d ids=%d", c.ID, len(phones), len(ids))
	}
	if len(codes) != len(phones) {
		return fmt.Errorf("audience codes mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}
	s.logger.Printf("Splus scheduler: campaign id=%d audience ready: phones=%d unmatched=%d", c.ID, len(phones), len(unmatchedUID))

	campaignJSON, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal campaign id=%d: %w", c.ID, err)
	}

	// Persist ProcessedCampaign and all audience data in one focused transaction.
	// No external calls here — the transaction stays short and the connection stays active.
	var pc *models.ProcessedCampaign
	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		pc = &models.ProcessedCampaign{
			CampaignID:          c.ID,
			CampaignJSON:        json.RawMessage(campaignJSON),
			AudienceIDs:         pq.Int64Array{},
			AudienceCodes:       []string{},
			LastAudienceID:      nil,
			AudienceSelectionID: selectionID,
			Statistics:          nil,
		}
		if err := s.pcRepo.Save(txCtx, pc); err != nil {
			return fmt.Errorf("save processed campaign: %w", err)
		}
		s.logger.Printf("Splus scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

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
		s.logger.Printf("Splus scheduler: updated processed campaign id=%d with %d audience ids", pc.ID, len(ids))
		return nil
	}); err != nil {
		return fmt.Errorf("persist campaign data for campaign id=%d: %w", c.ID, err)
	}
	s.logger.Printf("Splus scheduler: persisted processed campaign id=%d num_phones=%d, num_ids=%d, num_codes=%d, num_unmatched=%d", pc.ID, len(phones), len(ids), len(codes), len(unmatchedUID))

	if len(unmatchedUID) > 0 {
		s.logger.Printf("Splus scheduler: campaign id=%d creating %d unmatched sent rows for processed_campaign_id=%d", c.ID, len(unmatchedUID), pc.ID)
		if err := s.createUnmatchedSentSplusRows(ctx, pc.ID, unmatchedUID); err != nil {
			return fmt.Errorf("create unmatched sent rows for campaign id=%d: %w", c.ID, err)
		}
	}

	var fileID *string
	if c.MediaUUID != nil {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before uploading media for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("Splus scheduler: campaign id=%d uploading media uuid=%s", c.ID, c.MediaUUID)
		// TODO: Remove botID
		id, err := s.uploadCampaignMedia(ctx, jazzAccessToken, botID, c)
		if err != nil {
			return fmt.Errorf("upload media for campaign id=%d: %w", c.ID, err)
		}
		fileID = id
		s.logger.Printf("Splus scheduler: campaign id=%d media uploaded file_id=%v", c.ID, fileID)
	}

	for start := 0; start < len(phones); start += splusSendBatchSize {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired at batch start=%d for campaign id=%d: %w", start, c.ID, err)
		}

		end := min(start+splusSendBatchSize, len(phones))
		batchPhones := phones[start:end]
		batchIDs := ids[start:end]
		batchUIDs := uids[start:end]
		batchCodes := codes[start:end]

		items := make([]SplusSendMessageRequest, 0, len(batchPhones))
		rows := make([]*models.SentSplusMessage, 0, len(batchPhones))

		s.logger.Printf("Splus scheduler: campaign id=%d allocating tracking ids for batch [%d,%d)", c.ID, start, end)
		trackingIDs, err := allocateTrackingIDs(ctx, s.db, len(batchPhones))
		if err != nil {
			return fmt.Errorf("allocate tracking ids for batch [%d,%d) campaign id=%d: %w", start, end, c.ID, err)
		}

		for i, p := range batchPhones {
			trackingID := trackingIDs[i]
			items = append(items, SplusSendMessageRequest{
				PhoneNumber: p,
				Text:        s.buildSplusMessageBody(c, batchCodes[i], batchUIDs[i]),
				FileID:      fileID,
			})
			rows = append(rows, &models.SentSplusMessage{
				ProcessedCampaignID: pc.ID,
				PhoneNumber:         p,
				PartsDelivered:      0,
				Status:              models.SplusSendStatusPending,
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
		s.logger.Printf("Splus scheduler: campaign id=%d batch [%d,%d) saved, sending to Splus", c.ID, start, end)

		sendUpdates := make([]repository.SentSplusSendResultUpdate, 0, len(items))
		for i := range items {
			resp, sendErr := s.sendWithRetry(ctx, botID, &items[i])
			status, parts, serverID, errorCode, desc := s.resolveSendResult(resp, sendErr)
			sendUpdates = append(sendUpdates, repository.SentSplusSendResultUpdate{
				TrackingID:     trackingIDs[i],
				Status:         status,
				PartsDelivered: parts,
				ServerID:       serverID,
				ErrorCode:      errorCode,
				Description:    desc,
			})
		}

		if len(sendUpdates) > 0 {
			if updateErr := s.sentRepo.UpdateSendResultByTrackingIDs(ctx, sendUpdates); updateErr != nil {
				s.logger.Printf("Splus scheduler: failed to batch update sent_splus provider fields for campaign id=%d: %v", c.ID, updateErr)
				// NOTE: Error silent here; not returning to avoid blocking further processing.
			}
		}

		if err := s.scheduleStatusCheckJobs(ctx, pc.ID, trackingIDs); err != nil {
			s.logger.Printf("Splus scheduler: failed to schedule status jobs for campaign id=%d: %v", c.ID, err)
			// NOTE: Error silent here; not returning to avoid blocking further processing
		}
		s.logger.Printf("Splus scheduler: campaign id=%d batch [%d,%d) done, sleeping message_delay", c.ID, start, end)
		if err := sleepWithContext(ctx, s.messageDelay); err != nil {
			return fmt.Errorf("interrupted during batch delay at [%d,%d) for campaign id=%d: %w", start, end, c.ID, err)
		}
	}

	stats, err := s.updateProcessedCampaignStats(ctx, pc.ID)
	if err != nil {
		return fmt.Errorf("update stats for campaign id=%d: %w", c.ID, err)
	}
	if stats != nil && stats["aggregatedTotalSent"] != nil && stats["aggregatedTotalSent"].(int64) > 0 {
		if err := s.botClient.PushCampaignStatistics(ctx, c.ID, stats); err != nil {
			return fmt.Errorf("push statistics for campaign id=%d: %w", c.ID, err)
		}
	}

	s.logger.Printf("Splus scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, jazzAccessToken, c.ID); err != nil {
		return fmt.Errorf("move campaign id=%d to executed: %w", c.ID, err)
	}
	s.logger.Printf("Splus scheduler: campaign id=%d moved to executed", c.ID)

	go func(campaignID uint, uids, codes []string) {
		pushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := s.botClient.PushCampaignAudienceUIDs(pushCtx, campaignID, uids, codes); err != nil {
			s.logger.Printf("Splus scheduler: push audience UIDs failed for campaign id=%d: %v", campaignID, err)
			s.notifyAdmin(fmt.Sprintf("Splus Scheduler: push audience UIDs failed for campaign id=%d: %v", campaignID, err))
		}
	}(c.ID, uids, codes)

	return nil
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
	// if strings.TrimSpace(s.splusCfg.APIAccessKey) == "" {
	// 	return fmt.Errorf("Splus api-access-key is not configured")
	// }
	if _, err := extractSplusBotID(c); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformSPlus {
		return fmt.Errorf("campaign platform is not splus")
	}
	return nil
}

func (s *SplusCampaignScheduler) uploadCampaignMedia(ctx context.Context, jazzAccessToken, botID string, c dto.BotGetCampaignResponse) (*string, error) {
	if c.MediaUUID == nil || *c.MediaUUID == uuid.Nil {
		return nil, nil
	}

	path, err := s.botClient.DownloadCampaignMedia(ctx, jazzAccessToken, c.MediaUUID.String())
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

func buildSplusStatusMetadataDescription(existing *string, statusCode int, statusText string) string {
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

func buildSplusStatusResultMetadata(item SplusStatusResponse, normalizedStatus models.SplusSendStatus) json.RawMessage {
	metadata := map[string]any{
		"messageID":        strings.TrimSpace(item.MessageID),
		"statusCode":       item.Status,
		"statusText":       strings.TrimSpace(item.StatusText),
		"normalizedStatus": string(normalizedStatus),
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func (s *SplusCampaignScheduler) resolveScoreConstraint(ctx context.Context, c dto.BotGetCampaignResponse) (*models.NormalizedScoreConstraint, error) {
	if !gradesNeedScoreFilter(c.AudienceGrades) {
		return nil, nil
	}
	if s.statsRepo == nil {
		s.logger.Printf("resolveScoreConstraint: statsRepo not configured, skipping score filter for campaign id=%d", c.ID)
		return nil, nil
	}
	percentiles, err := s.statsRepo.FetchPercentiles(ctx, c.Level1, c.Level2s, c.Level3s)
	if err != nil {
		return nil, fmt.Errorf("fetch percentiles for campaign id=%d: %w", c.ID, err)
	}
	if percentiles == nil {
		s.logger.Printf("resolveScoreConstraint: no stats row found for campaign id=%d levels, skipping score filter", c.ID)
		return nil, nil
	}
	s.logger.Printf("resolveScoreConstraint: campaign id=%d grades=%v p33=%.4f p66=%.4f", c.ID, c.AudienceGrades, percentiles.P33, percentiles.P66)
	return gradesToScoreConstraint(c.AudienceGrades, percentiles.P33, percentiles.P66), nil
}

func (s *SplusCampaignScheduler) fetchSplusAudiencePhones(
	ctx context.Context,
	c dto.BotGetCampaignResponse,
	jazzAccessToken string,
	correlationID string,
) (*AudiencePhonesResult, error) {
	numAudiences := int64(0)
	if c.NumAudiences != nil {
		numAudiences = int64(*c.NumAudiences)
	}
	s.logger.Printf("fetchSplusAudiencePhones start: campaign_id=%d customer_id=%d num_audiences=%d tags_length=%d correlation_id=%s", c.ID, c.CustomerID, numAudiences, len(c.Tags), correlationID)

	if numAudiences <= 0 {
		return nil, fmt.Errorf("campaign num_audiences must be positive")
	}

	toExtract := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			s.logger.Printf("fetchSplusAudiencePhones tag parse failed: campaign_id=%d tag=%q err=%v", c.ID, tag, err)
			return nil, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhones tags lookup failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}

	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}
	s.logger.Printf("fetchSplusAudiencePhones tags resolved: campaign_id=%d requested=%d resolved=%d", c.ID, len(c.Tags), len(tagIDs))
	// NOTE: len(tagIDs) <= len(c.Tags) because some tags may not be found or are inactive

	scoreConstraint, err := s.resolveScoreConstraint(ctx, c)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhones resolve score constraint failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}

	const limit = 10000000

	tagsHash := hashTags(c.Tags)
	selection, err := s.audienceCache.Latest(ctx, c.CustomerID, tagsHash)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhones latest selection failed: campaign_id=%d customer_id=%d tags_hash=%s err=%v", c.ID, c.CustomerID, tagsHash, err)
		return nil, err
	}
	if selection != nil {
		s.logger.Printf("fetchSplusAudiencePhones selection hit: campaign_id=%d selection_id=%d prior_ids_length=%d", c.ID, selection.ID, len(selection.IDs))
	} else {
		s.logger.Printf("fetchSplusAudiencePhones selection miss: campaign_id=%d", c.ID)
	}

	selectAudiences := func(exclude map[int64]struct{}) ([]string, []int64, []string, error) {
		phones := make([]string, 0, numAudiences)
		ids := make([]int64, 0, numAudiences)
		uids := make([]string, 0, numAudiences)

		// Splus campaigns intentionally do not segment audiences by color (white/pink).
		// Color-based routing is SMS-specific, so Splus always queries by tag criteria only.
		filter := models.AudienceProfileFilter{Tags: &tagIDs, NormalizedScore: scoreConstraint}
		candidates, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
		if err != nil {
			s.logger.Printf("fetchSplusAudiencePhones fetch candidates failed: campaign_id=%d err=%v", c.ID, err)
			return nil, nil, nil, err
		}
		s.logger.Printf("fetchSplusAudiencePhones candidates: campaign_id=%d count=%d", c.ID, len(candidates))

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
			uids = append(uids, ap.UID)
		}

		for _, ap := range candidates {
			if int64(len(phones)) >= numAudiences {
				break
			}
			appendIfFresh(ap)
		}

		return phones, ids, uids, nil
	}

	// First attempt excluding prior picks for this customer/tags
	var exclude map[int64]struct{}
	if selection != nil && selection.IDs != nil {
		exclude = selection.IDs
	}
	phones, ids, uids, err := selectAudiences(exclude)
	if err != nil {
		return nil, err
	}
	s.logger.Printf("fetchSplusAudiencePhones selected (with exclusions): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)

	resetUsed := false
	if int64(len(phones)) < numAudiences {
		// Not enough fresh; retry from scratch without exclusions
		resetUsed = true
		phones, ids, uids, err = selectAudiences(nil)
		if err != nil {
			return nil, err
		}
		s.logger.Printf("fetchSplusAudiencePhones selected (reset): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)
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
		return nil, err
	}
	s.logger.Printf("fetchSplusAudiencePhones selection saved: campaign_id=%d selection_id=%d reset=%t selected=%d", c.ID, sel.ID, resetUsed, len(ids))

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchSplusAudiencePhones skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		s.logger.Printf("fetchSplusAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d ad_link=empty", c.ID, len(phones), len(phones), sel.ID)
		return &AudiencePhonesResult{
			Phones:      phones,
			IDs:         ids,
			UIDs:        uids,
			Codes:       make([]string, len(phones)),
			SelectionID: sel.ID,
		}, nil
	}

	// For new campaigns without a short link domain, skip AllocateShortLinks.
	// buildSplusMessageBody will replace {uid} directly in the ad link.
	if c.ShortLinkDomain == nil || strings.TrimSpace(*c.ShortLinkDomain) == "" {
		s.logger.Printf("fetchSplusAudiencePhones skipped short links generation: campaign_id=%d short_link_domain=empty", c.ID)
		s.logger.Printf("fetchSplusAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d short_link_domain=empty", c.ID, len(phones), len(phones), sel.ID)
		return &AudiencePhonesResult{
			Phones:      phones,
			IDs:         ids,
			UIDs:        uids,
			Codes:       make([]string, len(phones)),
			SelectionID: sel.ID,
		}, nil
	}

	// Generate sequential UIDs via bot API and persist short links centrally
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
		s.logger.Printf("fetchSplusAudiencePhones allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, err
	}
	if len(codes) != len(phones) {
		return nil, fmt.Errorf("allocate short links length mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}
	s.logger.Printf("fetchSplusAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d", c.ID, len(phones), len(codes), sel.ID)
	return &AudiencePhonesResult{
		Phones:      phones,
		IDs:         ids,
		UIDs:        uids,
		Codes:       codes,
		SelectionID: sel.ID,
	}, nil
}

// selectSplusTagAudiences fetches audience profiles matching tagIDs, skipping any IDs in exclude,
// up to numAudiences. Splus does not segment by color, so all matching profiles are queried.
func (s *SplusCampaignScheduler) selectSplusTagAudiences(
	ctx context.Context,
	campaignID uint,
	tagIDs pq.Int32Array,
	numAudiences int64,
	exclude map[int64]struct{},
	scoreConstraint *models.NormalizedScoreConstraint,
) (phones []string, ids []int64, uids []string, err error) {
	const limit = 10000000

	phones = make([]string, 0, numAudiences)
	ids = make([]int64, 0, numAudiences)
	uids = make([]string, 0, numAudiences)

	candidates, err := s.audRepo.ByFilter(ctx, models.AudienceProfileFilter{Tags: &tagIDs, NormalizedScore: scoreConstraint}, "id DESC", limit, 0)
	if err != nil {
		s.logger.Printf("selectSplusTagAudiences fetch candidates failed: campaign_id=%d err=%v", campaignID, err)
		return nil, nil, nil, err
	}
	s.logger.Printf("selectSplusTagAudiences candidates: campaign_id=%d count=%d", campaignID, len(candidates))

	for _, ap := range candidates {
		if int64(len(phones)) >= numAudiences {
			break
		}
		if ap == nil || ap.PhoneNumber == nil || *ap.PhoneNumber == "" {
			continue
		}
		if exclude != nil {
			if _, ok := exclude[int64(ap.ID)]; ok {
				continue
			}
		}
		phones = append(phones, *ap.PhoneNumber)
		ids = append(ids, int64(ap.ID))
		uids = append(uids, ap.UID)
	}

	return phones, ids, uids, nil
}

// fetchSplusAudiencePhonesByBundle selects audiences for an Splus campaign that belongs to a bundle.
// Uniqueness is enforced across all campaigns in the bundle via (customer_id, bundle_id):
// audiences already selected by earlier campaigns in the same bundle are excluded.
// No rolling-window reset — if fewer fresh audiences exist than requested, the available subset is returned as-is.
func (s *SplusCampaignScheduler) fetchSplusAudiencePhonesByBundle(
	ctx context.Context,
	c dto.BotGetCampaignResponse,
	jazzAccessToken string,
	correlationID string,
) (*AudiencePhonesResult, error) {
	bundleID := *c.BundleID
	numAudiences := int64(0)
	if c.NumAudiences != nil {
		numAudiences = int64(*c.NumAudiences)
	}
	s.logger.Printf("fetchSplusAudiencePhonesByBundle start: campaign_id=%d customer_id=%d bundle_id=%d num_audiences=%d correlation_id=%s",
		c.ID, c.CustomerID, bundleID, numAudiences, correlationID)

	if numAudiences <= 0 {
		return nil, fmt.Errorf("campaign num_audiences must be positive")
	}

	toExtract := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			s.logger.Printf("fetchSplusAudiencePhonesByBundle tag parse failed: campaign_id=%d tag=%q err=%v", c.ID, tag, err)
			return nil, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhonesByBundle tags lookup failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}
	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}
	s.logger.Printf("fetchSplusAudiencePhonesByBundle tags resolved: campaign_id=%d requested=%d resolved=%d", c.ID, len(c.Tags), len(tagIDs))

	scoreConstraint, err := s.resolveScoreConstraint(ctx, c)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhonesByBundle resolve score constraint failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}

	// Load the cumulative set of audience IDs already used by earlier campaigns in this bundle.
	var exclude map[int64]struct{}
	bundleSel, err := s.bundleAudienceCache.Latest(ctx, c.CustomerID, bundleID)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhonesByBundle latest bundle selection failed: campaign_id=%d bundle_id=%d err=%v", c.ID, bundleID, err)
		return nil, err
	}
	if bundleSel != nil {
		exclude = bundleSel.IDs
		s.logger.Printf("fetchSplusAudiencePhonesByBundle bundle selection hit: campaign_id=%d bundle_id=%d prior_ids=%d", c.ID, bundleID, len(bundleSel.IDs))
	} else {
		s.logger.Printf("fetchSplusAudiencePhonesByBundle bundle selection miss: campaign_id=%d bundle_id=%d", c.ID, bundleID)
	}

	phones, ids, uids, err := s.selectSplusTagAudiences(ctx, c.ID, tagIDs, numAudiences, exclude, scoreConstraint)
	if err != nil {
		return nil, err
	}
	s.logger.Printf("fetchSplusAudiencePhonesByBundle selected: campaign_id=%d bundle_id=%d selected=%d requested=%d",
		c.ID, bundleID, len(phones), numAudiences)

	// Persist the newly selected IDs merged with the existing bundle selection.
	sel, err := s.bundleAudienceCache.SaveWithMerge(ctx, c.CustomerID, bundleID, correlationID, ids)
	if err != nil {
		s.logger.Printf("fetchSplusAudiencePhonesByBundle selection save failed: campaign_id=%d bundle_id=%d err=%v", c.ID, bundleID, err)
		return nil, err
	}
	s.logger.Printf("fetchSplusAudiencePhonesByBundle selection saved: campaign_id=%d bundle_id=%d selection_id=%d selected=%d",
		c.ID, bundleID, sel.ID, len(ids))

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchSplusAudiencePhonesByBundle skipped short links: campaign_id=%d ad_link=empty", c.ID)
		return &AudiencePhonesResult{
			Phones:      phones,
			IDs:         ids,
			UIDs:        uids,
			Codes:       make([]string, len(phones)),
			SelectionID: sel.ID,
		}, nil
	}

	if c.ShortLinkDomain == nil || strings.TrimSpace(*c.ShortLinkDomain) == "" {
		s.logger.Printf("fetchSplusAudiencePhonesByBundle skipped short links: campaign_id=%d short_link_domain=empty", c.ID)
		return &AudiencePhonesResult{
			Phones:      phones,
			IDs:         ids,
			UIDs:        uids,
			Codes:       make([]string, len(phones)),
			SelectionID: sel.ID,
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
		s.logger.Printf("fetchSplusAudiencePhonesByBundle allocate short links failed: campaign_id=%d bundle_id=%d err=%v", c.ID, bundleID, err)
		return nil, err
	}
	if len(codes) != len(phones) {
		return nil, fmt.Errorf("allocate short links length mismatch for campaign id=%d bundle_id=%d: phones=%d codes=%d", c.ID, bundleID, len(phones), len(codes))
	}
	s.logger.Printf("fetchSplusAudiencePhonesByBundle success: campaign_id=%d bundle_id=%d selected=%d codes=%d selection_id=%d",
		c.ID, bundleID, len(phones), len(codes), sel.ID)
	return &AudiencePhonesResult{
		Phones:      phones,
		IDs:         ids,
		UIDs:        uids,
		Codes:       codes,
		SelectionID: sel.ID,
	}, nil
}

func (s *SplusCampaignScheduler) buildSplusMessageBody(c dto.BotGetCampaignResponse, code string, uid string) string {
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
			return strings.ReplaceAll(content, "{YOUR_LINK}", shortened)
		} else {
			injected := strings.ReplaceAll(*c.AdLink, "{uid}", uid)
			return strings.ReplaceAll(content, "{YOUR_LINK}", injected)
		}
	}
	return strings.ReplaceAll(content, "{YOUR_LINK}", "")
}

func (s *SplusCampaignScheduler) createUnmatchedSentSplusRows(ctx context.Context, processedCampaignID uint, unmatchedUIDs []string) error {
	if len(unmatchedUIDs) == 0 {
		return nil
	}

	const errCode = "AUDIENCE_UID_NOT_FOUND"
	rows := make([]*models.SentSplusMessage, 0, len(unmatchedUIDs))
	for _, uid := range unmatchedUIDs {
		desc := fmt.Sprintf("Audience uid not found or has no phone number: %s", uid)
		code := errCode
		rows = append(rows, &models.SentSplusMessage{
			ProcessedCampaignID: processedCampaignID,
			PhoneNumber:         "",
			PartsDelivered:      0,
			Status:              models.SplusSendStatusUnsuccessful,
			TrackingID:          uuid.NewString(),
			ServerID:            nil,
			ErrorCode:           &code,
			Description:         &desc,
		})
	}
	return s.sentRepo.SaveBatch(ctx, rows)
}

func (s *SplusCampaignScheduler) scheduleStatusCheckJobs(ctx context.Context, processedCampaignID uint, trackingIDs []string) error {
	if len(trackingIDs) == 0 || !s.splusClient.SupportsStatusTracking() || s.jobRepo == nil {
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
			Platform:            models.CampaignPlatformSPlus,
			TrackingIDs:         pq.StringArray(filteredTrackingIDs),
			RetryCount:          0,
			ScheduledAt:         now.Add(off),
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return s.jobRepo.SaveBatch(ctx, jobs)
}

func (s *SplusCampaignScheduler) startStatusJobWorker(parent context.Context) {
	ticker := time.NewTicker(statusJobWorkerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-parent.Done():
			return
		case <-ticker.C:
			if !s.splusClient.SupportsStatusTracking() || s.jobRepo == nil || s.resRepo == nil {
				continue
			}

			listCtx, listCancel := context.WithTimeout(parent, 30*time.Second)
			jobs, err := s.jobRepo.ListDue(listCtx, models.CampaignPlatformSPlus, utils.UTCNow(), numJobsPerTick)
			listCancel()
			if err != nil {
				s.logger.Printf("Splus scheduler: list status jobs failed: %v", err)
				continue
			}
			if len(jobs) == 0 {
				continue
			}

			for i, job := range jobs {
				if parent.Err() != nil {
					return
				}

				jobCtx, jobCancel := context.WithTimeout(parent, 2*time.Minute)
				err := s.handleStatusJob(jobCtx, job)
				jobCancel()

				if err != nil {
					s.logger.Printf("Splus scheduler: handle status job id=%d failed: %v", job.ID, err)
					if job.RetryCount >= statusJobMaxRetry {
						s.notifyAdmin(fmt.Sprintf("Splus scheduler: status job id=%d has failed %d times with error: %v", job.ID, job.RetryCount, err))
					}
				} else {
					s.logger.Printf("Splus scheduler: handle status job id=%d succeeded", job.ID)
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

func (s *SplusCampaignScheduler) handleStatusJob(ctx context.Context, job *models.CampaignStatusJob) error {
	rows, err := s.sentRepo.ListByTrackingIDs(ctx, job.ProcessedCampaignID, []string(job.TrackingIDs))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return s.markStatusJobExecuted(ctx, job, nil)
	}

	rowByServerID := make(map[string]*models.SentSplusMessage, len(rows))
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

	statusItems, fetchErr := s.splusClient.FetchStatus(ctx, serverIDs)
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

	txErr := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		now := utils.UTCNow()

		statusRows := make([]*models.SplusStatusResult, 0, len(statusItems))
		sendUpdates := make([]repository.SentSplusSendResultUpdate, 0, len(statusItems))
		for _, item := range statusItems {
			serverID := strings.TrimSpace(item.MessageID)
			if serverID == "" {
				s.logger.Printf("handleStatusJob: skipping status item with empty server_id for job_id=%d processed_campaign_id=%d", job.ID, job.ProcessedCampaignID)
				continue
			}
			row := rowByServerID[serverID]
			if row == nil {
				s.logger.Printf("handleStatusJob: no sent row found for server_id=%s job_id=%d processed_campaign_id=%d", serverID, job.ID, job.ProcessedCampaignID)
				continue
			}

			totalParts, deliveredParts, undeliveredParts, unknownParts, status := mapSplusProviderStatus(item.Status)
			statusText := strings.TrimSpace(item.StatusText)

			var errorCode *string
			var description *string
			if status == models.SplusSendStatusUnsuccessful {
				code := strconv.Itoa(item.Status)
				errorCode = &code
			}
			if metadataDesc := buildSplusStatusMetadataDescription(row.Description, item.Status, statusText); metadataDesc != "" {
				description = &metadataDesc
			}

			sendUpdates = append(sendUpdates, repository.SentSplusSendResultUpdate{
				TrackingID:     row.TrackingID,
				Status:         status,
				PartsDelivered: int(deliveredParts),
				ServerID:       row.ServerID,
				ErrorCode:      errorCode,
				Description:    description,
			})

			statusValue := strconv.Itoa(item.Status)
			providerStatusCode := int64(item.Status)
			var statusTextPtr *string
			if statusText != "" {
				statusTextPtr = &statusText
			}
			metadata := buildSplusStatusResultMetadata(item, status)

			statusRows = append(statusRows, &models.SplusStatusResult{
				JobID:                 job.ID,
				ProcessedCampaignID:   job.ProcessedCampaignID,
				TrackingID:            row.TrackingID,
				ServerID:              row.ServerID,
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
		if len(sendUpdates) == 0 {
			s.logger.Printf("handleStatusJob: all status items filtered out (no valid server_id or matched row) for job_id=%d processed_campaign_id=%d", job.ID, job.ProcessedCampaignID)
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
		if stats["aggregatedTotalSent"] != nil && stats["aggregatedTotalSent"].(int64) > 0 {
			if err := s.botClient.PushCampaignStatistics(ctx, pc.CampaignID, stats); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SplusCampaignScheduler) markStatusJobExecuted(ctx context.Context, job *models.CampaignStatusJob, errText *string) error {
	now := utils.UTCNow()
	job.ExecutedAt = &now
	job.UpdatedAt = now
	job.Error = errText
	return s.jobRepo.Update(ctx, job)
}

func mapSplusProviderStatus(statusCode int) (totalParts int64, deliveredParts int64, undeliveredParts int64, unknownParts int64, status models.SplusSendStatus) {
	switch statusCode {
	case 200, 202:
		return 1, 1, 0, 0, models.SplusSendStatusSuccessful
	case 400, 401, 404, 470, 700, 701, 702, 712, 715, 716, 723, 726, 727, 728, 729, 731, 732, 733, 734, 735, 739, 740:
		return 1, 0, 1, 0, models.SplusSendStatusUnsuccessful
	default:
		return 1, 0, 0, 1, models.SplusSendStatusPending
	}
}

func (s *SplusCampaignScheduler) updateProcessedCampaignStats(ctx context.Context, processedCampaignID uint) (map[string]any, error) {
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
		s.logger.Printf("updateProcessedCampaignStats: no status results yet for processed_campaign_id=%d, falling back to sent rows", processedCampaignID)
		return s.updateProcessedCampaignStatsFromSentRows(ctx, pc)
		// return nil, nil
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

func (s *SplusCampaignScheduler) updateProcessedCampaignStatsFromSentRows(ctx context.Context, pc *models.ProcessedCampaign) (map[string]any, error) {
	s.logger.Printf("updateProcessedCampaignStatsFromSentRows: computing stats from sent rows for processed_campaign_id=%d", pc.ID)
	type row struct {
		Total      int64
		Successful int64
	}
	var agg row
	if err := s.db.WithContext(ctx).Table("sent_splus_messages").
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
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id must not be empty")
		}
		return s, nil
	default:
		return "", fmt.Errorf("campaign platform_settings.metadata.splus_bot_id has unsupported type %T", raw)
	}
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
