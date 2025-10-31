package businessflow

import (
	"context"

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
	repo repository.ShortLinkRepository
}

func NewShortLinkVisitFlow(repo repository.ShortLinkRepository) ShortLinkVisitFlow {
	return &ShortLinkVisitFlowImpl{repo: repo}
}

func (f *ShortLinkVisitFlowImpl) Visit(ctx context.Context, uid string, userAgent *string, ip *string) (string, error) {
	row, err := f.repo.ByUID(ctx, uid)
	if err != nil {
		return "", NewBusinessError("SHORT_LINK_LOOKUP_FAILED", "Failed to lookup short link", err)
	}
	if row == nil {
		return "", ErrShortLinkNotFound
	}
	if err := f.repo.IncrementClicksByUID(ctx, uid, userAgent, ip); err != nil {
		return "", NewBusinessError("SHORT_LINK_TRACK_FAILED", "Failed to track short link click", err)
	}
	return row.Link, nil
}
