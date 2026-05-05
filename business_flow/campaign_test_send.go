package businessflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/scheduler"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
)

const campaignTestCooldown = 30 * time.Second

func (s *CampaignFlowImpl) SendCampaignTestMessage(ctx context.Context, req *dto.SendCampaignTestMessageRequest, metadata *ClientMetadata) (*dto.SendCampaignTestMessageResponse, error) {
	if req == nil {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_INVALID_REQUEST", "request is required", ErrInvalidState)
	}
	if strings.TrimSpace(req.UUID) == "" {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_INVALID_REQUEST", "campaign uuid is required", ErrCampaignUUIDRequired)
	}
	if req.CustomerID == 0 {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_INVALID_REQUEST", "customer id is required", ErrCustomerNotFound)
	}

	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_CUSTOMER_LOOKUP_FAILED", "failed to lookup customer", err)
	}
	campaign, err := getCampaign(ctx, s.campaignRepo, req.UUID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_CAMPAIGN_LOOKUP_FAILED", "failed to lookup campaign", err)
	}
	if !canTestSendCampaign(campaign.Status) {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_STATE_NOT_ALLOWED", "campaign state does not allow test sending", ErrCampaignTestStateNotAllowed)
	}

	platform, err := sanitizeCampaignPlatform(utils.ToPtr(campaign.Spec.Platform))
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_PLATFORM_INVALID", "campaign platform is invalid", err)
	}

	content := ""
	if campaign.Spec.Content != nil {
		content = strings.TrimSpace(*campaign.Spec.Content)
	}
	if content == "" {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_CONTENT_REQUIRED", "campaign content is required", ErrCampaignContentRequired)
	}

	recipient := strings.TrimSpace(customer.RepresentativeMobile)
	if recipient == "" {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_RECIPIENT_MISSING", "representative mobile is missing", ErrCampaignTestRecipientMissing)
	}

	ok, ttl, err := s.tryAcquireCampaignTestCooldown(ctx, customer.ID, campaignTestCooldown)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_COOLDOWN_UNAVAILABLE", "failed to enforce campaign test cooldown", ErrCampaignTestCooldownUnavailable)
	}
	if !ok {
		secs := int(math.Ceil(ttl.Seconds()))
		if secs <= 0 {
			secs = int(campaignTestCooldown.Seconds())
		}
		return nil, NewBusinessErrorf("CAMPAIGN_TEST_SEND_RATE_LIMITED", "Please wait %d seconds before sending another test message", ErrCampaignTestRateLimited, secs)
	}

	fakeCode := buildFakeShortCode()
	body := buildCampaignTestMessageBody(platform, campaign.Spec.Content, campaign.Spec.AdLink, fakeCode)

	var fakeShortLink *string
	if hasCampaignAdLink(campaign.Spec.AdLink) {
		v := "jo1n.ir/" + fakeCode
		fakeShortLink = &v
	}

	softErr, hardErr := s.sendCampaignTestMessageBestEffort(ctx, campaign, platform, recipient, body)
	if hardErr != nil {
		errMsg := fmt.Sprintf("Campaign test send failed: %v", hardErr)
		_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignTestSend, errMsg, false, &errMsg, metadata)
		return nil, NewBusinessError("CAMPAIGN_TEST_SEND_FAILED", "campaign test send failed", hardErr)
	}

	var warning *string
	auditSuccess := true
	var auditErrMsg *string
	if softErr != nil {
		w := fmt.Sprintf("Provider call returned error; delivery was attempted best-effort: %v", softErr)
		warning = &w
		auditSuccess = false
		e := softErr.Error()
		auditErrMsg = &e
	}

	auditDesc := map[string]any{
		"campaign_id":      campaign.ID,
		"campaign_uuid":    campaign.UUID.String(),
		"platform":         platform,
		"delivery":         "best_effort_attempted",
		"recipient_masked": maskPhoneNumber(recipient),
		"has_warning":      softErr != nil,
	}
	if fakeShortLink != nil {
		auditDesc["fake_short_link"] = *fakeShortLink
	}
	auditDescBytes, _ := json.Marshal(auditDesc)
	auditDescText := string(auditDescBytes)
	_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignTestSend, auditDescText, auditSuccess, auditErrMsg, metadata)

	return &dto.SendCampaignTestMessageResponse{
		Message:   "Campaign test message attempted",
		Platform:  platform,
		Recipient: recipient,
		Delivery:  "best_effort_attempted",
		Warning:   warning,
	}, nil
}

func canTestSendCampaign(status models.CampaignStatus) bool {
	return status == models.CampaignStatusInitiated || status == models.CampaignStatusInProgress
}

func (s *CampaignFlowImpl) tryAcquireCampaignTestCooldown(ctx context.Context, customerID uint, ttl time.Duration) (bool, time.Duration, error) {
	if s.rc == nil {
		return false, 0, ErrCampaignTestCooldownUnavailable
	}
	key := redisKey(s.cacheConfig, fmt.Sprintf("campaign_test_send:%d", customerID))
	ok, err := s.rc.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, 0, err
	}
	if ok {
		return true, ttl, nil
	}

	remaining, err := s.rc.TTL(ctx, key).Result()
	if err != nil {
		return false, ttl, nil
	}
	if remaining < 0 {
		remaining = ttl
	}
	return false, remaining, nil
}

func buildFakeShortCode() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	return "tst" + id
}

func hasCampaignAdLink(adLink *string) bool {
	return adLink != nil && strings.TrimSpace(*adLink) != ""
}

func buildCampaignTestMessageBody(platform string, contentPtr *string, adLink *string, fakeCode string) string {
	content := ""
	if contentPtr != nil {
		content = *contentPtr
	}
	if hasCampaignAdLink(adLink) {
		content = strings.ReplaceAll(content, "🔗", "jo1n.ir/"+fakeCode)
	} else {
		content = strings.ReplaceAll(content, "🔗", "")
	}
	if platform == models.CampaignPlatformSMS {
		return content + "\n" + "لغو۱۱"
	}
	return content
}

func (s *CampaignFlowImpl) sendCampaignTestMessageBestEffort(
	ctx context.Context,
	campaign models.Campaign,
	platform string,
	recipient string,
	body string,
) (softErr error, hardErr error) {
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	switch platform {
	case models.CampaignPlatformSMS:
		lineNumber := ""
		if campaign.Spec.LineNumber != nil {
			lineNumber = strings.TrimSpace(*campaign.Spec.LineNumber)
		}
		if lineNumber == "" {
			return nil, ErrCampaignLineNumberRequired
		}
		if _, err := s.fetchLineNumberPriceFactor(sendCtx, &lineNumber); err != nil {
			return nil, err
		}
		client := scheduler.NewPayamSMSClient(s.payamSMSConfig)
		_, err := client.SendBatch(sendCtx, lineNumber, []scheduler.PayamSMSItem{{
			Recipient:  recipient,
			Body:       body,
			TrackingID: buildProviderTestID("test-sms", campaign.CustomerID),
		}})
		return err, nil

	case models.CampaignPlatformBale:
		settings, err := s.requireActivePlatformSettings(sendCtx, campaign, platform)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(s.baleConfig.APIAccessKey) == "" {
			return nil, fmt.Errorf("bale api-access-key is not configured")
		}
		botID, err := extractBaleBotIDFromSettings(settings)
		if err != nil {
			return nil, err
		}

		baleClient := scheduler.NewBaleClient(s.baleConfig)
		var fileID *string
		if campaign.Spec.MediaUUID != nil {
			path, err := s.resolveCampaignMediaPath(sendCtx, campaign)
			if err != nil {
				return nil, err
			}
			up, err := baleClient.UploadFile(sendCtx, path)
			if err != nil {
				return nil, err
			}
			if up != nil && strings.TrimSpace(up.FileID) != "" {
				v := strings.TrimSpace(up.FileID)
				fileID = &v
			}
		}

		_, err = baleClient.SendMessage(sendCtx, &scheduler.BaleSendMessageRequest{
			RequestID:   buildProviderTestID("test-bale", campaign.CustomerID),
			BotID:       botID,
			PhoneNumber: recipient,
			MessageData: scheduler.BaleSendMessageData{
				Message: &scheduler.BaleMessage{
					Text:   body,
					FileID: fileID,
				},
			},
		})
		return err, nil

	case models.CampaignPlatformRubika:
		settings, err := s.requireActivePlatformSettings(sendCtx, campaign, platform)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(s.rubikaConfig.Token) == "" {
			return nil, fmt.Errorf("rubika token is not configured")
		}
		serviceID, err := extractRubikaServiceIDFromSettings(settings)
		if err != nil {
			return nil, err
		}

		rubikaClient := scheduler.NewRubikaClient(s.rubikaConfig)
		var fileID *string
		if campaign.Spec.MediaUUID != nil {
			path, err := s.resolveCampaignMediaPath(sendCtx, campaign)
			if err != nil {
				return nil, err
			}

			up, err := rubikaClient.UploadFile(sendCtx, path)
			if err != nil {
				return nil, err
			}
			if up != nil && strings.TrimSpace(up.Data.FileID) != "" {
				v := strings.TrimSpace(up.Data.FileID)
				fileID = &v
			}
		}

		_, err = rubikaClient.SendBulkMessages(sendCtx, serviceID, []scheduler.RubikaMessagePayload{{
			Phone:  recipient,
			Text:   body,
			FileID: fileID,
		}})
		return err, nil

	case models.CampaignPlatformSPlus:
		settings, err := s.requireActivePlatformSettings(sendCtx, campaign, platform)
		if err != nil {
			return nil, err
		}
		botID, err := extractSplusBotIDFromSettings(settings)
		if err != nil {
			return nil, err
		}

		splusClient := scheduler.NewSplusClient(s.splusConfig)
		var fileID *string
		if campaign.Spec.MediaUUID != nil {
			path, err := s.resolveCampaignMediaPath(sendCtx, campaign)
			if err != nil {
				return nil, err
			}

			up, err := splusClient.UploadFile(sendCtx, botID, path)
			if err != nil {
				return nil, err
			}
			if up != nil && strings.TrimSpace(up.FileID) != "" {
				v := strings.TrimSpace(up.FileID)
				fileID = &v
			}
		}

		_, err = splusClient.SendMessage(sendCtx, botID, &scheduler.SplusSendMessageRequest{
			PhoneNumber: recipient,
			Text:        body,
			FileID:      fileID,
		})
		return err, nil

	default:
		return nil, ErrCampaignPlatformInvalid
	}
}

func (s *CampaignFlowImpl) resolveCampaignMediaPath(ctx context.Context, campaign models.Campaign) (string, error) {
	if campaign.Spec.MediaUUID == nil {
		return "", ErrCampaignMediaNotFound
	}

	asset, err := s.multimediaRepo.ByUUID(ctx, campaign.Spec.MediaUUID.String())
	if err != nil {
		return "", err
	}
	if asset == nil || asset.CustomerID != campaign.CustomerID {
		return "", ErrCampaignMediaNotFound
	}

	path, err := sanitizeMultimediaPath(asset.StoredPath)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", ErrCampaignMediaNotFound
		}
		return "", err
	}
	return filepath.Clean(path), nil
}

func (s *CampaignFlowImpl) requireActivePlatformSettings(ctx context.Context, campaign models.Campaign, platform string) (*models.PlatformSettings, error) {
	if campaign.Spec.PlatformSettingsID == nil || *campaign.Spec.PlatformSettingsID == 0 {
		return nil, ErrCampaignPlatformSettingRequired
	}
	rows, err := s.platformSettingsRepo.ByFilter(ctx, models.PlatformSettingsFilter{
		ID:         campaign.Spec.PlatformSettingsID,
		CustomerID: &campaign.CustomerID,
		Platform:   &platform,
		Status:     utils.ToPtr(models.PlatformSettingsStatusActive),
	}, "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || rows[0] == nil {
		return nil, ErrCampaignTestPlatformSettingsInvalid
	}
	return rows[0], nil
}

func extractBaleBotIDFromSettings(settings *models.PlatformSettings) (int64, error) {
	if settings == nil || settings.Metadata == nil {
		return 0, fmt.Errorf("%w: missing metadata", ErrCampaignTestPlatformSettingsInvalid)
	}
	raw, ok := settings.Metadata["bale_bot_id"]
	if !ok {
		return 0, fmt.Errorf("%w: bale_bot_id is missing", ErrCampaignTestPlatformSettingsInvalid)
	}

	switch v := raw.(type) {
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("%w: bale_bot_id must be positive", ErrCampaignTestPlatformSettingsInvalid)
		}
		return int64(v), nil
	case int64:
		if v <= 0 {
			return 0, fmt.Errorf("%w: bale_bot_id must be positive", ErrCampaignTestPlatformSettingsInvalid)
		}
		return v, nil
	case float64:
		if v <= 0 || v != float64(int64(v)) {
			return 0, fmt.Errorf("%w: bale_bot_id must be positive integer", ErrCampaignTestPlatformSettingsInvalid)
		}
		return int64(v), nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, fmt.Errorf("%w: bale_bot_id must not be empty", ErrCampaignTestPlatformSettingsInvalid)
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("%w: bale_bot_id must be positive integer", ErrCampaignTestPlatformSettingsInvalid)
		}
		return id, nil
	case json.Number:
		id, err := v.Int64()
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("%w: bale_bot_id must be positive integer", ErrCampaignTestPlatformSettingsInvalid)
		}
		return id, nil
	default:
		return 0, fmt.Errorf("%w: bale_bot_id has unsupported type %T", ErrCampaignTestPlatformSettingsInvalid, raw)
	}
}

func extractSplusBotIDFromSettings(settings *models.PlatformSettings) (string, error) {
	if settings == nil || settings.Metadata == nil {
		return "", fmt.Errorf("%w: missing metadata", ErrCampaignTestPlatformSettingsInvalid)
	}
	raw, ok := settings.Metadata["splus_bot_id"]
	if !ok {
		return "", fmt.Errorf("%w: splus_bot_id is missing", ErrCampaignTestPlatformSettingsInvalid)
	}

	switch v := raw.(type) {
	case string:
		id := strings.TrimSpace(v)
		if id == "" {
			return "", fmt.Errorf("%w: splus_bot_id must not be empty", ErrCampaignTestPlatformSettingsInvalid)
		}
		return id, nil
	default:
		return "", fmt.Errorf("%w: splus_bot_id has unsupported type %T", ErrCampaignTestPlatformSettingsInvalid, raw)
	}
}

func extractRubikaServiceIDFromSettings(settings *models.PlatformSettings) (string, error) {
	if settings == nil || settings.Metadata == nil {
		return "", fmt.Errorf("%w: missing metadata", ErrCampaignTestPlatformSettingsInvalid)
	}
	raw, ok := settings.Metadata["rubika_service_id"]
	if !ok {
		return "", fmt.Errorf("%w: rubika_service_id is missing", ErrCampaignTestPlatformSettingsInvalid)
	}

	switch v := raw.(type) {
	case string:
		id := strings.TrimSpace(v)
		if id == "" {
			return "", fmt.Errorf("%w: rubika_service_id must not be empty", ErrCampaignTestPlatformSettingsInvalid)
		}
		return id, nil
	case int:
		if v <= 0 {
			return "", fmt.Errorf("%w: rubika_service_id must be positive", ErrCampaignTestPlatformSettingsInvalid)
		}
		return strconv.Itoa(v), nil
	case int64:
		if v <= 0 {
			return "", fmt.Errorf("%w: rubika_service_id must be positive", ErrCampaignTestPlatformSettingsInvalid)
		}
		return strconv.FormatInt(v, 10), nil
	case float64:
		if v <= 0 {
			return "", fmt.Errorf("%w: rubika_service_id must be positive", ErrCampaignTestPlatformSettingsInvalid)
		}
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case json.Number:
		s := strings.TrimSpace(v.String())
		if s == "" {
			return "", fmt.Errorf("%w: rubika_service_id must not be empty", ErrCampaignTestPlatformSettingsInvalid)
		}
		return s, nil
	default:
		return "", fmt.Errorf("%w: rubika_service_id has unsupported type %T", ErrCampaignTestPlatformSettingsInvalid, raw)
	}
}

func maskPhoneNumber(phone string) string {
	trimmed := strings.TrimSpace(phone)
	if len(trimmed) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-4:]
}

func buildProviderTestID(prefix string, customerID uint) string {
	raw := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(raw) > 8 {
		raw = raw[:8]
	}
	return fmt.Sprintf("%s-%d-%s", prefix, customerID, raw)
}
