package businessflow

import (
	"context"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

// BotCampaignFlow handles campaign listing logic accessible to bots
type BotCampaignFlow interface {
	ListReadyCampaigns(ctx context.Context) (*dto.BotListCampaignsResponse, error)
	MoveCampaignToExecuted(ctx context.Context, campaignID uint) error
}

type BotCampaignFlowImpl struct {
	db           *gorm.DB
	campaignRepo repository.CampaignRepository
}

func NewBotCampaignFlow(db *gorm.DB, campaignRepo repository.CampaignRepository) BotCampaignFlow {
	return &BotCampaignFlowImpl{db: db, campaignRepo: campaignRepo}
}

// ListReadyCampaigns retrieves ready campaigns for bot
func (s *BotCampaignFlowImpl) ListReadyCampaigns(ctx context.Context) (*dto.BotListCampaignsResponse, error) {
	cf := models.CampaignFilter{
		Status:         utils.ToPtr(models.CampaignStatusApproved),
		ScheduleBefore: utils.ToPtr(utils.UTCNow()),
		ScheduleAfter:  utils.ToPtr(utils.UTCNow().Add(-1 * time.Hour)),
	}

	var readyCampaigns []*models.Campaign
	var err error

	// make all status to running in with transsction block
	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		readyCampaigns, err = s.campaignRepo.ByFilter(ctx, cf, "created_at DESC", 0, 0)
		if err != nil {
			return err
		}

		for _, readyCampaign := range readyCampaigns {
			readyCampaign.Status = models.CampaignStatusRunning
			err = s.campaignRepo.Update(txCtx, *readyCampaign)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, NewBusinessError("BOT_LIST_READY_CAMPAIGNS_FAILED", "Failed to list ready campaigns", err)
	}

	items := make([]dto.BotGetCampaignResponse, 0, len(readyCampaigns))
	for _, c := range readyCampaigns {
		items = append(items, dto.BotGetCampaignResponse{
			ID:         c.ID,
			Status:     c.Status.String(),
			CreatedAt:  c.CreatedAt,
			UpdatedAt:  c.UpdatedAt,
			Title:      c.Spec.Title,
			Segment:    c.Spec.Segment,
			Subsegment: c.Spec.Subsegment,
			Tags:       c.Spec.Tags,
			Sex:        c.Spec.Sex,
			City:       c.Spec.City,
			AdLink:     c.Spec.AdLink,
			Content:    c.Spec.Content,
			ScheduleAt: c.Spec.ScheduleAt,
			LineNumber: c.Spec.LineNumber,
			Budget:     c.Spec.Budget,
			Comment:    c.Comment,
		})
	}

	return &dto.BotListCampaignsResponse{Message: "Ready campaigns retrieved successfully", Items: items}, nil
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
