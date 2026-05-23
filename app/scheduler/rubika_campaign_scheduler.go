package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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
	defaultRubikaBaseURL    = "https://messaging.rubika.ir"
	rubikaSendMaxRetries    = 5
	rubikaSendBatchSize     = 200
	rubikaStatusJobMaxRetry = 3
)

type RubikaCampaignScheduler struct {
	audRepo      repository.AudienceProfileRepository
	tagRepo      repository.TagRepository
	sentRepo     repository.SentRubikaMessageRepository
	pcRepo       repository.ProcessedCampaignRepository
	jobRepo      repository.CampaignStatusJobRepository
	resRepo      repository.RubikaStatusResultRepository
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
	FetchStatus(ctx context.Context, messageIDs []string) ([]RubikaStatusResponse, error)
	SupportsStatusTracking() bool
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

type RubikaStatusResponse struct {
	MessageID string          `json:"message_id,omitempty"`
	Status    string          `json:"status,omitempty"`
	StatusDet string          `json:"status_det,omitempty"`
	RawBody   json.RawMessage `json:"-"`
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

func (c *httpRubikaClient) SupportsStatusTracking() bool {
	return false
}

func (c *httpRubikaClient) FetchStatus(_ context.Context, _ []string) ([]RubikaStatusResponse, error) {
	return nil, fmt.Errorf("rubika status tracking is not supported by the configured client yet")
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
	sentRepo repository.SentRubikaMessageRepository,
	pcRepo repository.ProcessedCampaignRepository,
	jobRepo repository.CampaignStatusJobRepository,
	resRepo repository.RubikaStatusResultRepository,
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

func (s *RubikaCampaignScheduler) runOnce(ctx context.Context, parent context.Context) {
	token, err := s.botClient.Login(ctx)
	if err != nil {
		s.logger.Printf("Rubika scheduler: bot login failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Rubika scheduler bot login failed: %v", err))
		return
	}

	ready, err := s.botClient.ListReadyCampaigns(ctx, token, models.CampaignPlatformRubika)
	if err != nil {
		s.logger.Printf("Rubika scheduler: list ready campaigns failed: %v", err)
		s.notifyAdmin(fmt.Sprintf("Rubika scheduler list ready campaigns failed: %v", err))
		return
	}
	if len(ready) == 0 {
		return
	}
	s.logger.Printf("Rubika scheduler: listed %d ready campaigns", len(ready))

	pending := make([]dto.BotGetCampaignResponse, 0, len(ready))
	for _, c := range ready {
		if strings.ToLower(strings.TrimSpace(c.Platform)) != models.CampaignPlatformRubika {
			s.logger.Printf("Rubika scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform)
			s.notifyAdmin(fmt.Sprintf("Rubika scheduler: campaign id=%d has unsupported platform %q, skipping", c.ID, c.Platform))
			continue
		}
		if err := s.validateRubikaCampaign(c); err != nil {
			s.logger.Printf("Rubika scheduler: validate campaign failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Rubika scheduler: validate campaign failed for id=%d: %v", c.ID, err))
			continue
		}
		pc, err := s.pcRepo.ByCampaignID(ctx, c.ID)
		if err != nil {
			s.logger.Printf("Rubika scheduler: check processed failed for campaign id=%d (skipped): %v", c.ID, err)
			s.notifyAdmin(fmt.Sprintf("Rubika scheduler: check processed failed for id=%d: %v", c.ID, err))
			continue
		}
		if pc == nil {
			pending = append(pending, c)
		} else {
			s.logger.Printf("Rubika scheduler: campaign id=%d already processed, skipping", c.ID)
		}
	}
	if len(pending) == 0 {
		return
	}
	s.logger.Printf("Rubika scheduler: %d campaigns pending processing...", len(pending))

	for _, camp := range pending {
		go func(c dto.BotGetCampaignResponse) {
			// TODO: Make 4 hours configurable or use a more dynamic approach based on campaign content/size
			ctx2, cancel2 := context.WithTimeout(parent, 4*time.Hour)
			defer cancel2()
			if err := s.processRubikaCampaign(ctx2, token, c); err != nil {
				s.logger.Printf("Rubika scheduler: process campaign id=%d failed: %v", c.ID, err)
				s.notifyAdmin(fmt.Sprintf("Rubika scheduler: process campaign failed for campaign id=%d: %v", c.ID, err))
			}
		}(camp)
	}
}

func (s *RubikaCampaignScheduler) processRubikaCampaign(ctx context.Context, token string, c dto.BotGetCampaignResponse) error {
	serviceID, err := s.extractRubikaServiceID(c)
	if err != nil {
		return fmt.Errorf("resolve Rubika service id for campaign id=%d: %w", c.ID, err)
	}
	if c.NumAudiences == nil || *c.NumAudiences <= 0 {
		return fmt.Errorf("campaign id=%d has no audiences", c.ID)
	}

	if err := s.botClient.MoveCampaignToRunning(ctx, token, c.ID); err != nil {
		return fmt.Errorf("move campaign id=%d to running: %w", c.ID, err)
	}
	s.logger.Printf("Rubika scheduler: campaign id=%d moved to running", c.ID)

	// Fetch audience data OUTSIDE any DB transaction.
	// AllocateShortLinks and DownloadTargetAudienceExcelFile are external HTTP calls that can
	// take 60+ seconds for large audiences. Holding a Postgres transaction open during these
	// calls triggers idle_in_transaction_session_timeout, killing the connection with
	// "driver: bad connection" on the next SQL statement.
	var (
		phones       []string
		ids          []int64
		codes        []string
		unmatchedUID []string
		selectionID  *uint
	)
	if hasTargetAudienceExcelFileUUID(c.TargetAudienceExcelFileUUID) {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before fetching excel UIDs for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("Rubika scheduler: campaign id=%d fetching audience UIDs from excel", c.ID)
		fileUIDs, err := fetchTargetAudienceUIDsFromExcel(ctx, s.botClient, token, c.ID)
		if err != nil {
			return fmt.Errorf("fetch excel UIDs for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("Rubika scheduler: campaign id=%d resolving %d UIDs to phones", c.ID, len(fileUIDs))
		audienceResult, err := fetchAudiencePhonesByUIDs(ctx, s.logger, s.audRepo, s.botClient, c, token, fileUIDs, "jo1n.ir/")
		if err != nil {
			return fmt.Errorf("fetch audience phones by UIDs for campaign id=%d: %w", c.ID, err)
		}
		phones = audienceResult.Phones
		ids = audienceResult.IDs
		codes = audienceResult.Codes
		unmatchedUID = audienceResult.UnmatchedUIDs
		selectionID = nil
		s.logger.Printf("Rubika scheduler: campaign id=%d fetched %d phones via excel (unmatched=%d)", c.ID, len(phones), len(unmatchedUID))
	} else {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before fetching audiences for campaign id=%d: %w", c.ID, err)
		}
		correlationID := uuid.NewString()
		s.logger.Printf("Rubika scheduler: campaign id=%d fetching audience phones (correlation_id=%s)", c.ID, correlationID)
		audienceResult, err := s.fetchRubikaAudiencePhones(ctx, c, token, correlationID)
		if err != nil {
			return fmt.Errorf("fetch audience phones for campaign id=%d: %w", c.ID, err)
		}
		phones = audienceResult.Phones
		ids = audienceResult.IDs
		codes = audienceResult.Codes
		selectionID = utils.ToPtr(audienceResult.SelectionID)
		s.logger.Printf("Rubika scheduler: campaign id=%d fetched %d phones (selection_id=%d)", c.ID, len(phones), audienceResult.SelectionID)
	}

	if len(ids) != len(phones) {
		return fmt.Errorf("audience ids mismatch for campaign id=%d: phones=%d ids=%d", c.ID, len(phones), len(ids))
	}
	if len(codes) != len(phones) {
		return fmt.Errorf("audience codes mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}
	s.logger.Printf("Rubika scheduler: campaign id=%d audience ready: phones=%d unmatched=%d", c.ID, len(phones), len(unmatchedUID))

	// Persist ProcessedCampaign and all audience data in one focused transaction.
	// No external calls here — the transaction stays short and the connection stays active.
	var pc *models.ProcessedCampaign
	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		pc = &models.ProcessedCampaign{
			CampaignID:          c.ID,
			CampaignJSON:        func() json.RawMessage { b, _ := json.Marshal(c); return b }(),
			AudienceIDs:         pq.Int64Array{},
			AudienceCodes:       []string{},
			LastAudienceID:      nil,
			AudienceSelectionID: selectionID,
			Statistics:          nil,
		}
		if err := s.pcRepo.Save(txCtx, pc); err != nil {
			return fmt.Errorf("save processed campaign: %w", err)
		}
		s.logger.Printf("Rubika scheduler: persisted processed campaign id=%d for campaign id=%d", pc.ID, c.ID)

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
		s.logger.Printf("Rubika scheduler: updated processed campaign id=%d with %d audience ids", pc.ID, len(ids))
		return nil
	}); err != nil {
		return fmt.Errorf("persist campaign data for campaign id=%d: %w", c.ID, err)
	}
	s.logger.Printf("Rubika scheduler: persisted processed campaign id=%d num_phones=%d, num_ids=%d, num_codes=%d, num_unmatched=%d", pc.ID, len(phones), len(ids), len(codes), len(unmatchedUID))

	if len(unmatchedUID) > 0 {
		s.logger.Printf("Rubika scheduler: campaign id=%d creating %d unmatched sent rows for processed_campaign_id=%d", c.ID, len(unmatchedUID), pc.ID)
		if err := s.createUnmatchedSentRubikaRows(ctx, pc.ID, unmatchedUID); err != nil {
			return fmt.Errorf("create unmatched sent rows for campaign id=%d: %w", c.ID, err)
		}
	}

	var fileID *string
	if c.MediaUUID != nil {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired before uploading media for campaign id=%d: %w", c.ID, err)
		}
		s.logger.Printf("Rubika scheduler: campaign id=%d uploading media uuid=%s", c.ID, c.MediaUUID)
		id, err := s.uploadCampaignMedia(ctx, token, c)
		if err != nil {
			return fmt.Errorf("upload media for campaign id=%d: %w", c.ID, err)
		}
		fileID = id
		s.logger.Printf("Rubika scheduler: campaign id=%d media uploaded file_id=%v", c.ID, fileID)
	}

	for start := 0; start < len(phones); start += rubikaSendBatchSize {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context expired at batch start=%d for campaign id=%d: %w", start, c.ID, err)
		}

		end := min(start+rubikaSendBatchSize, len(phones))
		batchPhones := phones[start:end]
		batchIDs := ids[start:end]
		batchCodes := codes[start:end]

		items := make([]RubikaMessagePayload, 0, len(batchPhones))
		rows := make([]*models.SentRubikaMessage, 0, len(batchPhones))

		s.logger.Printf("Rubika scheduler: campaign id=%d allocating tracking ids for batch [%d,%d)", c.ID, start, end)
		trackingIDs, err := allocateTrackingIDs(ctx, s.db, len(batchPhones))
		if err != nil {
			return fmt.Errorf("allocate tracking ids for batch [%d,%d) campaign id=%d: %w", start, end, c.ID, err)
		}

		for i, p := range batchPhones {
			trackingID := trackingIDs[i]
			items = append(items, RubikaMessagePayload{
				Phone:  p,
				Text:   s.buildRubikaMessageBody(c, batchCodes[i]),
				FileID: fileID,
			})
			rows = append(rows, &models.SentRubikaMessage{
				ProcessedCampaignID: pc.ID,
				PhoneNumber:         p,
				PartsDelivered:      0,
				Status:              models.RubikaSendStatusPending,
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
		s.logger.Printf("Rubika scheduler: campaign id=%d batch [%d,%d) saved, sending to Rubika", c.ID, start, end)

		resp, batchErr := s.sendWithRetry(ctx, serviceID, items)
		if batchErr != nil {
			s.logger.Printf("Rubika scheduler: send batch [%d,%d) failed for campaign id=%d: %v", start, end, c.ID, batchErr)
		}
		s.logger.Printf("Rubika scheduler: campaign id=%d batch [%d,%d) Rubika responded: sent=%d responses=%d", c.ID, start, end, len(items), rubikaResponseLen(resp))

		sendUpdates := make([]repository.SentRubikaSendResultUpdate, 0, len(items))
		for i, item := range items {
			sendUpdates = append(sendUpdates, buildRubikaSendResultUpdate(trackingIDs[i], item.Phone, rubikaResponseItem(resp, i, item.Phone), batchErr))
		}
		if len(sendUpdates) > 0 {
			if updateErr := s.sentRepo.UpdateSendResultByTrackingIDs(ctx, sendUpdates); updateErr != nil {
				s.logger.Printf("Rubika scheduler: failed to batch update sent_rubika provider fields for campaign id=%d: %v", c.ID, updateErr)
				// NOTE: Error silent here; not returning to avoid blocking further processing
			}
		}

		if err := s.scheduleStatusCheckJobs(ctx, pc.ID, trackingIDs); err != nil {
			s.logger.Printf("Rubika scheduler: failed to schedule status jobs for campaign id=%d: %v", c.ID, err)
			// NOTE: Error silent here; not returning to avoid blocking further processing
		}
		s.logger.Printf("Rubika scheduler: campaign id=%d batch [%d,%d) done, sleeping message_delay", c.ID, start, end)
		if err := sleepWithContext(ctx, s.messageDelay); err != nil {
			return fmt.Errorf("interrupted during batch delay at [%d,%d) for campaign id=%d: %w", start, end, c.ID, err)
		}
	}

	stats, err := s.updateProcessedCampaignStats(ctx, pc.ID)
	if err != nil {
		return fmt.Errorf("update stats for campaign id=%d: %w", c.ID, err)
	}
	if err := s.botClient.PushCampaignStatistics(ctx, c.ID, stats); err != nil {
		return fmt.Errorf("push statistics for campaign id=%d: %w", c.ID, err)
	}

	s.logger.Printf("Rubika scheduler: campaign id=%d all batches sent", c.ID)

	if err := s.botClient.MoveCampaignToExecuted(ctx, token, c.ID); err != nil {
		return fmt.Errorf("move campaign id=%d to executed: %w", c.ID, err)
	}
	s.logger.Printf("Rubika scheduler: campaign id=%d moved to executed", c.ID)

	return nil
}

func (s *RubikaCampaignScheduler) uploadCampaignMedia(ctx context.Context, token string, c dto.BotGetCampaignResponse) (*string, error) {
	if c.MediaUUID == nil || *c.MediaUUID == uuid.Nil {
		return nil, nil
	}

	path, err := s.botClient.DownloadCampaignMedia(ctx, token, c.MediaUUID.String())
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

func (s *RubikaCampaignScheduler) fetchRubikaAudiencePhones(
	ctx context.Context,
	c dto.BotGetCampaignResponse,
	token string,
	correlationID string,
) (*AudiencePhonesResult, error) {
	numAudiences := int64(0)
	if c.NumAudiences != nil {
		numAudiences = int64(*c.NumAudiences)
	}
	s.logger.Printf("fetchRubikaAudiencePhones start: campaign_id=%d customer_id=%d num_audiences=%d tags_length=%d correlation_id=%s", c.ID, c.CustomerID, numAudiences, len(c.Tags), correlationID)

	if numAudiences <= 0 {
		return nil, fmt.Errorf("campaign num_audiences must be positive")
	}

	toExtract := make([]uint, len(c.Tags))
	for i, tag := range c.Tags {
		tagID, err := strconv.ParseUint(tag, 10, 32)
		if err != nil {
			s.logger.Printf("fetchRubikaAudiencePhones tag parse failed: campaign_id=%d tag=%q err=%v", c.ID, tag, err)
			return nil, err
		}
		toExtract[i] = uint(tagID)
	}
	tags, err := s.tagRepo.ListByIDs(ctx, toExtract)
	if err != nil {
		s.logger.Printf("fetchRubikaAudiencePhones tags lookup failed: campaign_id=%d err=%v", c.ID, err)
		return nil, err
	}

	tagIDs := make(pq.Int32Array, len(tags))
	for i, tag := range tags {
		tagIDs[i] = int32(tag.ID)
	}
	s.logger.Printf("fetchRubikaAudiencePhones tags resolved: campaign_id=%d requested=%d resolved=%d", c.ID, len(c.Tags), len(tagIDs))

	const limit = 10000000
	tagsHash := hashTags(c.Tags)
	selection, err := s.audienceCache.Latest(ctx, c.CustomerID, tagsHash)
	if err != nil {
		s.logger.Printf("fetchRubikaAudiencePhones latest selection failed: campaign_id=%d customer_id=%d tags_hash=%s err=%v", c.ID, c.CustomerID, tagsHash, err)
		return nil, err
	}
	if selection != nil {
		s.logger.Printf("fetchRubikaAudiencePhones selection hit: campaign_id=%d selection_id=%d prior_ids_length=%d", c.ID, selection.ID, len(selection.IDs))
	} else {
		s.logger.Printf("fetchRubikaAudiencePhones selection miss: campaign_id=%d", c.ID)
	}

	selectAudiences := func(exclude map[int64]struct{}) ([]string, []int64, []string, error) {
		phones := make([]string, 0, numAudiences)
		ids := make([]int64, 0, numAudiences)
		uids := make([]string, 0, numAudiences)

		// Rubika campaigns intentionally do not segment audiences by color (white/pink).
		// Color-based routing is SMS-specific, so Rubika always queries by tag criteria only.
		filter := models.AudienceProfileFilter{Tags: &tagIDs}
		candidates, err := s.audRepo.ByFilter(ctx, filter, "id DESC", limit, 0)
		if err != nil {
			s.logger.Printf("fetchRubikaAudiencePhones fetch candidates failed: campaign_id=%d err=%v", c.ID, err)
			return nil, nil, nil, err
		}
		s.logger.Printf("fetchRubikaAudiencePhones candidates: campaign_id=%d count=%d", c.ID, len(candidates))

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

	var exclude map[int64]struct{}
	if selection != nil && selection.IDs != nil {
		exclude = selection.IDs
	}
	phones, ids, uids, err := selectAudiences(exclude)
	if err != nil {
		return nil, err
	}
	s.logger.Printf("fetchRubikaAudiencePhones selected (with exclusions): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)

	resetUsed := false
	if int64(len(phones)) < numAudiences {
		resetUsed = true
		phones, ids, uids, err = selectAudiences(nil)
		if err != nil {
			return nil, err
		}
		s.logger.Printf("fetchRubikaAudiencePhones selected (reset): campaign_id=%d selected=%d requested=%d", c.ID, len(phones), numAudiences)
	}

	var sel *AudienceSelection
	if resetUsed {
		sel, err = s.audienceCache.SaveSnapshot(ctx, c.CustomerID, tagsHash, correlationID, ids)
	} else {
		sel, err = s.audienceCache.SaveWithMerge(ctx, c.CustomerID, tagsHash, correlationID, ids)
	}
	if err != nil {
		s.logger.Printf("fetchRubikaAudiencePhones selection save failed: campaign_id=%d err=%v reset=%t", c.ID, err, resetUsed)
		return nil, err
	}
	s.logger.Printf("fetchRubikaAudiencePhones selection saved: campaign_id=%d selection_id=%d reset=%t selected=%d", c.ID, sel.ID, resetUsed, len(ids))

	if !hasCampaignAdLink(c.AdLink) {
		s.logger.Printf("fetchRubikaAudiencePhones skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		s.logger.Printf("fetchRubikaAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d ad_link=empty", c.ID, len(phones), len(phones), sel.ID)
		return &AudiencePhonesResult{
			Phones:      phones,
			IDs:         ids,
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
	codes, err := s.botClient.AllocateShortLinks(ctx, token, &dto.BotAllocateShortLinksRequest{
		CampaignID:      c.ID,
		Items:           items,
		ShortLinkDomain: "jo1n.ir/", // TODO:
	})
	if err != nil {
		s.logger.Printf("fetchRubikaAudiencePhones allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, err
	}
	if len(codes) != len(phones) {
		return nil, fmt.Errorf("allocate short links length mismatch for campaign id=%d: phones=%d codes=%d", c.ID, len(phones), len(codes))
	}
	s.logger.Printf("fetchRubikaAudiencePhones success: campaign_id=%d selected=%d codes_length=%d selection_id=%d", c.ID, len(phones), len(codes), sel.ID)
	return &AudiencePhonesResult{
		Phones:      phones,
		IDs:         ids,
		Codes:       codes,
		SelectionID: sel.ID,
	}, nil
}

func (s *RubikaCampaignScheduler) buildRubikaMessageBody(c dto.BotGetCampaignResponse, code string) string {
	content := ""
	if c.Content != nil {
		content = *c.Content
	}
	if hasCampaignAdLink(c.AdLink) {
		shortened := "jo1n.ir/" + code // TODO:
		return strings.ReplaceAll(content, "🔗", shortened)
	}
	return strings.ReplaceAll(content, "🔗", "")
}

func (s *RubikaCampaignScheduler) createUnmatchedSentRubikaRows(ctx context.Context, processedCampaignID uint, unmatchedUIDs []string) error {
	if len(unmatchedUIDs) == 0 {
		return nil
	}

	const errCode = "AUDIENCE_UID_NOT_FOUND"
	rows := make([]*models.SentRubikaMessage, 0, len(unmatchedUIDs))
	for _, uid := range unmatchedUIDs {
		desc := fmt.Sprintf("Audience uid not found or has no phone number: %s", uid)
		code := errCode
		rows = append(rows, &models.SentRubikaMessage{
			ProcessedCampaignID: processedCampaignID,
			PhoneNumber:         "",
			PartsDelivered:      0,
			Status:              models.RubikaSendStatusUnsuccessful,
			TrackingID:          uuid.NewString(),
			ServerID:            nil,
			ErrorCode:           &code,
			Description:         &desc,
		})
	}
	return s.sentRepo.SaveBatch(ctx, rows)
}

func (s *RubikaCampaignScheduler) scheduleStatusCheckJobs(ctx context.Context, processedCampaignID uint, trackingIDs []string) error {
	if len(trackingIDs) == 0 || !s.rubikaClient.SupportsStatusTracking() || s.jobRepo == nil {
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
			TrackingIDs:         pq.StringArray(filteredTrackingIDs),
			RetryCount:          0,
			ScheduledAt:         now.Add(off),
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return s.jobRepo.SaveBatch(ctx, jobs)
}

func (s *RubikaCampaignScheduler) startStatusJobWorker(parent context.Context) {
	ticker := time.NewTicker(statusJobWorkerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-parent.Done():
			return
		case <-ticker.C:
			if !s.rubikaClient.SupportsStatusTracking() || s.jobRepo == nil || s.resRepo == nil {
				continue
			}

			listCtx, listCancel := context.WithTimeout(parent, 30*time.Second)
			jobs, err := s.jobRepo.ListDue(listCtx, utils.UTCNow(), numJobsPerTick)
			listCancel()
			if err != nil {
				s.logger.Printf("Rubika scheduler: list status jobs failed: %v", err)
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
					s.logger.Printf("Rubika scheduler: handle status job id=%d failed: %v", job.ID, err)
					if job.RetryCount >= rubikaStatusJobMaxRetry {
						s.notifyAdmin(fmt.Sprintf("Rubika scheduler: status job id=%d has failed %d times with error: %v", job.ID, job.RetryCount, err))
					}
				} else {
					s.logger.Printf("Rubika scheduler: handle status job id=%d succeeded", job.ID)
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

func (s *RubikaCampaignScheduler) handleStatusJob(ctx context.Context, job *models.CampaignStatusJob) error {
	rows, err := s.sentRepo.ListByTrackingIDs(ctx, job.ProcessedCampaignID, []string(job.TrackingIDs))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return s.markStatusJobExecuted(ctx, job, nil)
	}

	rowByServerID := make(map[string]*models.SentRubikaMessage, len(rows))
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

	statusItems, fetchErr := s.rubikaClient.FetchStatus(ctx, serverIDs)
	if fetchErr != nil {
		now := utils.UTCNow()
		job.RetryCount++
		msg := fetchErr.Error()
		job.Error = &msg
		job.UpdatedAt = now
		if job.RetryCount >= rubikaStatusJobMaxRetry {
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

		statusRows := make([]*models.RubikaStatusResult, 0, len(statusItems))
		sendUpdates := make([]repository.SentRubikaSendResultUpdate, 0, len(statusItems))
		for _, item := range statusItems {
			serverID := strings.TrimSpace(item.MessageID)
			if serverID == "" {
				continue
			}
			row := rowByServerID[serverID]
			if row == nil {
				continue
			}

			totalParts, deliveredParts, undeliveredParts, unknownParts, status := mapRubikaProviderStatus(item)
			statusText := strings.TrimSpace(item.StatusDet)
			if statusText == "" {
				statusText = strings.TrimSpace(item.Status)
			}

			var errorCode *string
			var description *string
			if status == models.RubikaSendStatusUnsuccessful {
				code := strings.TrimSpace(item.Status)
				if code == "" {
					code = "RUBIKA_STATUS_UNSUCCESSFUL"
				}
				errorCode = &code
			}
			if metadataDesc := buildRubikaStatusMetadataDescription(row.Description, item.Status, statusText); metadataDesc != "" {
				description = &metadataDesc
			}

			sendUpdates = append(sendUpdates, repository.SentRubikaSendResultUpdate{
				TrackingID:     row.TrackingID,
				Status:         status,
				PartsDelivered: int(deliveredParts),
				ServerID:       row.ServerID,
				ErrorCode:      errorCode,
				Description:    description,
			})

			statusValue := strings.TrimSpace(item.Status)
			var statusPtr *string
			if statusValue != "" {
				statusPtr = &statusValue
			}
			var statusTextPtr *string
			if statusText != "" {
				statusTextPtr = &statusText
			}
			metadata := buildRubikaStatusResultMetadata(item, status)

			statusRows = append(statusRows, &models.RubikaStatusResult{
				JobID:                 job.ID,
				ProcessedCampaignID:   job.ProcessedCampaignID,
				TrackingID:            row.TrackingID,
				ServerID:              row.ServerID,
				ProviderStatusCode:    nil,
				ProviderStatusText:    statusTextPtr,
				TotalParts:            &totalParts,
				TotalDeliveredParts:   &deliveredParts,
				TotalUndeliveredParts: &undeliveredParts,
				TotalUnknownParts:     &unknownParts,
				Status:                statusPtr,
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
		if err := s.botClient.PushCampaignStatistics(ctx, pc.CampaignID, stats); err != nil {
			return err
		}
	}
	return nil
}

func (s *RubikaCampaignScheduler) markStatusJobExecuted(ctx context.Context, job *models.CampaignStatusJob, errText *string) error {
	now := utils.UTCNow()
	job.ExecutedAt = &now
	job.UpdatedAt = now
	job.Error = errText
	return s.jobRepo.Update(ctx, job)
}

func mapRubikaProviderStatus(item RubikaStatusResponse) (totalParts int64, deliveredParts int64, undeliveredParts int64, unknownParts int64, status models.RubikaSendStatus) {
	totalParts = 1
	normalized := strings.ToLower(strings.TrimSpace(item.Status))
	detail := strings.ToLower(strings.TrimSpace(item.StatusDet))
	switch {
	case normalized == "ok", normalized == "success", normalized == "successful", normalized == "sent", normalized == "delivered", strings.Contains(detail, "success"), strings.Contains(detail, "delivered"):
		return 1, 1, 0, 0, models.RubikaSendStatusSuccessful
	case normalized == "failed", normalized == "fail", normalized == "error", normalized == "rejected", strings.Contains(detail, "fail"), strings.Contains(detail, "error"), strings.Contains(detail, "reject"):
		return 1, 0, 1, 0, models.RubikaSendStatusUnsuccessful
	default:
		return 1, 0, 0, 1, models.RubikaSendStatusPending
	}
}

func buildRubikaStatusMetadataDescription(existing *string, status string, statusText string) string {
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
		"status": strings.TrimSpace(status),
		"text":   strings.TrimSpace(statusText),
		"at":     utils.UTCNow().Format(time.RFC3339),
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return strings.TrimSpace(statusText)
	}
	return string(data)
}

func buildRubikaStatusResultMetadata(item RubikaStatusResponse, normalizedStatus models.RubikaSendStatus) json.RawMessage {
	metadata := map[string]any{
		"messageID":        strings.TrimSpace(item.MessageID),
		"status":           strings.TrimSpace(item.Status),
		"statusText":       strings.TrimSpace(item.StatusDet),
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

func (s *RubikaCampaignScheduler) updateProcessedCampaignStats(ctx context.Context, processedCampaignID uint) (map[string]any, error) {
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

func (s *RubikaCampaignScheduler) updateProcessedCampaignStatsFromSentRows(ctx context.Context, pc *models.ProcessedCampaign) (map[string]any, error) {
	s.logger.Printf("updateProcessedCampaignStatsFromSentRows: computing stats from sent rows for processed_campaign_id=%d", pc.ID)
	type row struct {
		Total      int64
		Successful int64
	}
	var agg row
	if err := s.db.WithContext(ctx).Table("sent_rubika_messages").
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

func rubikaResponseLen(resp *RubikaSendBulkMessagesResponse) int {
	if resp == nil {
		return 0
	}
	return len(resp.Data.MessageStatusList)
}

func rubikaResponseItem(resp *RubikaSendBulkMessagesResponse, index int, phone string) *RubikaMessageStatus {
	if resp == nil {
		return nil
	}
	normalizedPhone := strings.TrimSpace(phone)
	for i := range resp.Data.MessageStatusList {
		item := resp.Data.MessageStatusList[i]
		if normalizedPhone != "" && strings.TrimSpace(item.Phone) == normalizedPhone {
			return &item
		}
	}
	if index >= 0 && index < len(resp.Data.MessageStatusList) {
		item := resp.Data.MessageStatusList[index]
		return &item
	}
	return nil
}

func buildRubikaSendResultUpdate(trackingID string, phone string, resp *RubikaMessageStatus, sendErr error) repository.SentRubikaSendResultUpdate {
	update := repository.SentRubikaSendResultUpdate{
		TrackingID:     trackingID,
		PartsDelivered: 0,
		Status:         models.RubikaSendStatusUnsuccessful,
	}

	if sendErr != nil {
		code := "SEND_FAILED"
		desc := sendErr.Error()
		update.ErrorCode = &code
		update.Description = &desc
		return update
	}

	if resp == nil {
		code := "MISSING_SEND_RESPONSE"
		desc := fmt.Sprintf("missing send response for tracking_id=%s phone=%s", trackingID, phone)
		update.ErrorCode = &code
		update.Description = &desc
		return update
	}

	if rubikaMessageStatusSuccessful(*resp) {
		update.Status = models.RubikaSendStatusSuccessful
		update.PartsDelivered = 1
		if strings.TrimSpace(resp.MessageID) != "" {
			id := strings.TrimSpace(resp.MessageID)
			update.ServerID = &id
		}
	} else {
		code := strings.TrimSpace(resp.Status)
		if code == "" {
			code = "SEND_UNSUCCESSFUL"
		}
		update.ErrorCode = &code
	}

	desc := buildRubikaSendMetadataDescription(resp, "")
	if strings.TrimSpace(desc) != "" {
		update.Description = &desc
	}
	return update
}

func buildRubikaSendMetadataDescription(resp *RubikaMessageStatus, fallback string) string {
	if resp == nil {
		return strings.TrimSpace(fallback)
	}
	metadata := map[string]any{}
	if strings.TrimSpace(resp.MessageID) != "" {
		metadata["messageID"] = strings.TrimSpace(resp.MessageID)
	}
	if strings.TrimSpace(resp.Phone) != "" {
		metadata["phone"] = strings.TrimSpace(resp.Phone)
	}
	if strings.TrimSpace(resp.Status) != "" {
		metadata["status"] = strings.TrimSpace(resp.Status)
	}
	if strings.TrimSpace(resp.StatusDet) != "" {
		metadata["statusDet"] = strings.TrimSpace(resp.StatusDet)
	}
	if trimmedFallback := strings.TrimSpace(fallback); trimmedFallback != "" {
		metadata["error"] = trimmedFallback
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return strings.TrimSpace(fallback)
	}
	return string(data)
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
