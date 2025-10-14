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
	"sync"

	"gorm.io/gorm"

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
	if logger == nil {
		logger = log.Default()
	}
	s := &CampaignScheduler{
		audRepo:     audRepo,
		tagRepo:     tagRepo,
		sentRepo:    sentRepo,
		pcRepo:      pcRepo,
		notifier:    notifier,
		db:          db,
		logger:      logger,
		interval:    interval,
		payamSMSCfg: payamSMSCfg,
		botCfg:      botCfg,
	}
	if s.botCfg.APIDomain == "" {
		s.botCfg.APIDomain = "https://jazebeh.ir"
	}
	return s
}

// Start launches the scheduler loop in a background goroutine and returns a stop function
func (s *CampaignScheduler) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)

	go func() {
		// 1)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

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
	var wg sync.WaitGroup
	wg.Add(len(pending))
	for _, camp := range pending {
		c := camp
		go func() {
			defer wg.Done()
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
			CampaignID:   c.ID,
			CampaignJSON: func() json.RawMessage { b, _ := json.Marshal(c); return b }(),
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

		// TODO: Update j0in.ir db before sending

		// Send via PayamSMS for this batch
		_, err := s.sendPayamSMSBatchWithBodies(ctx, sender, items)
		if err != nil {
			// TODO: handle error
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
	var out dto.BotLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Session.AccessToken == "" {
		return "", fmt.Errorf("empty bot access token")
	}
	return out.Session.AccessToken, nil
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
	var data dto.BotListCampaignsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Items, nil
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

	tags, err := s.tagRepo.ListByNames(ctx, c.Tags)
	if err != nil {
		return nil, nil, nil, err
	}
	// create a pq int32 array from tags
	tagIds := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIds[i] = int32(tag.ID)
	}

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
		shortened := "https://j0in.ir/" + code
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
	for _, it := range items {
		payload.SMSItems = append(payload.SMSItems, map[string]any{
			"recipient":  it.Recipient,
			"body":       it.Body,
			"customerId": it.CustomerID,
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
