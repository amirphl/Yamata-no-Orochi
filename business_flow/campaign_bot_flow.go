package businessflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
	ListReadyCampaigns(ctx context.Context) (*dto.BotListCampaignsResponse, error)
	MoveCampaignToRunning(ctx context.Context, campaignID uint) error
	MoveCampaignToExecuted(ctx context.Context, campaignID uint) error
	UpdateAudienceSpec(ctx context.Context, req *dto.BotUpdateAudienceSpecRequest) (*dto.BotUpdateAudienceSpecResponse, error)
	ResetAudienceSpec(ctx context.Context, req *dto.BotResetAudienceSpecRequest) (*dto.BotResetAudienceSpecResponse, error)
}

type BotCampaignFlowImpl struct {
	campaignRepo repository.CampaignRepository
	cacheConfig  *config.CacheConfig
	db           *gorm.DB
	rc           *redis.Client
}

func NewBotCampaignFlow(
	campaignRepo repository.CampaignRepository,
	cacheConfig *config.CacheConfig,
	db *gorm.DB,
	rc *redis.Client,
) BotCampaignFlow {
	return &BotCampaignFlowImpl{
		campaignRepo: campaignRepo,
		cacheConfig:  cacheConfig,
		db:           db,
		rc:           rc,
	}
}

// ListReadyCampaigns retrieves ready campaigns for bot
func (s *BotCampaignFlowImpl) ListReadyCampaigns(ctx context.Context) (*dto.BotListCampaignsResponse, error) {
	cf := models.CampaignFilter{
		Status:         utils.ToPtr(models.CampaignStatusApproved),
		ScheduleBefore: utils.ToPtr(utils.UTCNow()),
		ScheduleAfter:  utils.ToPtr(utils.UTCNow().Add(-1 * time.Hour)),
	}

	readyCampaigns, err := s.campaignRepo.ByFilter(ctx, cf, "created_at DESC", 0, 0)
	if err != nil {
		return nil, NewBusinessError("BOT_LIST_READY_CAMPAIGNS_FAILED", "Failed to list ready campaigns", err)
	}

	items := make([]dto.BotGetCampaignResponse, 0, len(readyCampaigns))
	for _, c := range readyCampaigns {
		items = append(items, dto.BotGetCampaignResponse{
			ID:           c.ID,
			Status:       c.Status.String(),
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
			Title:        c.Spec.Title,
			Level1:       c.Spec.Level1,
			Level2s:      c.Spec.Level2s,
			Tags:         c.Spec.Tags,
			Sex:          c.Spec.Sex,
			City:         c.Spec.City,
			AdLink:       c.Spec.AdLink,
			Content:      c.Spec.Content,
			ScheduleAt:   c.Spec.ScheduleAt,
			LineNumber:   c.Spec.LineNumber,
			Budget:       c.Spec.Budget,
			Comment:      c.Comment,
			NumAudiences: *c.NumAudience,
		})
	}

	return &dto.BotListCampaignsResponse{
		Message: "Ready campaigns retrieved successfully",
		Items:   items,
	}, nil
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

type AudienceSpecLeaf struct {
	Tags              []string `json:"tags"`
	AvailableAudience int      `json:"available_audience"`
}

type AudienceSpecMap map[string]map[string]map[string]AudienceSpecLeaf

// v2 on-disk format structures (Level2 holds metadata and items)
type audienceSpecLevel2File struct {
	Metadata map[string]any              `json:"metadata,omitempty"`
	Items    map[string]AudienceSpecLeaf `json:"items,omitempty"`
}

type audienceSpecFileV2 map[string]map[string]*audienceSpecLevel2File

func (s *BotCampaignFlowImpl) UpdateAudienceSpec(ctx context.Context, req *dto.BotUpdateAudienceSpecRequest) (*dto.BotUpdateAudienceSpecResponse, error) {
	lockKey := redisKey(*s.cacheConfig, utils.AudienceSpecLockKey)
	cacheKey := redisKey(*s.cacheConfig, utils.AudienceSpecCacheKey)
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

	// Read existing JSON file (if any) in v2 format
	current, err := readAudienceSpecFileV2(filePath)
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
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
	bytes, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal merged spec", err)
	}
	if err := atomicWrite(filePath, bytes, 0o644); err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_WRITE_FAILED", "Failed to write audience spec file", err)
	}

	// Update Redis cache
	if err := s.rc.Set(ctx, cacheKey, bytes, 0).Err(); err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_CACHE_FAILED", "Failed to cache audience spec", err)
	}

	return &dto.BotUpdateAudienceSpecResponse{Message: "Audience spec updated"}, nil
}

// ResetAudienceSpec deletes the specified level1/level2/level3 from the audience spec
func (s *BotCampaignFlowImpl) ResetAudienceSpec(ctx context.Context, req *dto.BotResetAudienceSpecRequest) (*dto.BotResetAudienceSpecResponse, error) {
	lockKey := redisKey(*s.cacheConfig, utils.AudienceSpecLockKey)
	cacheKey := redisKey(*s.cacheConfig, utils.AudienceSpecCacheKey)
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
	current, err := readAudienceSpecFileV2(filePath)
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
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
	bytes, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal updated spec", err)
	}
	if err := atomicWrite(filePath, bytes, 0o644); err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_WRITE_FAILED", "Failed to write audience spec file", err)
	}

	// Update Redis cache
	if err := s.rc.Set(ctx, cacheKey, bytes, 0).Err(); err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_CACHE_FAILED", "Failed to cache audience spec", err)
	}

	return &dto.BotResetAudienceSpecResponse{Message: "Audience spec reset successfully"}, nil
}

// readAudienceSpecFileV2 reads the on-disk spec and upgrades legacy format to v2 in-memory
func readAudienceSpecFileV2(path string) (audienceSpecFileV2, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(audienceSpecFileV2), nil
		}
		return nil, err
	}
	if len(bytes) == 0 {
		return make(audienceSpecFileV2), nil
	}
	// Try v2
	var v2 audienceSpecFileV2
	if err := json.Unmarshal(bytes, &v2); err == nil && v2 != nil {
		// Ensure inner maps are non-nil
		for l1, l2map := range v2 {
			if l2map == nil {
				v2[l1] = make(map[string]*audienceSpecLevel2File)
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
		return v2, nil
	}
	// Legacy format: map[level1][level2][level3]leaf
	var legacy AudienceSpecMap
	if err := json.Unmarshal(bytes, &legacy); err != nil || legacy == nil {
		// If unmarshal failed, return empty v2 but not an error to avoid breaking
		return make(audienceSpecFileV2), nil
	}
	upgraded := make(audienceSpecFileV2)
	for l1, l2 := range legacy {
		if _, ok := upgraded[l1]; !ok {
			upgraded[l1] = make(map[string]*audienceSpecLevel2File)
		}
		for l2k, l3 := range l2 {
			node := &audienceSpecLevel2File{Metadata: map[string]any{}, Items: map[string]AudienceSpecLeaf{}}
			for l3k, leaf := range l3 {
				node.Items[l3k] = leaf
			}
			upgraded[l1][l2k] = node
		}
	}
	return upgraded, nil
}
