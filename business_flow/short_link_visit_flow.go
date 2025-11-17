package businessflow

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// ShortLinkVisitFlow resolves a short link and tracks a click
// Returns the target URL to redirect
// Also updates last known user-agent and IP if provided
// Public flow, no authentication required
type ShortLinkVisitFlow interface {
	Visit(ctx context.Context, uid string, userAgent *string, ip *string) (string, error)
}

type ShortLinkVisitFlowImpl struct {
	repo      repository.ShortLinkRepository
	clickRepo repository.ShortLinkClickRepository
}

func NewShortLinkVisitFlow(repo repository.ShortLinkRepository, clickRepo repository.ShortLinkClickRepository) ShortLinkVisitFlow {
	return &ShortLinkVisitFlowImpl{repo: repo, clickRepo: clickRepo}
}

func (f *ShortLinkVisitFlowImpl) Visit(ctx context.Context, uid string, userAgent *string, ip *string) (string, error) {
	row, err := f.repo.ByUID(ctx, uid)
	if err != nil {
		return "", NewBusinessError("SHORT_LINK_LOOKUP_FAILED", "Failed to lookup short link", err)
	}
	if row == nil {
		return "", ErrShortLinkNotFound
	}
	// Insert click row
	if err := f.clickRepo.Save(ctx, &models.ShortLinkClick{
		ShortLinkID: row.ID,
		ScenarioID:  row.ScenarioID,
		UserAgent:   userAgent,
		IP:          ip,
	}); err != nil {
		return "", NewBusinessError("SHORT_LINK_TRACK_FAILED", "Failed to track short link click", err)
	}
	return row.LongLink, nil
}
