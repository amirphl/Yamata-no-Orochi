// Package scheduler
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"gorm.io/gorm"

	"io"
	"os"
	"path/filepath"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/lib/pq"
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

	db          *gorm.DB
	payamSMSCfg config.PayamSMSConfig
	botCfg      config.BotConfig

	logFile *os.File
}

// NotificationSender is a minimal interface extracted from NotificationService for SMS
// This keeps the scheduler independent and easy to test
type NotificationSender interface {
	SendSMS(ctx context.Context, to string, message string, customerID *int64) error
	SendSMSBulk(ctx context.Context, mobiles []string, message string, customerID *int64) error
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
) *CampaignScheduler {
	if interval <= 0 {
		interval = time.Minute
	}

	s := &CampaignScheduler{
		audRepo:     audRepo,
		tagRepo:     tagRepo,
		sentRepo:    sentRepo,
		pcRepo:      pcRepo,
		jobRepo:     jobRepo,
		resRepo:     resRepo,
		notifier:    notifier,
		db:          db,
		interval:    interval,
		payamSMSCfg: payamSMSCfg,
		botCfg:      botCfg,
	}
	if s.botCfg.APIDomain == "" {
		s.botCfg.APIDomain = "https://jazebeh.ir"
	}

	// Initialize scheduler-specific logger (to stdout and persistent file)
	if err := s.initSchedulerLogger(); err != nil {
		// Fallback to default stdout logger if file logger init fails
		s.logger = log.Default()
		s.logger.Printf("scheduler: failed to initialize file logger: %v", err)
	}

	return s
}

// initSchedulerLogger configures a logger that writes to both stdout and a persistent file under data/ (or /data)
func (s *CampaignScheduler) initSchedulerLogger() error {
	// Prefer relative data/ then fallback to /data for containerized environments
	candidates := []string{
		filepath.Join("data"),
		"/data",
	}
	var logPath string
	for _, dir := range candidates {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		logPath = filepath.Join(dir, "scheduler.log")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		// Success
		s.logFile = f
		mw := io.MultiWriter(os.Stdout, f)
		// log.Logger is goroutine-safe; include timestamps with microseconds and UTC
		s.logger = log.New(mw, "scheduler ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		return nil
	}
	return fmt.Errorf("could not create scheduler log file in any candidate directory")
}

// Start launches the scheduler loop in a background goroutine and returns a stop function
func (s *CampaignScheduler) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)

	go func() {
		// 1)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runOnce(ctx)
			}
		}
	}()

	// Start status job worker (runs every 10 minutes)
	go s.startStatusJobWorker(ctx)

	return cancel
}

func (s *CampaignScheduler) runOnce(ctx context.Context) {
	// 2) Login to bot API and get access token
	token, err := s.loginBot(ctx)
	if err != nil {
		s.logger.Printf("scheduler: bot login failed: %v", err)
		return
	}
	// Success (concise)
	s.logger.Printf("scheduler: bot login succeeded")

	// 3) Get ready campaigns
	ready, err := s.listReadyCampaigns(ctx, token)
	if err != nil {
		s.logger.Printf("scheduler: list ready campaigns failed: %v", err)
		return
	}
	if len(ready) == 0 {
		return
	}
	s.logger.Printf("scheduler: listed %d ready campaigns", len(ready))

	// 4) Filter already processed
	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if err := s.validateCampaign(c); err != nil {
			s.logger.Printf("scheduler: validate campaign failed for campaign id=%d: %v", c.ID, err)
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("scheduler: check processed failed for campaign id=%d: %v", c.ID, err)
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		}
	}
	if len(pending) == 0 {
		return
	}
	s.logger.Printf("scheduler: %d campaigns pending processing", len(pending))

	// 5) Spawn goroutines per campaign
	for _, camp := range pending {
		c := camp
		go func() {
			if err := s.processCampaign(ctx, token, c); err != nil {
				s.logger.Printf("scheduler: process campaign id=%d failed: %v", c.ID, err)
			}
		}()
	}
	// Do not wait to keep scheduler loop non-blocking; optionally wait if desired
	// wg.Wait()
}

func (s *CampaignScheduler) processCampaign(ctx context.Context, token string, c dto.BotGetCampaignResponse) error {
	// 6) Mark running
	if err := s.moveCampaignToRunning(ctx, token, c.ID); err != nil {
		return fmt.Errorf("move to running: %w", err)
	}
	s.logger.Printf("scheduler: campaign id=%d moved to running", c.ID)

	// First transaction: create processed_campaign and persist full audience IDs
	var (
		phones []string
		ids    []int64
		codes  []string
		pc     *models.ProcessedCampaign
	)
	// 7) Save the campaign in the processed campaign table AND 8), 9), 10) Save the list of audiences in the processed campaign table
	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		pc = &models.ProcessedCampaign{
			CampaignID:     c.ID,
			CampaignJSON:   func() json.RawMessage { b, _ := json.Marshal(c); return b }(),
			AudienceIDs:    pq.Int64Array{},
			AudienceCodes:  []string{},
			LastAudienceID: nil,
		}
		if err := s.pcRepo.Save(txCtx, pc); err != nil {
			return err
		}

		// Fetch audiences (white then pink, DB-shuffled), and sort order is enforced inside repo
		var err error
		phones, ids, codes, err = s.fetchAudiencePhones(txCtx, c, token)
		if err != nil {
			return err
		}

		pc.AudienceIDs = pq.Int64Array(ids)
		pc.AudienceCodes = codes
		if err := s.pcRepo.Update(txCtx, pc); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	s.logger.Printf("scheduler: persisted processed campaign id=%d audiences=%d", pc.ID, len(ids))

	// 12/13) Send batches; after each batch, save sent_sms and update LastAudienceID in SAME transaction
	batchSize := 200 // MUST BE LESS THAN 250
	// Sender from campaign line number
	if c.LineNumber == nil {
		return fmt.Errorf("sender is nil")
	}
	sender := *c.LineNumber

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
		for i, p := range batchPhones {
			// 11) Build message
			body := s.buildSMSBody(c, batchCodes[i])

			trackingID := uuid.New().String()

			items = append(items, PayamSMSItem{
				Recipient:  p,
				Body:       body,
				CustomerID: trackingID,
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
			last := batchIDs[len(batchIDs)-1]
			pc.LastAudienceID = &last
			if err := s.pcRepo.Update(txCtx, pc); err != nil {
				return err
			}

			// TODO: Write jobs for updating delivery status

			return nil
		}); err != nil {
			return err
		}

		// Sending via PayamSMS for this batch
		respItems, err := s.sendPayamSMSBatchWithBodies(ctx, sender, items)
		if err != nil {
			// TODO: handle error
		} else {
			// Map provider response back to our sent_sms rows by customerId (trackingID) using a batch update
			updates := make([]repository.SentSMSProviderUpdate, 0, len(respItems))
			for _, r := range respItems {
				if r.CustomerID == "" {
					continue
				}
				updates = append(updates, repository.SentSMSProviderUpdate{
					TrackingID:  r.CustomerID,
					ServerID:    r.ServerID,
					ErrorCode:   r.ErrorCode,
					Description: r.Desc,
				})
			}
			if len(updates) > 0 {
				if updateErr := s.sentRepo.UpdateProviderFieldsByTrackingIDs(ctx, updates); updateErr != nil {
					s.logger.Printf("scheduler: failed to batch update sent_sms provider fields: %v", updateErr)
				} else {
					// Success (concise)
					s.logger.Printf("scheduler: sent sms batch items=%d for campaign id=%d", len(items), c.ID)
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

	// 15) Mark executed
	if err := s.moveCampaignToExecuted(ctx, token, c.ID); err != nil {
		s.logger.Printf("scheduler: move executed failed for id=%d: %v", c.ID, err)
	}
	s.logger.Printf("scheduler: campaign id=%d moved to executed", c.ID)
	return nil
}

func (s *CampaignScheduler) loginBot(ctx context.Context) (string, error) {
	if s.botCfg.Username == "" || s.botCfg.Password == "" {
		return "", fmt.Errorf("bot credentials not configured")
	}
	url := s.botCfg.APIDomain + "/api/v1/bot/auth/login"
	reqBody := dto.BotLoginRequest{
		Username: s.botCfg.Username,
		Password: s.botCfg.Password,
	}
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bot login http status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Now try to decode into the expected struct
	var apiResp dto.APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to decode JSON into APIResponse: %w", err)
	}

	// Check if the API call was successful
	if !apiResp.Success {
		return "", fmt.Errorf("bot login failed: %v", apiResp.Message)
	}

	// Convert the Data field to JSON again to parse into BotLoginResponse
	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal APIResponse data: %w", err)
	}

	var botLoginResp dto.BotLoginResponse
	if err := json.Unmarshal(dataBytes, &botLoginResp); err != nil {
		return "", fmt.Errorf("failed to decode JSON into BotLoginResponse: %w", err)
	}

	if botLoginResp.Session.AccessToken == "" {
		return "", fmt.Errorf("empty bot access token")
	}

	return botLoginResp.Session.AccessToken, nil
}

func (s *CampaignScheduler) listReadyCampaigns(ctx context.Context, token string) ([]dto.BotGetCampaignResponse, error) {
	url := s.botCfg.APIDomain + "/api/v1/bot/campaigns/ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ready campaigns http status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp dto.APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode JSON into APIResponse: %w", err)
	}

	// Check if the API call was successful
	if !apiResp.Success {
		return nil, fmt.Errorf("list ready campaigns failed: %v", apiResp.Message)
	}

	// Convert the Data field to JSON again to parse into BotListCampaignsResponse
	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal APIResponse data: %w", err)
	}

	var listCampaignsResp dto.BotListCampaignsResponse
	if err := json.Unmarshal(dataBytes, &listCampaignsResp); err != nil {
		return nil, fmt.Errorf("failed to decode JSON into BotListCampaignsResponse: %w", err)
	}

	return listCampaignsResp.Items, nil
}

func (s *CampaignScheduler) moveCampaignToRunning(ctx context.Context, token string, id uint) error {
	url := s.botCfg.APIDomain + "/api/v1/bot/campaigns/" + strconv.FormatUint(uint64(id), 10) + "/running"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("move to running http status: %d", resp.StatusCode)
	}
	return nil
}

func (s *CampaignScheduler) moveCampaignToExecuted(ctx context.Context, token string, id uint) error {
	url := s.botCfg.APIDomain + "/api/v1/bot/campaigns/" + strconv.FormatUint(uint64(id), 10) + "/executed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("move to executed http status: %d", resp.StatusCode)
	}
	return nil
}

func (s *CampaignScheduler) createShortLinksViaAPI(ctx context.Context, token string, reqBody *dto.BotCreateShortLinksRequest) error {
	url := s.botCfg.APIDomain + "/api/v1/bot/short-links"
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("create short-links http status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	var apiResp dto.APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to decode JSON into APIResponse: %w", err)
	}
	if !apiResp.Success {
		return fmt.Errorf("create short-links failed: %v", apiResp.Message)
	}
	return nil
}

func (s *CampaignScheduler) validateCampaign(c dto.BotGetCampaignResponse) error {
	// check status = approved, created_at < nowutc, schedule_at < nowutc, updated_at < nowutc
	if c.Status != string(models.CampaignStatusApproved) {
		return fmt.Errorf("campaign status is not approved")
	}
	if c.ScheduleAt.After(utils.UTCNow()) {
		return fmt.Errorf("campaign schedule_at is after now")
	}
	if c.CreatedAt.After(utils.UTCNow()) {
		return fmt.Errorf("campaign created_at is after now")
	}
	if c.UpdatedAt.After(utils.UTCNow()) {
		return fmt.Errorf("campaign updated_at is after now")
	}
	return nil
}

func (s *CampaignScheduler) fetchAudiencePhones(ctx context.Context, c dto.BotGetCampaignResponse, token string) ([]string, []int64, []string, error) {
	var phones []string
	var ids []int64

	adLink := ""
	if c.AdLink != nil {
		adLink = *c.AdLink
	}

	toExtract := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			return nil, nil, nil, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		return nil, nil, nil, err
	}
	// create a pq int32 array from tags
	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}

	// NOTE: len(tagIDs) <= len(c.Tags) because some tags may not be found or are inactive

	filter := models.AudienceProfileFilter{
		Tags:  &tagIDs,
		Color: utils.ToPtr("white"),
	}

	const MAX = 10000000

	whites, err := s.audRepo.ByFilter(ctx, filter, "id DESC", MAX, 0)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, ap := range whites {
		if ap == nil || ap.PhoneNumber == nil || *ap.PhoneNumber == "" {
			continue
		}
		phones = append(phones, *ap.PhoneNumber)
		ids = append(ids, int64(ap.ID))
		if len(phones) >= int(c.NumAudiences) {
			break
		}
	}

	if len(phones) >= int(c.NumAudiences) {
		// Generate sequential UIDs via bot API and persist short links centrally
		codes, err := s.allocateShortLinksViaAPI(ctx, token, c.ID, adLink, phones)
		if err != nil {
			return nil, nil, nil, err
		}
		return phones, ids, codes, nil
	}

	filter.Color = utils.ToPtr("pink")
	pinks, err := s.audRepo.ByFilter(ctx, filter, "id DESC", MAX, 0)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, ap := range pinks {
		if ap == nil || ap.PhoneNumber == nil || *ap.PhoneNumber == "" {
			continue
		}
		phones = append(phones, *ap.PhoneNumber)
		ids = append(ids, int64(ap.ID))
		if len(phones) >= int(c.NumAudiences) {
			break
		}
	}

	// Generate sequential UIDs via bot API and persist short links centrally
	codes, err := s.allocateShortLinksViaAPI(ctx, token, c.ID, adLink, phones)
	if err != nil {
		return nil, nil, nil, err
	}
	return phones, ids, codes, nil
}

func (s *CampaignScheduler) buildSMSBody(c dto.BotGetCampaignResponse, code string) string {
	content := ""
	if c.Content != nil {
		content = *c.Content
	}
	if c.AdLink != nil && *c.AdLink != "" {
		shortened := "https://jo1n.ir/s/" + code
		return strings.ReplaceAll(content, "🔗", shortened) + "\n" + "لغو۱۱"

	}
	return strings.ReplaceAll(content, "🔗", "") + "\n" + "لغو۱۱"
}

// sendPayamSMSBatchWithBodies sends a batch using custom per-recipient bodies
type PayamSMSItem struct {
	Recipient  string
	Body       string
	CustomerID string
}

func (s *CampaignScheduler) sendPayamSMSBatchWithBodies(ctx context.Context, sender string, items []PayamSMSItem) ([]struct {
	CustomerID string  `json:"customerId"`
	Mobile     string  `json:"mobile"`
	ServerID   *string `json:"serverId"`
	ErrorCode  *string `json:"errorCode"`
	Desc       *string `json:"description"`
}, error) {
	if len(items) == 0 {
		return nil, nil
	}
	payload := struct {
		Sender   string `json:"sender"`
		SMSItems []any  `json:"smsItems"`
	}{
		Sender:   sender,
		SMSItems: make([]any, 0, len(items)),
	}
	sendDate, err := utils.TehranNow()
	if err != nil {
		return nil, err
	}
	sendDate = sendDate.Add(time.Minute)

	for _, it := range items {
		payload.SMSItems = append(payload.SMSItems, map[string]any{
			"recipient":  it.Recipient,
			"body":       it.Body,
			"customerId": it.CustomerID,
			"sendDate":   sendDate.Format("2006-01-02 15:04:05"),
		})
	}
	b, _ := json.Marshal(payload)
	token, err := s.getPayamSMSToken(ctx)
	if err != nil {
		return nil, err
	}
	url := "https://www.payamsms.com/panel/webservice/sendMultipleWithSrc"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("payamsms sendMultiple http status: %d", resp.StatusCode)
	}
	var out []struct {
		CustomerID string  `json:"customerId"`
		Mobile     string  `json:"mobile"`
		ServerID   *string `json:"serverId"`
		ErrorCode  *string `json:"errorCode"`
		Desc       *string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// getPayamSMSToken fetches OAuth token using env-configured credentials
func (s *CampaignScheduler) getPayamSMSToken(ctx context.Context) (string, error) {
	tokenURL := s.payamSMSCfg.TokenURL
	if tokenURL == "" {
		tokenURL = "https://www.payamsms.com/auth/oauth/token"
	}
	systemName := s.payamSMSCfg.SystemName
	username := s.payamSMSCfg.Username
	password := s.payamSMSCfg.Password
	scope := s.payamSMSCfg.Scope
	grantType := s.payamSMSCfg.GrantType
	rootToken := s.payamSMSCfg.RootAccessToken
	if scope == "" {
		scope = "webservice"
	}
	if grantType == "" {
		grantType = "password"
	}

	url := fmt.Sprintf("%s?systemName=%s&username=%s&password=%s&scope=%s&grant_type=%s", tokenURL, systemName, username, password, scope, grantType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	if rootToken != "" {
		req.Header.Set("Authorization", "Basic "+rootToken)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("payamsms token http status: %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("empty access_token")
	}
	return out.AccessToken, nil
}

func (s *CampaignScheduler) allocateShortLinksViaAPI(ctx context.Context, token string, campaignID uint, adLink string, phones []string) ([]string, error) {
	payload := dto.BotGenerateShortLinksRequest{
		CampaignID:      campaignID,
		AdLink:          adLink,
		Phones:          phones,
		ShortLinkDomain: "https://jo1n.ir/",
	}
	b, _ := json.Marshal(payload)
	url := s.botCfg.APIDomain + "/api/v1/bot/short-links/allocate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("allocate short-links http status: %d", resp.StatusCode)
	}
	var apiResp dto.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if !apiResp.Success {
		return nil, fmt.Errorf("allocate short-links failed: %v", apiResp.Message)
	}
	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, err
	}
	var out dto.BotGenerateShortLinksResponse
	if err := json.Unmarshal(dataBytes, &out); err != nil {
		return nil, err
	}
	return out.Codes, nil
}

// scheduleStatusCheckJobs creates three status check jobs for the provided tracking IDs
func (s *CampaignScheduler) scheduleStatusCheckJobs(ctx context.Context, processedCampaignID uint, customerIDs []string) error {
	if len(customerIDs) == 0 {
		return nil
	}
	corrID := uuid.NewString()
	filtered := make([]string, 0, len(customerIDs))
	for _, id := range customerIDs {
		if strings.TrimSpace(id) != "" {
			filtered = append(filtered, strings.TrimSpace(id))
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	now := utils.UTCNow()
	offsets := []time.Duration{5 * time.Minute, 1 * time.Hour, 3 * time.Hour}
	jobs := make([]*models.SMSStatusJob, 0, len(offsets))
	for _, off := range offsets {
		jobs = append(jobs, &models.SMSStatusJob{
			ProcessedCampaignID: processedCampaignID,
			CorrelationID:       corrID,
			CustomerIDs:         pq.StringArray(filtered),
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
	ticker := time.NewTicker(10 * time.Minute)
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
	if s.jobRepo == nil || s.resRepo == nil {
		return
	}

	now := utils.UTCNow()
	jobs, err := s.jobRepo.ListDue(ctx, now, 100)
	if err != nil {
		s.logger.Printf("scheduler: list status jobs failed: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	token, err := s.getPayamSMSToken(ctx)
	if err != nil {
		s.logger.Printf("scheduler: payamsms token for status jobs failed: %v", err)
		return
	}

	for _, job := range jobs {
		jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := s.handleStatusJob(jobCtx, job, token); err != nil {
			s.logger.Printf("scheduler: handle status job id=%d failed: %v", job.ID, err)
		}
	}
}

func (s *CampaignScheduler) handleStatusJob(ctx context.Context, job *models.SMSStatusJob, token string) error {
	results, err := s.fetchPayamSMSStatus(ctx, token, []string(job.CustomerIDs))
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
		for _, r := range results {
			tp := r.TotalParts
			td := r.TotalDeliveredParts
			tu := r.TotalUndeliveredParts
			tu2 := r.TotalUnknownParts
			status := r.Status
			rows = append(rows, &models.SMSStatusResult{
				JobID:                 job.ID,
				ProcessedCampaignID:   job.ProcessedCampaignID,
				CustomerID:            r.CustomerID,
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
		if err := s.pushCampaignStatistics(ctx, job.ProcessedCampaignID, stats); err != nil {
			s.logger.Printf("scheduler: failed to push campaign statistics processed_campaign_id=%d: %v", job.ProcessedCampaignID, err)
		}
	}
	return nil
}

func (s *CampaignScheduler) updateProcessedCampaignStats(ctx context.Context, processedCampaignID uint) (map[string]any, error) {
	pc, err := s.pcRepo.ByID(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, fmt.Errorf("processed campaign not found for campaign_id=%d", processedCampaignID)
	}

	agg, err := s.resRepo.AggregateByCampaign(ctx, processedCampaignID)
	if err != nil {
		return nil, err
	}

	stats := map[string]any{
		"totalSent":                       agg.TotalSent,
		"aggregatedTotalParts":            agg.AggregatedTotalParts,
		"aggregatedTotalDeliveredParts":   agg.AggregatedDeliveredParts,
		"aggregatedTotalUnDeliveredParts": agg.AggregatedUndelivered,
		"aggregatedTotalUnKnownParts":     agg.AggregatedUnknown,
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

type PayamStatusResponse struct {
	CustomerID            string  `json:"customerId"`
	ServerID              *string `json:"serverId"`
	TotalParts            int64   `json:"totalParts"`
	TotalDeliveredParts   int64   `json:"totalDeliveredParts"`
	TotalUndeliveredParts int64   `json:"totalUnDeliveredParts"`
	TotalUnknownParts     int64   `json:"totalUnKnownParts"`
	Status                string  `json:"status"`
}

func (s *CampaignScheduler) fetchPayamSMSStatus(ctx context.Context, token string, ids []string) ([]PayamStatusResponse, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no ids provided")
	}
	baseURL := "https://www.payamsms.com/report/webservice/status"
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("byCustomer", "false")
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			q.Add("ids", strings.TrimSpace(id))
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("payamsms status http status: %d", resp.StatusCode)
	}

	var out []PayamStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *CampaignScheduler) pushCampaignStatistics(ctx context.Context, processedCampaignID uint, stats map[string]any) error {
	token, err := s.loginBot(ctx)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/v1/bot/campaigns/%d/statistics", s.botCfg.APIDomain, processedCampaignID)
	payload, _ := json.Marshal(dto.BotUpdateCampaignStatisticsRequest{Statistics: stats})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("push statistics http status: %d", resp.StatusCode)
	}
	return nil
}
