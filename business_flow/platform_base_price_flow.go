package businessflow

import (
	"context"
	"sort"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// PlatformBasePriceFlow defines user-facing operations for platform base prices.
type PlatformBasePriceFlow interface {
	ListPlatformBasePrices(ctx context.Context) (*dto.ListPlatformBasePricesResponse, error)
}

// PlatformBasePriceFlowImpl implements PlatformBasePriceFlow.
type PlatformBasePriceFlowImpl struct {
	platformBasePriceRepo repository.PlatformBasePriceRepository
}

// NewPlatformBasePriceFlow creates a new platform base price flow.
func NewPlatformBasePriceFlow(platformBasePriceRepo repository.PlatformBasePriceRepository) PlatformBasePriceFlow {
	return &PlatformBasePriceFlowImpl{
		platformBasePriceRepo: platformBasePriceRepo,
	}
}

// ListPlatformBasePrices returns platform base prices for authenticated users.
func (f *PlatformBasePriceFlowImpl) ListPlatformBasePrices(ctx context.Context) (*dto.ListPlatformBasePricesResponse, error) {
	rows, err := f.platformBasePriceRepo.List(ctx)
	if err != nil {
		return nil, NewBusinessError("PLATFORM_BASE_PRICE_LIST_FAILED", "failed to list platform base prices", err)
	}

	items := make([]dto.PlatformBasePriceItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.PlatformBasePriceItem{
			Platform: row.Platform,
			Price:    row.Price,
		})
	}
	// Keep output deterministic for clients.
	sort.Slice(items, func(i, j int) bool {
		return items[i].Platform < items[j].Platform
	})

	return &dto.ListPlatformBasePricesResponse{
		Message: "Platform base prices retrieved successfully",
		Items:   items,
	}, nil
}
