package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/config"
)

const defaultBotAPIDomain = "https://jazebeh.ir"

type BotClient interface {
	Login(ctx context.Context) (string, error)
	ListReadyCampaigns(ctx context.Context, token string) ([]dto.BotGetCampaignResponse, error)
	MoveCampaignToRunning(ctx context.Context, token string, id uint) error
	MoveCampaignToExecuted(ctx context.Context, token string, id uint) error
	AllocateShortLinks(ctx context.Context, token string, campaignID uint, adLink *string, phones []string) ([]string, error)
	PushCampaignStatistics(ctx context.Context, processedCampaignID uint, stats map[string]any) error
	CreateShortLinks(ctx context.Context, token string, reqBody *dto.BotCreateShortLinksRequest) error
}

type httpBotClient struct {
	cfg    config.BotConfig
	client *http.Client
}

func newHTTPBotClient(cfg config.BotConfig) *httpBotClient {
	if cfg.APIDomain == "" {
		cfg.APIDomain = defaultBotAPIDomain
	}
	return &httpBotClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *httpBotClient) Login(ctx context.Context) (string, error) {
	if c.cfg.Username == "" || c.cfg.Password == "" {
		return "", fmt.Errorf("bot credentials not configured")
	}
	url := c.cfg.APIDomain + "/api/v1/bot/auth/login"
	reqBody := dto.BotLoginRequest{
		Username: c.cfg.Username,
		Password: c.cfg.Password,
	}
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
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

	var apiResp dto.APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to decode JSON into APIResponse: %w", err)
	}

	if !apiResp.Success {
		return "", fmt.Errorf("bot login failed: %v", apiResp.Message)
	}

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

func (c *httpBotClient) ListReadyCampaigns(ctx context.Context, token string) ([]dto.BotGetCampaignResponse, error) {
	url := c.cfg.APIDomain + "/api/v1/bot/campaigns/ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
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

	if !apiResp.Success {
		return nil, fmt.Errorf("list ready campaigns failed: %v", apiResp.Message)
	}

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

func (c *httpBotClient) MoveCampaignToRunning(ctx context.Context, token string, id uint) error {
	url := c.cfg.APIDomain + "/api/v1/bot/campaigns/" + strconv.FormatUint(uint64(id), 10) + "/running"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("move to running http status: %d", resp.StatusCode)
	}
	return nil
}

func (c *httpBotClient) MoveCampaignToExecuted(ctx context.Context, token string, id uint) error {
	url := c.cfg.APIDomain + "/api/v1/bot/campaigns/" + strconv.FormatUint(uint64(id), 10) + "/executed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("move to executed http status: %d", resp.StatusCode)
	}
	return nil
}

func (c *httpBotClient) AllocateShortLinks(ctx context.Context, token string, campaignID uint, adLink *string, phones []string) ([]string, error) {
	payload := dto.BotGenerateShortLinksRequest{
		CampaignID:      campaignID,
		AdLink:          adLink,
		Phones:          phones,
		ShortLinkDomain: "https://jo1n.ir/",
	}
	b, _ := json.Marshal(payload)
	url := c.cfg.APIDomain + "/api/v1/bot/short-links/allocate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
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

func (c *httpBotClient) PushCampaignStatistics(ctx context.Context, campaignID uint, stats map[string]any) error {
	token, err := c.Login(ctx)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/v1/bot/campaigns/%d/statistics", c.cfg.APIDomain, campaignID)
	payload, _ := json.Marshal(dto.BotUpdateCampaignStatisticsRequest{Statistics: stats})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("push statistics http status: %d", resp.StatusCode)
	}
	return nil
}

func (c *httpBotClient) CreateShortLinks(ctx context.Context, token string, reqBody *dto.BotCreateShortLinksRequest) error {
	url := c.cfg.APIDomain + "/api/v1/bot/short-links"
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
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
