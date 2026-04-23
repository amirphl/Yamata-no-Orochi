// Package scheduler
package scheduler

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/xuri/excelize/v2"
)

// CampaignScheduler periodically checks for campaigns ready to execute and triggers delivery
type CampaignScheduler struct {
	audRepo  repository.AudienceProfileRepository
	tagRepo  repository.TagRepository
	sentRepo repository.SentSMSRepository
	pcRepo   repository.ProcessedCampaignRepository
	jobRepo  repository.SMSStatusJobRepository
	resRepo  repository.SMSStatusResultRepository
	notifier NotificationSender
	logger   *log.Logger
	interval time.Duration

	db       *gorm.DB
	adminCfg config.AdminConfig

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

type SMSAudiencePhonesResult struct {
	Phones        []string
	IDs           []int64
	Codes         []string
	SelectionID   uint
	MatchedUIDs   []string
	UnmatchedUIDs []string
}

func NewCampaignScheduler(
	audRepo repository.AudienceProfileRepository,
	tagRepo repository.TagRepository,
	sentRepo repository.SentSMSRepository,
	pcRepo repository.ProcessedCampaignRepository,
	jobRepo repository.SMSStatusJobRepository,
	resRepo repository.SMSStatusResultRepository,
	notifier NotificationSender,
	db *gorm.DB,
	logger *log.Logger,
	interval time.Duration,
	payamSMSCfg config.PayamSMSConfig,
	botCfg config.BotConfig,
	adminCfg config.AdminConfig,
) *CampaignScheduler {
	if interval <= 0 {
		interval = time.Minute
	}

	if botCfg.APIDomain == "" {
		botCfg.APIDomain = defaultBotAPIDomain
	}

	s := &CampaignScheduler{
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
		botClient:     newHTTPBotClient(botCfg),
		smsClient:     newHTTPPayamSMSClient(payamSMSCfg),
		audienceCache: NewAudienceCache(repository.NewAudienceSelectionRepository(db)),
		schedulerName: "sms",
	}

	if err := s.initSchedulerLogger(); err != nil {
		s.logger = log.New(io.Discard, "sms_scheduler ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		s.logger.Printf("scheduler: failed to initialize file logger: %v", err)
	}

	return s
}

func initSchedulerLogger(name string) (*log.Logger, *os.File, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		clean = "scheduler"
	}

	candidates := []string{"data", "/data"}
	for _, dir := range candidates {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		logPath := filepath.Join(dir, fmt.Sprintf("%s.log", clean))
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		mw := io.MultiWriter(f)
		l := log.New(mw, fmt.Sprintf("%s ", clean), log.LstdFlags|log.Lmicroseconds|log.LUTC)
		return l, f, nil
	}

	return nil, nil, fmt.Errorf("could not create %s log file in any candidate directory", clean)
}

func (s *CampaignScheduler) initSchedulerLogger() error {
	l, f, err := initSchedulerLogger(s.schedulerName + "_scheduler")
	if err != nil {
		return err
	}
	s.logFile = f
	s.logger = l
	return nil
}

func (s *CampaignScheduler) Start(parent context.Context) func() {
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

	go s.startStatusJobWorker(ctx)

	return func() {
		cancel()
		if s.logFile != nil {
			_ = s.logFile.Close()
		}
	}
}

func (s *CampaignScheduler) runOnce(ctx context.Context) {
	// 2) Login to bot API and get access token
	token, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Scheduler bot login failed: %v", err))
		return
	}

	// 3) Get ready campaigns
	ready, err := s.botClient.ListReadyCampaigns(ctx, token, models.CampaignPlatformSMS)
	if err != nil {
		s.logger.Printf("scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Scheduler list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}
	s.logger.Printf("scheduler: listed %d ready campaigns", len(ready))

	// 4) Filter already processed
	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformSMS {
			// NOTE: Just skip.
			continue
		}
		if err := s.validateSMSCampaign(c); err != nil {
			s.logger.Printf("scheduler: validate campaign failed for campaign id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Scheduler validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("scheduler: check processed failed for campaign id=%d: %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Scheduler check processed failed for id=%d: %v", c.ID, err))
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		} else {
			s.logger.Printf("scheduler: campaign id=%d already processed, skipping", c.ID)
		}
	}
	if len(pending) == 0 {
		return
	}
	s.logger.Printf("scheduler: %d campaigns pending processing...", len(pending))

	// 5) Spawn goroutines per campaign
	for _, camp := range pending {
		c := camp
		go func() {
			if err := s.processSMSCampaign(ctx, token, c); err != nil {
				s.logger.Printf("scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("Scheduler process campaign failed for campaign id=%d: %v", c.ID, err))
			}
		}()
	}
	// Do not wait to keep scheduler loop non-blocking; optionally wait if desired
	// wg.Wait()
}

func (s *CampaignScheduler) processSMSCampaign(ctx context.Context, jazzToken string, c dto.BotGetCampaignResponse) error {
	// Sender from campaign line number
	if c.LineNumber == nil {
		return fmt.Errorf("sender is nil")
	}
	if c.NumAudiences == nil || *c.NumAudiences <= 0 {
		return fmt.Errorf("campaign has no audiences")
	}
	sender := *c.LineNumber

	if err := s.botClient.MoveCampaignToRunning(ctx, jazzToken, c.ID); err != nil {
		return fmt.Errorf("move to running: %w", err)
	}
	s.logger.Printf("scheduler: campaign id=%d moved to running", c.ID)

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
		s.logger.Printf("scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

		if hasTargetAudienceExcelFileUUID(c.TargetAudienceExcelFileUUID) {
			fileUIDs, err := s.fetchTargetAudienceUIDsFromExcel(txCtx, jazzToken, c.ID)
			if err != nil {
				return err
			}
			audienceResult, err := s.fetchSMSAudiencePhonesByUIDs(txCtx, c, jazzToken, fileUIDs)
			if err != nil {
				return err
			}
			phones = audienceResult.Phones
			ids = audienceResult.IDs
			codes = audienceResult.Codes
			unmatchedUID = audienceResult.UnmatchedUIDs
			s.logger.Printf("scheduler: fetched %d audience phones via excel for campaign id=%d (unmatched=%d)", len(phones), c.ID, len(unmatchedUID))
			pc.AudienceIDs = pq.Int64Array(ids)
			pc.AudienceCodes = codes
			pc.AudienceSelectionID = nil
		} else {
			// Fetch audiences (white then pink, DB-shuffled), and sort order is enforced inside repo
			var err error
			correlationID := uuid.NewString()
			audienceResult, err := s.fetchSMSAudiencePhones(txCtx, c, jazzToken, correlationID)
			if err != nil {
				return err
			}
			phones = audienceResult.Phones
			ids = audienceResult.IDs
			codes = audienceResult.Codes
			s.logger.Printf("scheduler: fetched %d audience phones for campaign id=%d", len(phones), c.ID)
			pc.AudienceIDs = pq.Int64Array(ids)
			pc.AudienceCodes = codes
			pc.AudienceSelectionID = utils.ToPtr(audienceResult.SelectionID)
		}

		pc.UpdatedAt = utils.UTCNow()
		if err := s.pcRepo.Update(txCtx, pc); err != nil {
			return err
		}
		s.logger.Printf("scheduler: updated processed campaign id=%d with audience ids", pc.ID)

		return nil
	}); err != nil {
		return err
	}
	s.logger.Printf("scheduler: persisted processed campaign id=%d num_audiences=%d", pc.ID, len(ids))

	if len(unmatchedUID) > 0 {
		if err := s.createUnmatchedSentSMSRows(ctx, pc.ID, unmatchedUID); err != nil {
			return err
		}
	}

	// NOTE: MUST BE LESS THAN 250
	batchSize := 100

	for start := 0; start < len(phones); start += batchSize {
		end := start + batchSize
		end = min(end, len(phones))
		batchPhones := phones[start:end]
		batchIDs := ids[start:end]
		batchCodes := codes[start:end]

		// Build per-recipient bodies by replacing short URL with unique 6-char code
		items := make([]PayamSMSItem, 0, len(batchPhones))
		// Build SentSMS rows from response
		rows := make([]*models.SentSMS, 0, len(batchPhones))
		trackingIDs, err := s.allocateTrackingIDs(ctx, len(batchPhones))
		if err != nil {
			return err
		}

		for i, p := range batchPhones {
			body := s.buildSMSBody(c, batchCodes[i])

			trackingID := trackingIDs[i]

			items = append(items, PayamSMSItem{
				Recipient:  p,
				Body:       body,
				trackingID: trackingID,
			})

			rows = append(rows, &models.SentSMS{
				ProcessedCampaignID: pc.ID,
				PhoneNumber:         p,
				PartsDelivered:      0,
				Status:              models.SMSSendStatusPending,
				TrackingID:          trackingID,
			})
		}

		// Store batch results and update last audience id + audience codes in one transaction
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

		// Sending via PayamSMS for this batch
		respItems, err := s.smsClient.SendBatchWithBodies(ctx, sender, items)
		if err != nil {
			s.logger.Printf("scheduler: payamsms send batch failed for campaign id=%d: %v", c.ID, err)
			// TODO: How to handle this error? Retry sending? Skip to next batch?
		} else {
			// Map provider response back to our sent_sms rows by trackingID using a batch update
			updates := make([]repository.SentSMSProviderUpdate, 0, len(respItems))
			for _, r := range respItems {
				if r.TrackingID == "" {
					continue
				}
				updates = append(updates, repository.SentSMSProviderUpdate{
					TrackingID:  r.TrackingID,
					ServerID:    r.ServerID,
					ErrorCode:   r.ErrorCode,
					Description: r.Desc,
				})
			}
			if len(updates) > 0 {
				// TODO: Start tx here if needed?
				if updateErr := s.sentRepo.UpdateProviderFieldsByTrackingIDs(ctx, updates); updateErr != nil {
					s.logger.Printf("scheduler: failed to batch update sent_sms provider fields for campaign id=%d: %v", c.ID, updateErr)
					// NOTE: Error silent here; not returning to avoid blocking further processing
				}
			}

			// Schedule status check jobs for this batch
			trackingIDs := make([]string, 0, len(rows))
			for _, r := range rows {
				if r.TrackingID != "" {
					trackingIDs = append(trackingIDs, r.TrackingID)
				}
			}
			if err := s.scheduleStatusCheckJobs(ctx, pc.ID, trackingIDs); err != nil {
				s.logger.Printf("scheduler: failed to schedule status jobs for campaign id=%d: %v", c.ID, err)
			}
		}
	}

	s.logger.Printf("scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, jazzToken, c.ID); err != nil {
		s.logger.Printf("scheduler: move executed failed for campaign id=%d: %v", c.ID, err)
		return err
	}
	s.logger.Printf("scheduler: campaign id=%d moved to executed", c.ID)

	return nil
}

func (s *CampaignScheduler) notifyAdmin(message string) {
	if s.notifier == nil {
		return
	}
	go func(msg string) {
		for _, mobile := range s.adminCfg.ActiveMobiles() {
			_ = s.notifier.SendSMS(context.Background(), mobile, msg, nil)
		}
	}(message)
}

func (s *CampaignScheduler) validateSMSCampaign(c dto.BotGetCampaignResponse) error {
	// check status = approved, created_at < nowutc, schedule_at < nowutc, updated_at < nowutc
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
	return nil
}

func (s *CampaignScheduler) fetchSMSAudiencePhones(ctx context.Context, c dto.BotGetCampaignResponse, token string, correlationID string) (*SMSAudiencePhonesResult, error) {
	s.logger.Printf("fetchSMSAudiencePhones start: campaign_id=%d customer_id=%d num_audiences=%d tags_length=%d correlation_id=%s", c.ID, c.CustomerID, c.NumAudiences, len(c.Tags), correlationID)

	numAudiences := int64(0)
	if c.NumAudiences != nil {
		numAudiences = int64(*c.NumAudiences)
	}
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
			if len(phones) >= int(numAudiences) {
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
	s.logger.Printf("fetchSMSAudiencePhones selected (with exclusions): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), c.NumAudiences)

	resetUsed := false
	if len(phones) < int(*c.NumAudiences) {
		// Not enough fresh; retry from scratch without exclusions
		resetUsed = true
		phones, ids, err = selectAudiences(nil)
		if err != nil {
			return nil, err
		}
		s.logger.Printf("fetchSMSAudiencePhones selected (reset): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), c.NumAudiences)
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
		return &SMSAudiencePhonesResult{
			Phones:      phones,
			IDs:         ids,
			Codes:       make([]string, len(phones)),
			SelectionID: sel.ID,
		}, nil
	}

	// Generate sequential UIDs via bot API and persist short links centrally
	codes, err := s.botClient.AllocateShortLinks(ctx, token, c.ID, c.AdLink, phones)
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhones allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, err
	}
	s.logger.Printf("fetchSMSAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d", c.ID, len(phones), len(codes), sel.ID)
	return &SMSAudiencePhonesResult{
		Phones:      phones,
		IDs:         ids,
		Codes:       codes,
		SelectionID: sel.ID,
	}, nil
}

func (s *CampaignScheduler) buildSMSBody(c dto.BotGetCampaignResponse, code string) string {
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

func hasCampaignAdLink(link *string) bool {
	return link != nil && strings.TrimSpace(*link) != ""
}

func hasTargetAudienceExcelFileUUID(fileUUID *string) bool {
	return fileUUID != nil && strings.TrimSpace(*fileUUID) != ""
}

func (s *CampaignScheduler) fetchTargetAudienceUIDsFromExcel(ctx context.Context, jazzToken string, campaignID uint) ([]string, error) {
	data, err := s.botClient.DownloadTargetAudienceExcelFile(ctx, jazzToken, campaignID)
	if err != nil {
		return nil, err
	}

	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    2 << 30, // 2GB
		UnzipXMLSizeLimit: 1 << 30, // 1GB
	})
	if err != nil {
		return nil, fmt.Errorf("cannot open target audience excel file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("target audience excel file has no sheets")
	}

	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("cannot iterate target audience excel rows: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	uids := make([]string, 0)
	rowIndex := 0
	for rows.Next() {
		row, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to read target audience excel row: %w", err)
		}
		if rowIndex == 0 {
			rowIndex++
			continue // header row
		}
		if len(row) == 0 {
			rowIndex++
			continue
		}
		uid := strings.TrimSpace(row[0])
		if uid != "" {
			uids = append(uids, uid)
		}
		rowIndex++
	}
	if err := rows.Error(); err != nil {
		return nil, fmt.Errorf("failed while reading target audience excel rows: %w", err)
	}
	return uids, nil
}

func (s *CampaignScheduler) fetchSMSAudiencePhonesByUIDs(ctx context.Context, c dto.BotGetCampaignResponse, token string, inputUIDs []string) (*SMSAudiencePhonesResult, error) {
	if len(inputUIDs) == 0 {
		return &SMSAudiencePhonesResult{}, nil
	}

	uniqueUIDs := make([]string, 0, len(inputUIDs))
	seen := make(map[string]struct{}, len(inputUIDs))
	for _, raw := range inputUIDs {
		uid := strings.TrimSpace(raw)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		uniqueUIDs = append(uniqueUIDs, uid)
	}
	if len(uniqueUIDs) == 0 {
		return &SMSAudiencePhonesResult{}, nil
	}

	profiles, err := s.audRepo.ByUIDs(ctx, uniqueUIDs)
	if err != nil {
		return nil, err
	}

	byUID := make(map[string]*models.AudienceProfile, len(profiles))
	for _, p := range profiles {
		if p == nil {
			continue
		}
		byUID[p.UID] = p
	}

	type matchedAudience struct {
		id    int64
		phone string
		uid   string
	}

	matched := make([]matchedAudience, 0, len(uniqueUIDs))
	unmatchedUIDs := make([]string, 0)
	for _, uid := range uniqueUIDs {
		profile, ok := byUID[uid]
		if !ok {
			unmatchedUIDs = append(unmatchedUIDs, uid)
			continue
		}
		if profile.PhoneNumber == nil || strings.TrimSpace(*profile.PhoneNumber) == "" {
			unmatchedUIDs = append(unmatchedUIDs, uid)
			continue
		}
		matched = append(matched, matchedAudience{
			id:    profile.ID,
			phone: strings.TrimSpace(*profile.PhoneNumber),
			uid:   uid,
		})
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].id < matched[j].id
	})

	phones := make([]string, 0, len(matched))
	ids := make([]int64, 0, len(matched))
	matchedUIDs := make([]string, 0, len(matched))
	for _, item := range matched {
		phones = append(phones, item.phone)
		ids = append(ids, item.id)
		matchedUIDs = append(matchedUIDs, item.uid)
	}

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchSMSAudiencePhonesByUIDs skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		return &SMSAudiencePhonesResult{
			Phones:        phones,
			IDs:           ids,
			Codes:         make([]string, len(phones)),
			MatchedUIDs:   matchedUIDs,
			UnmatchedUIDs: unmatchedUIDs,
		}, nil
	}

	// Generate sequential UIDs via bot API and persist short links centrally.
	codes, err := s.botClient.AllocateShortLinks(ctx, token, c.ID, c.AdLink, phones)
	if err != nil {
		s.logger.Printf("fetchSMSAudiencePhonesByUIDs allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, err
	}

	return &SMSAudiencePhonesResult{
		Phones:        phones,
		IDs:           ids,
		Codes:         codes,
		MatchedUIDs:   matchedUIDs,
		UnmatchedUIDs: unmatchedUIDs,
	}, nil
}

func (s *CampaignScheduler) createUnmatchedSentSMSRows(ctx context.Context, processedCampaignID uint, unmatchedUIDs []string) error {
	pc, err := s.pcRepo.ByID(ctx, processedCampaignID)
	if err != nil {
		return err
	}
	if pc == nil {
		return fmt.Errorf("processed campaign not found for processed campaign id=%d", processedCampaignID)
	}

	trackingIDs, err := s.allocateTrackingIDs(ctx, len(unmatchedUIDs))
	if err != nil {
		return err
	}

	const (
		errCode = "AUDIENCE_UID_NOT_FOUND"
	)
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
	err = s.sentRepo.SaveBatch(ctx, fakeSentSMSs)
	if err != nil {
		return err
	}

	now := utils.UTCNow()
	fakeJob := &models.SMSStatusJob{
		ProcessedCampaignID: processedCampaignID,
		CorrelationID:       uuid.NewString(),
		TrackingIDs:         pq.StringArray(trackingIDs),
		RetryCount:          0,
		ScheduledAt:         now,
		ExecutedAt:          new(now.Add(time.Second)),
		CreatedAt:           now,
		UpdatedAt:           now.Add(time.Second),
	}
	if err := s.jobRepo.Save(ctx, fakeJob); err != nil {
		return err
	}

	zero := new(int64(0))
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
	if err := s.resRepo.SaveBatch(ctx, fakeSMSStatusResults); err != nil {
		return err
	}

	stats, err := s.updateProcessedCampaignStats(ctx, fakeJob.ProcessedCampaignID)
	if err != nil {
		return err
	}
	if stats != nil {
		if err := s.botClient.PushCampaignStatistics(ctx, pc.CampaignID, stats); err != nil {
			return err
		}
	}

	return nil
}

const (
	trackingCounterName   = "sms_tracking_id"
	trackingCounterHexLen = 16
	trackingCounterBits   = 16 * 4
)

func (s *CampaignScheduler) allocateTrackingIDs(ctx context.Context, count int) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}

	var ids []string
	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		db := s.db.WithContext(txCtx)
		if tx, ok := txCtx.Value(repository.TxContextKey).(*gorm.DB); ok && tx != nil {
			db = tx.WithContext(txCtx)
		}

		var counter models.SequenceCounter
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&counter, "name = ?", trackingCounterName).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			now := utils.UTCNow()
			counter = models.SequenceCounter{
				Name:      trackingCounterName,
				LastValue: strings.Repeat("0", trackingCounterHexLen),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := db.Create(&counter).Error; err != nil {
				return err
			}
		}

		last := strings.TrimSpace(counter.LastValue)
		if last == "" {
			last = strings.Repeat("0", trackingCounterHexLen)
		}
		if len(last) > trackingCounterHexLen {
			return fmt.Errorf("tracking counter exceeds %d hex chars", trackingCounterHexLen)
		}
		last = strings.Repeat("0", trackingCounterHexLen-len(last)) + strings.ToLower(last)
		base := new(big.Int)
		if _, ok := base.SetString(last, 16); !ok {
			return fmt.Errorf("invalid tracking counter value")
		}

		ids = make([]string, count)
		for i := 0; i < count; i++ {
			base.Add(base, big.NewInt(1))
			if base.BitLen() > trackingCounterBits {
				return fmt.Errorf("tracking counter overflow")
			}
			ids[i] = fmt.Sprintf("%0*x", trackingCounterHexLen, base)
		}

		counter.LastValue = ids[len(ids)-1]
		counter.UpdatedAt = utils.UTCNow()
		return db.Model(&models.SequenceCounter{}).
			Where("name = ?", counter.Name).
			Updates(map[string]any{
				"last_value": counter.LastValue,
				"updated_at": counter.UpdatedAt,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}

func hashTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	cp := make([]string, len(tags))
	copy(cp, tags)
	sort.Strings(cp)
	h := sha1.Sum([]byte(strings.Join(cp, ",")))
	return hex.EncodeToString(h[:])
}

// scheduleStatusCheckJobs creates three status check jobs for the provided tracking IDs
func (s *CampaignScheduler) scheduleStatusCheckJobs(ctx context.Context, processedCampaignID uint, trackingIDs []string) error {
	if len(trackingIDs) == 0 {
		return nil
	}
	corrID := uuid.NewString()
	filteredTrackingIDs := make([]string, 0, len(trackingIDs))
	for _, id := range trackingIDs {
		if strings.TrimSpace(id) != "" {
			filteredTrackingIDs = append(filteredTrackingIDs, strings.TrimSpace(id))
		}
	}
	if len(filteredTrackingIDs) == 0 {
		return nil
	}

	now := utils.UTCNow()
	offsets := []time.Duration{5 * time.Minute, 15 * time.Minute, 1 * time.Hour, 24 * time.Hour, 48 * time.Hour}
	jobs := make([]*models.SMSStatusJob, 0, len(offsets))
	for _, off := range offsets {
		jobs = append(jobs, &models.SMSStatusJob{
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

// startStatusJobWorker polls and executes due SMS status jobs every 10 minutes
func (s *CampaignScheduler) startStatusJobWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// initial run
	s.processStatusJobs(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processStatusJobs(ctx)
		}
	}
}

func (s *CampaignScheduler) processStatusJobs(ctx context.Context) {
	now := utils.UTCNow()
	jobs, err := s.jobRepo.ListDue(ctx, now, 100)
	if err != nil {
		s.logger.Printf("scheduler: list status jobs failed: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	token, err := s.smsClient.GetToken(ctx)
	if err != nil {
		s.logger.Printf("scheduler: payamsms token for status jobs failed: %v", err)
		return
	}

	// TODO: What about parallel execution with limited concurrency?
	for _, job := range jobs {
		jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // #TODO: adjust timeout as needed
		if err := s.handleStatusJob(jobCtx, job, token); err != nil {
			s.logger.Printf("scheduler: handle status job id=%d failed: %v", job.ID, err)
		} else {
			s.logger.Printf("scheduler: handle status job id=%d succeeded", job.ID)
		}
		cancel()
	}
}

func (s *CampaignScheduler) handleStatusJob(ctx context.Context, job *models.SMSStatusJob, token string) error {
	results, err := s.smsClient.FetchStatus(ctx, token, []string(job.TrackingIDs))
	var stats map[string]any

	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		now := utils.UTCNow()
		if err != nil {
			job.RetryCount++
			msg := err.Error()
			job.Error = &msg
			job.ExecutedAt = &now
			job.UpdatedAt = now
			return s.jobRepo.Update(txCtx, job)
		}

		rows := make([]*models.SMSStatusResult, 0, len(results))
		for idx, r := range results {
			tp := r.TotalParts
			td := r.TotalDeliveredParts
			tu := r.TotalUndeliveredParts
			tu2 := r.TotalUnknownParts
			status := r.Status
			trackingID := ""
			if idx < len(job.TrackingIDs) {
				trackingID = job.TrackingIDs[idx]
			}
			rows = append(rows, &models.SMSStatusResult{
				JobID:                 job.ID,
				ProcessedCampaignID:   job.ProcessedCampaignID,
				TrackingID:            trackingID,
				ServerID:              r.ServerID,
				TotalParts:            &tp,
				TotalDeliveredParts:   &td,
				TotalUndeliveredParts: &tu,
				TotalUnknownParts:     &tu2,
				Status:                &status,
			})
		}
		if err := s.resRepo.SaveBatch(txCtx, rows); err != nil {
			return err
		}
		if stats, err = s.updateProcessedCampaignStats(txCtx, job.ProcessedCampaignID); err != nil {
			return err
		}
		job.ExecutedAt = &now
		job.Error = nil
		job.UpdatedAt = now
		return s.jobRepo.Update(txCtx, job)
	})
	if err != nil {
		return err
	}

	// Push statistics to bot API after transaction commits
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

func (s *CampaignScheduler) updateProcessedCampaignStats(txCtx context.Context, processedCampaignID uint) (map[string]any, error) {
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
