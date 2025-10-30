package businessflow

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"gorm.io/gorm"
)

// BotShortLinkFlow handles creation of short links by bot
// Validates inputs and persists records in a transaction for batch
// UID must be unique and provided by the caller
// Clicks initialize to 0
// CampaignID is optional (no FK)
type BotShortLinkFlow interface {
	CreateShortLink(ctx context.Context, req *dto.BotCreateShortLinkRequest) (*dto.BotCreateShortLinkResponse, error)
	CreateShortLinks(ctx context.Context, req *dto.BotCreateShortLinksRequest) (*dto.BotCreateShortLinksResponse, error)
}

type BotShortLinkFlowImpl struct {
	shortRepo repository.ShortLinkRepository
	db        *gorm.DB
}

func NewBotShortLinkFlow(shortRepo repository.ShortLinkRepository, db *gorm.DB) BotShortLinkFlow {
	return &BotShortLinkFlowImpl{shortRepo: shortRepo, db: db}
}

func (s *BotShortLinkFlowImpl) CreateShortLink(ctx context.Context, req *dto.BotCreateShortLinkRequest) (*dto.BotCreateShortLinkResponse, error) {
	if req == nil {
		return nil, NewBusinessError("VALIDATION_ERROR", "Request body is required", nil)
	}
	if req.UID == "" || req.PhoneNumber == "" || req.Link == "" {
		return nil, NewBusinessError("VALIDATION_ERROR", "uid, phone_number and link are required", nil)
	}

	row := &models.ShortLink{
		UID:         req.UID,
		CampaignID:  req.CampaignID,
		PhoneNumber: req.PhoneNumber,
		Clicks:      0,
		Link:        req.Link,
		UserAgent:   req.UserAgent,
		IP:          req.IP,
	}
	if err := s.shortRepo.Save(ctx, row); err != nil {
		return nil, NewBusinessError("BOT_CREATE_SHORT_LINK_FAILED", "Failed to create short link", err)
	}
	return &dto.BotCreateShortLinkResponse{
		Message: "Short link created",
		Item:    mapShortLinkDTO(row),
	}, nil
}

func (s *BotShortLinkFlowImpl) CreateShortLinks(ctx context.Context, req *dto.BotCreateShortLinksRequest) (*dto.BotCreateShortLinksResponse, error) {
	if req == nil || len(req.Items) == 0 {
		return nil, NewBusinessError("VALIDATION_ERROR", "items must contain at least one element", nil)
	}
	rows := make([]*models.ShortLink, 0, len(req.Items))
	for _, it := range req.Items {
		if it.UID == "" || it.PhoneNumber == "" || it.Link == "" {
			return nil, NewBusinessError("VALIDATION_ERROR", "uid, phone_number and link are required for all items", nil)
		}
		rows = append(rows, &models.ShortLink{
			UID:         it.UID,
			CampaignID:  it.CampaignID,
			PhoneNumber: it.PhoneNumber,
			Clicks:      0,
			Link:        it.Link,
			UserAgent:   it.UserAgent,
			IP:          it.IP,
		})
	}

	// Persist in a single transaction for consistency
	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		return s.shortRepo.SaveBatch(txCtx, rows)
	}); err != nil {
		return nil, NewBusinessError("BOT_CREATE_SHORT_LINKS_FAILED", "Failed to create short links", err)
	}

	out := make([]dto.ShortLinkDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapShortLinkDTO(r))
	}
	return &dto.BotCreateShortLinksResponse{
		Message: "Short links created",
		Items:   out,
	}, nil
}

func mapShortLinkDTO(m *models.ShortLink) dto.ShortLinkDTO {
	return dto.ShortLinkDTO{
		ID:          m.ID,
		UID:         m.UID,
		CampaignID:  m.CampaignID,
		PhoneNumber: m.PhoneNumber,
		Clicks:      m.Clicks,
		Link:        m.Link,
		UserAgent:   m.UserAgent,
		IP:          m.IP,
	}
}
