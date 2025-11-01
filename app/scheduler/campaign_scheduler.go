// Package scheduler
package scheduler

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"bytes"
	"encoding/json"
	"net/http"

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

	return cancel
}

func (s *CampaignScheduler) runOnce(ctx context.Context) {
	// 2) Login to bot API and get access token
	token, err := s.loginBot(ctx)
	if err != nil {
		s.logger.Printf("scheduler: bot login failed: %v", err)
		return
	}

	// 3) Get ready campaigns
	ready, err := s.listReadyCampaigns(ctx, token)
	if err != nil {
		s.logger.Printf("scheduler: list ready campaigns failed: %v", err)
		return
	}
	if len(ready) == 0 {
		return
	}

	// 4) Filter already processed
	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if err := s.validateCampaign(c); err != nil {
			s.logger.Printf("scheduler: validate campaign failed for id=%d: %v", c.ID, err)
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("scheduler: check processed failed for id=%d: %v", c.ID, err)
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		}
	}
	if len(pending) == 0 {
		return
	}

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
		phones, ids, codes, err = s.fetchAudiencePhones(txCtx, c)
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

	// 12/13) Send batches; after each batch, save sent_sms and update LastAudienceID in SAME transaction
	batchSize := 100
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
		items := make([]payamSMSItem, 0, len(batchPhones))
		// Build SentSMS rows from response
		rows := make([]*models.SentSMS, 0, len(batchPhones))
		for i, p := range batchPhones {
			// 11) Build message
			body := s.buildSMSBody(c, batchCodes[i])

			trackingID := uuid.New().String()

			items = append(items, payamSMSItem{
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

		// Create short links for this batch (if campaign has AdLink) via bot API
		if c.AdLink != nil && *c.AdLink != "" {
			req := dto.BotCreateShortLinksRequest{Items: make([]dto.BotCreateShortLinkRequest, 0, len(batchPhones))}
			for i := range batchPhones {
				campaignID := c.ID
				req.Items = append(req.Items, dto.BotCreateShortLinkRequest{
					UID:         batchCodes[i],
					CampaignID:  &campaignID,
					PhoneNumber: batchPhones[i],
					Link:        *c.AdLink,
				})
			}
			if len(req.Items) > 0 {
				if err := s.createShortLinksViaAPI(ctx, token, &req); err != nil {
					s.logger.Printf("scheduler: failed to create short links via api: %v", err)
					return fmt.Errorf("create short links via api: %w", err)
				}
			}
		}

		// Send via PayamSMS for this batch
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
				}
			}
		}
	}

	// 15) Mark executed
	if err := s.moveCampaignToExecuted(ctx, token, c.ID); err != nil {
		s.logger.Printf("scheduler: move executed failed for id=%d: %v", c.ID, err)
	}
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

func (s *CampaignScheduler) fetchAudiencePhones(ctx context.Context, c dto.BotGetCampaignResponse) ([]string, []int64, []string, error) {
	var result []string
	var ids []int64

	tagIDs := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			return nil, nil, nil, err
		}
		tagIDs[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, tagIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	// create a pq int32 array from tags
	tagIds := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIds[i] = int32(tag.ID)
	}

	// NOTE: len(tagIds) <= len(c.Tags) because some tags may not be found or are inactive

	filter := models.AudienceProfileFilter{
		Tags:  &tagIds,
		Color: utils.ToPtr("white"),
	}

	whites, err := s.audRepo.ByFilter(ctx, filter, "id DESC", int(c.NumAudiences), 0)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, ap := range whites {
		if ap == nil || ap.PhoneNumber == nil || *ap.PhoneNumber == "" {
			continue
		}
		result = append(result, *ap.PhoneNumber)
		ids = append(ids, int64(ap.ID))
		if len(result) >= int(c.NumAudiences) {
			break
		}
	}

	if len(result) >= int(c.NumAudiences) {
		codes := s.generateCodes(c.ID, ids)
		return result, ids, codes, nil
	}

	remaining := int(c.NumAudiences) - len(result)
	upperbound := remaining * 2

	filter.Color = utils.ToPtr("pink")
	pinks, err := s.audRepo.ByFilter(ctx, filter, "id DESC", upperbound, 0)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, ap := range pinks {
		if ap == nil || ap.PhoneNumber == nil || *ap.PhoneNumber == "" {
			continue
		}
		result = append(result, *ap.PhoneNumber)
		ids = append(ids, int64(ap.ID))
		if len(result) >= int(c.NumAudiences) {
			break
		}
	}
	codes := s.generateCodes(c.ID, ids)
	return result, ids, codes, nil
}

func (s *CampaignScheduler) buildSMSBody(c dto.BotGetCampaignResponse, code string) string {
	content := ""
	if c.Content != nil {
		content = *c.Content
	}
	if c.AdLink != nil && *c.AdLink != "" {
		shortened := "https://j01n.ir/s/" + code
		return strings.ReplaceAll(content, "🔗", shortened) + "\n" + "لغو۱۱"

	}
	return strings.ReplaceAll(content, "🔗", "") + "\n" + "لغو۱۱"
}

// generateCode creates a deterministic-like 6-char code per campaign/audience pair
func (s *CampaignScheduler) generateCode(campaignID uint, audienceID int64) string {
	// simple sha1-based hex digest, first 6 chars
	src := fmt.Sprintf("%d-%d-%s", campaignID, audienceID, uuid.New().String())
	// local hash (avoid external utils)
	sum := sha1Sum(src)
	if len(sum) < 6 {
		sum = sum + "000000"
	}
	return sum[:6]
}

func (s *CampaignScheduler) generateCodes(campaignID uint, audienceIDs []int64) []string {
	codes := make([]string, len(audienceIDs))
	for i, id := range audienceIDs {
		codes[i] = s.generateCode(campaignID, id)
	}
	return codes
}

func sha1Sum(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// sendPayamSMSBatchWithBodies sends a batch using custom per-recipient bodies
type payamSMSItem struct {
	Recipient  string
	Body       string
	CustomerID string
}

func (s *CampaignScheduler) sendPayamSMSBatchWithBodies(ctx context.Context, sender string, items []payamSMSItem) ([]struct {
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
