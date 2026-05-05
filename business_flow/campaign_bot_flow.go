package businessflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// BotCampaignFlow handles campaign listing logic accessible to bots
type BotCampaignFlow interface {
	ListReadyCampaigns(ctx context.Context, platform *string) (*dto.BotListCampaignsResponse, error)
	MoveCampaignToRunning(ctx context.Context, campaignID uint) error
	MoveCampaignToExecuted(ctx context.Context, campaignID uint) error
	UpdateAudienceSpec(ctx context.Context, req *dto.BotUpdateAudienceSpecRequest) (*dto.BotUpdateAudienceSpecResponse, error)
	ResetAudienceSpec(ctx context.Context, req *dto.BotResetAudienceSpecRequest) (*dto.BotResetAudienceSpecResponse, error)
	UpdateCampaignStatistics(ctx context.Context, campaignID uint, statistics map[string]any) (*dto.BotUpdateCampaignStatisticsResponse, error)
}

type BotCampaignFlowImpl struct {
	campaignRepo         repository.CampaignRepository
	platformSettingsRepo repository.PlatformSettingsRepository
	cacheConfig          config.CacheConfig
	db                   *gorm.DB
	rc                   *redis.Client
}

func NewBotCampaignFlow(
	campaignRepo repository.CampaignRepository,
	platformSettingsRepo repository.PlatformSettingsRepository,
	cacheConfig config.CacheConfig,
	db *gorm.DB,
	rc *redis.Client,
) BotCampaignFlow {
	return &BotCampaignFlowImpl{
		campaignRepo:         campaignRepo,
		platformSettingsRepo: platformSettingsRepo,
		cacheConfig:          cacheConfig,
		db:                   db,
		rc:                   rc,
	}
}

// ListReadyCampaigns retrieves ready campaigns for bot
func (s *BotCampaignFlowImpl) ListReadyCampaigns(ctx context.Context, platform *string) (*dto.BotListCampaignsResponse, error) {
	cf := models.CampaignFilter{
		Status:         utils.ToPtr(models.CampaignStatusApproved),
		ScheduleBefore: utils.ToPtr(utils.UTCNow()),
		ScheduleAfter:  utils.ToPtr(utils.UTCNow().Add(-1 * time.Hour)),
	}
	if platform != nil {
		p := strings.ToLower(strings.TrimSpace(*platform))
		if p != "" {
			if !models.IsValidCampaignPlatform(p) {
				return nil, NewBusinessError("BOT_LIST_READY_CAMPAIGNS_FAILED", "Failed to list ready campaigns", ErrCampaignPlatformInvalid)
			}
			cf.Platform = &p
		}
	}

	readyCampaigns, err := s.campaignRepo.ByFilter(ctx, cf, "created_at DESC", 0, 0)
	if err != nil {
		return nil, NewBusinessError("BOT_LIST_READY_CAMPAIGNS_FAILED", "Failed to list ready campaigns", err)
	}

	platformSettingsByID, err := s.loadPlatformSettingsSpecs(ctx, readyCampaigns)
	if err != nil {
		return nil, err
	}

	items := make([]dto.BotGetCampaignResponse, 0, len(readyCampaigns))
	for _, c := range readyCampaigns {
		var platformSettings *dto.BotCampaignPlatformSettingsSpec
		if c.Spec.PlatformSettingsID != nil && *c.Spec.PlatformSettingsID != 0 {
			platformSettings = platformSettingsByID[*c.Spec.PlatformSettingsID]
		}

		items = append(items, dto.BotGetCampaignResponse{
			ID:                 c.ID,
			CustomerID:         c.CustomerID,
			Status:             c.Status.String(),
			CreatedAt:          c.CreatedAt,
			UpdatedAt:          c.UpdatedAt,
			Title:              c.Spec.Title,
			Level1:             c.Spec.Level1,
			Level2s:            c.Spec.Level2s,
			Tags:               c.Spec.Tags,
			Sex:                c.Spec.Sex,
			City:               c.Spec.City,
			AdLink:             c.Spec.AdLink,
			Content:            c.Spec.Content,
			ShortLinkDomain:    c.Spec.ShortLinkDomain,
			Category:           c.Spec.Category,
			Job:                c.Spec.Job,
			ScheduleAt:         c.Spec.ScheduleAt,
			LineNumber:         c.Spec.LineNumber,
			MediaUUID:          c.Spec.MediaUUID,
			PlatformSettingsID: c.Spec.PlatformSettingsID,
			PlatformSettings:   platformSettings,
			Platform:           c.Spec.Platform,
			Budget:             c.Spec.Budget,
			Comment:            c.Comment,
			NumAudiences:       c.NumAudience,
		})
	}

	return &dto.BotListCampaignsResponse{
		Message: "Ready campaigns retrieved successfully",
		Items:   items,
	}, nil
}

func (s *BotCampaignFlowImpl) loadPlatformSettingsSpecs(ctx context.Context, campaigns []*models.Campaign) (map[uint]*dto.BotCampaignPlatformSettingsSpec, error) {
	ids := make(map[uint]struct{})
	for _, campaign := range campaigns {
		if campaign.Spec.PlatformSettingsID == nil || *campaign.Spec.PlatformSettingsID == 0 {
			continue
		}
		ids[*campaign.Spec.PlatformSettingsID] = struct{}{}
	}
	if len(ids) == 0 {
		return map[uint]*dto.BotCampaignPlatformSettingsSpec{}, nil
	}

	result := make(map[uint]*dto.BotCampaignPlatformSettingsSpec, len(ids))
	for id := range ids {
		row, err := s.platformSettingsRepo.ByID(ctx, id)
		if err != nil {
			return nil, NewBusinessError("BOT_LIST_READY_CAMPAIGNS_FAILED", "Failed to fetch platform settings", err)
		}
		if row == nil {
			err := fmt.Errorf("platform settings not found: id=%d", id)
			return nil, NewBusinessError("BOT_LIST_READY_CAMPAIGNS_FAILED", "Campaign references missing platform settings", err)
		}
		result[id] = &dto.BotCampaignPlatformSettingsSpec{
			ID:           row.ID,
			Platform:     row.Platform,
			Name:         row.Name,
			Description:  row.Description,
			MultimediaID: row.MultimediaID,
			Metadata:     row.Metadata,
			Status:       string(row.Status),
		}
	}

	return result, nil
}

// MoveCampaignToRunning moves campaign status to running
func (s *BotCampaignFlowImpl) MoveCampaignToRunning(ctx context.Context, campaignID uint) error {
	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		campaign, err := s.campaignRepo.ByID(txCtx, campaignID)
		if err != nil {
			return err
		}
		if campaign == nil {
			return ErrCampaignNotFound
		}
		campaign.Status = models.CampaignStatusRunning
		err = s.campaignRepo.Update(txCtx, *campaign)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return NewBusinessError("BOT_MOVE_CAMPAIGN_TO_RUNNING_FAILED", "Failed to move campaign to running", err)
	}
	return nil
}

// MoveCampaignToExecuted moves campaign status to executed
func (s *BotCampaignFlowImpl) MoveCampaignToExecuted(ctx context.Context, campaignID uint) error {
	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		campaign, err := s.campaignRepo.ByID(txCtx, campaignID)
		if err != nil {
			return err
		}
		if campaign == nil {
			return ErrCampaignNotFound
		}
		campaign.Status = models.CampaignStatusExecuted
		err = s.campaignRepo.Update(txCtx, *campaign)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return NewBusinessError("BOT_MOVE_CAMPAIGN_TO_EXECUTED_FAILED", "Failed to move campaign to executed", err)
	}
	return nil
}

// UpdateCampaignStatistics updates the statistics JSON field of a campaign
func (s *BotCampaignFlowImpl) UpdateCampaignStatistics(ctx context.Context, campaignID uint, statistics map[string]any) (*dto.BotUpdateCampaignStatisticsResponse, error) {
	if campaignID == 0 {
		return nil, NewBusinessError("VALIDATION_ERROR", "campaign_id must be greater than 0", nil)
	}
	campaign, err := s.campaignRepo.ByID(ctx, campaignID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_FETCH_FAILED", "Failed to fetch campaign", err)
	}
	if campaign == nil {
		return nil, ErrCampaignNotFound
	}

	data, err := json.Marshal(statistics)
	if err != nil {
		return nil, NewBusinessError("STATISTICS_MARSHAL_FAILED", "Failed to marshal statistics", err)
	}

	if err := s.campaignRepo.UpdateStatistics(ctx, campaignID, data); err != nil {
		return nil, NewBusinessError("CAMPAIGN_STATISTICS_UPDATE_FAILED", "Failed to update campaign statistics", err)
	}

	return &dto.BotUpdateCampaignStatisticsResponse{Message: "Campaign statistics updated"}, nil
}

type AudienceSpecLeaf struct {
	Tags              []string `json:"tags"`
	AvailableAudience int      `json:"available_audience"`
}

// on-disk format structures (Level2 holds metadata and items)
type audienceSpecLevel2File struct {
	Metadata map[string]any              `json:"metadata,omitempty"`
	Items    map[string]AudienceSpecLeaf `json:"items,omitempty"`
}

type audienceSpecFile map[string]map[string]*audienceSpecLevel2File
type audienceSpecByPlatformFile map[string]audienceSpecFile

func audienceSpecPlatformCacheKey(cacheConfig config.CacheConfig, platform string) string {
	return redisKey(cacheConfig, fmt.Sprintf("%s:%s", utils.AudienceSpecCacheKey, platform))
}

func audienceSpecPlatformLockKey(cacheConfig config.CacheConfig, platform string) string {
	return redisKey(cacheConfig, fmt.Sprintf("%s:%s", utils.AudienceSpecLockKey, platform))
}

func normalizeAudienceSpecPlatformRequired(platform string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(platform))
	if p == "" {
		return "", ErrAudienceSpecPlatformRequired
	}
	if !models.IsValidCampaignPlatform(p) {
		return "", ErrAudienceSpecPlatformInvalid
	}
	return p, nil
}

func normalizeAudienceSpecPlatformDefault(platform *string) (string, error) {
	if platform == nil {
		return models.CampaignPlatformSMS, nil
	}
	p := strings.ToLower(strings.TrimSpace(*platform))
	if p == "" {
		return models.CampaignPlatformSMS, nil
	}
	if !models.IsValidCampaignPlatform(p) {
		return "", ErrAudienceSpecPlatformInvalid
	}
	return p, nil
}

func (s *BotCampaignFlowImpl) UpdateAudienceSpec(ctx context.Context, req *dto.BotUpdateAudienceSpecRequest) (*dto.BotUpdateAudienceSpecResponse, error) {
	platform, err := normalizeAudienceSpecPlatformRequired(req.Platform)
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_PLATFORM_REQUIRED", "Platform is required", err)
	}

	lockKey := audienceSpecPlatformLockKey(s.cacheConfig, platform)
	cacheKey := audienceSpecPlatformCacheKey(s.cacheConfig, platform)
	filePath := audienceSpecFilePath()

	// Acquire distributed lock (SETNX with TTL)
	ok, err := s.rc.SetNX(ctx, lockKey, "1", 10*time.Second).Result()
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_LOCK_FAILED", "Failed to acquire lock", err)
	}
	if !ok {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_LOCK_BUSY", "Another worker is updating audience spec", errors.New("lock busy"))
	}
	defer func() {
		_ = s.rc.Del(context.Background(), lockKey).Err()
	}()

	// Read existing JSON file (if any)
	currentByPlatform, err := readAudienceSpecFileByPlatform(filePath)
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
	}
	current, exists := currentByPlatform[platform]
	if !exists || current == nil {
		current = make(audienceSpecFile)
	}

	// Ensure maps
	if _, exists := current[req.Level1]; !exists {
		current[req.Level1] = make(map[string]*audienceSpecLevel2File)
	}
	if _, exists := current[req.Level1][req.Level2]; !exists {
		current[req.Level1][req.Level2] = &audienceSpecLevel2File{Metadata: map[string]any{}, Items: map[string]AudienceSpecLeaf{}}
	}
	lvl2 := current[req.Level1][req.Level2]
	if lvl2.Items == nil {
		lvl2.Items = make(map[string]AudienceSpecLeaf)
	}
	// Optionally set/merge metadata if provided
	if req.Metadata != nil {
		lvl2.Metadata = req.Metadata
	}
	// Upsert leaf
	lvl2.Items[req.Level3] = AudienceSpecLeaf{
		Tags:              req.Tags,
		AvailableAudience: req.AvailableAudience,
	}

	// Marshal and write atomically (tmp + rename)
	currentByPlatform[platform] = current
	bytes, err := json.MarshalIndent(currentByPlatform, "", "  ")
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal merged spec", err)
	}
	if err := atomicWrite(filePath, bytes, 0o644); err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_WRITE_FAILED", "Failed to write audience spec file", err)
	}

	// Update Redis cache
	platformBytes, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal platform spec", err)
	}
	if err := s.rc.Set(ctx, cacheKey, platformBytes, 0).Err(); err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_CACHE_FAILED", "Failed to cache audience spec", err)
	}

	return &dto.BotUpdateAudienceSpecResponse{Message: "Audience spec updated"}, nil
}

// ResetAudienceSpec deletes the specified level1/level2/level3 from the audience spec
func (s *BotCampaignFlowImpl) ResetAudienceSpec(ctx context.Context, req *dto.BotResetAudienceSpecRequest) (*dto.BotResetAudienceSpecResponse, error) {
	platform, err := normalizeAudienceSpecPlatformRequired(req.Platform)
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_PLATFORM_REQUIRED", "Platform is required", err)
	}

	lockKey := audienceSpecPlatformLockKey(s.cacheConfig, platform)
	cacheKey := audienceSpecPlatformCacheKey(s.cacheConfig, platform)
	filePath := audienceSpecFilePath()

	// Acquire distributed lock (SETNX with TTL)
	ok, err := s.rc.SetNX(ctx, lockKey, "1", 10*time.Second).Result()
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_LOCK_FAILED", "Failed to acquire lock", err)
	}
	if !ok {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_LOCK_BUSY", "Another worker is updating audience spec", errors.New("lock busy"))
	}
	defer func() {
		_ = s.rc.Del(context.Background(), lockKey).Err()
	}()

	// Read existing JSON file (if any)
	currentByPlatform, err := readAudienceSpecFileByPlatform(filePath)
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
	}
	current, exists := currentByPlatform[platform]
	if !exists || current == nil {
		return &dto.BotResetAudienceSpecResponse{Message: "Platform not found, nothing to reset"}, nil
	}

	// Check if level1 exists
	lvl2Map, ok := current[req.Level1]
	if !ok {
		return &dto.BotResetAudienceSpecResponse{Message: "Level1 not found, nothing to reset"}, nil
	}
	// Check if level2 exists
	lvl2, ok := lvl2Map[req.Level2]
	if !ok || lvl2 == nil {
		return &dto.BotResetAudienceSpecResponse{Message: "Level2 not found, nothing to reset"}, nil
	}
	// Check if level3 exists
	if _, ok := lvl2.Items[req.Level3]; !ok {
		return &dto.BotResetAudienceSpecResponse{Message: "Level3 not found, nothing to reset"}, nil
	}

	// Delete the level3 leaf
	delete(lvl2.Items, req.Level3)
	// If level3 map is now empty, delete level2 (metadata discarded)
	if len(lvl2.Items) == 0 {
		delete(lvl2Map, req.Level2)
	}
	// If level2 map is now empty, delete level1
	if len(lvl2Map) == 0 {
		delete(current, req.Level1)
	}

	// Marshal and write atomically (tmp + rename)
	if len(current) == 0 {
		delete(currentByPlatform, platform)
	} else {
		currentByPlatform[platform] = current
	}

	bytes, err := json.MarshalIndent(currentByPlatform, "", "  ")
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal updated spec", err)
	}
	if err := atomicWrite(filePath, bytes, 0o644); err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_WRITE_FAILED", "Failed to write audience spec file", err)
	}

	// Update Redis cache
	if len(current) == 0 {
		if err := s.rc.Del(ctx, cacheKey).Err(); err != nil {
			return nil, NewBusinessError("BOT_AUDIENCE_SPEC_CACHE_FAILED", "Failed to clear platform cache", err)
		}
	} else {
		platformBytes, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			return nil, NewBusinessError("BOT_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal platform spec", err)
		}
		if err := s.rc.Set(ctx, cacheKey, platformBytes, 0).Err(); err != nil {
			return nil, NewBusinessError("BOT_AUDIENCE_SPEC_CACHE_FAILED", "Failed to cache audience spec", err)
		}
	}

	return &dto.BotResetAudienceSpecResponse{Message: "Audience spec reset successfully"}, nil
}

func readAudienceSpecFileByPlatform(path string) (audienceSpecByPlatformFile, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(audienceSpecByPlatformFile), nil
		}
		return nil, err
	}
	if len(bytes) == 0 {
		return make(audienceSpecByPlatformFile), nil
	}

	// Current format: map[platform]audienceSpecFile
	var byPlatform audienceSpecByPlatformFile
	if err := json.Unmarshal(bytes, &byPlatform); err == nil && byPlatform != nil {
		for platform, spec := range byPlatform {
			normalized, nerr := normalizeAudienceSpecPlatformRequired(platform)
			if nerr != nil {
				continue
			}
			byPlatform[normalized] = ensureAudienceSpecFile(spec)
			if normalized != platform {
				delete(byPlatform, platform)
			}
		}
		return byPlatform, nil
	}
	return make(audienceSpecByPlatformFile), nil
}

func ensureAudienceSpecFile(in audienceSpecFile) audienceSpecFile {
	if in == nil {
		return make(audienceSpecFile)
	}
	for l1, l2map := range in {
		if l2map == nil {
			in[l1] = make(map[string]*audienceSpecLevel2File)
			continue
		}
		for l2, node := range l2map {
			if node == nil {
				l2map[l2] = &audienceSpecLevel2File{Metadata: map[string]any{}, Items: map[string]AudienceSpecLeaf{}}
				continue
			}
			if node.Items == nil {
				node.Items = make(map[string]AudienceSpecLeaf)
			}
			if node.Metadata == nil {
				node.Metadata = make(map[string]any)
			}
		}
	}
	return in
}
