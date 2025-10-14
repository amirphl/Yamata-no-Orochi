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
			Segment:      c.Spec.Segment,
			Subsegment:   c.Spec.Subsegment,
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

type AudienceSpecMap map[string]map[string]AudienceSpecLeaf

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

	// Read existing JSON file (if any)
	current, err := readAudienceSpecFile(filePath)
	if err != nil {
		return nil, NewBusinessError("BOT_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
	}

	// Merge
	if _, exists := current[req.Segment]; !exists {
		current[req.Segment] = make(map[string]AudienceSpecLeaf)
	}
	current[req.Segment][req.Subsegment] = AudienceSpecLeaf{
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

func readAudienceSpecFile(path string) (AudienceSpecMap, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(AudienceSpecMap), nil
		}
		return nil, err
	}
	var out AudienceSpecMap
	if len(bytes) == 0 {
		return make(AudienceSpecMap), nil
	}
	if err := json.Unmarshal(bytes, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = make(AudienceSpecMap)
	}
	return out, nil
}
