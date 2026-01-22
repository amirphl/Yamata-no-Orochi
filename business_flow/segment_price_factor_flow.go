package businessflow

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
)

// SegmentPriceFactorFlow defines admin operations for segment price factors.
type SegmentPriceFactorFlow interface {
	AdminCreateSegmentPriceFactor(ctx context.Context, req *dto.AdminCreateSegmentPriceFactorRequest) (*dto.AdminCreateSegmentPriceFactorResponse, error)
	AdminListSegmentPriceFactors(ctx context.Context) (*dto.AdminListSegmentPriceFactorsResponse, error)
	AdminListLevel3Options(ctx context.Context) (*dto.AdminListLevel3OptionsResponse, error)
}

type SegmentPriceFactorFlowImpl struct {
	segmentPriceFactorRepo repository.SegmentPriceFactorRepository
}

func NewSegmentPriceFactorFlow(segmentPriceFactorRepo repository.SegmentPriceFactorRepository) SegmentPriceFactorFlow {
	return &SegmentPriceFactorFlowImpl{segmentPriceFactorRepo: segmentPriceFactorRepo}
}

// AdminCreateSegmentPriceFactor upserts a price factor for a given level3 (latest row wins).
func (f *SegmentPriceFactorFlowImpl) AdminCreateSegmentPriceFactor(ctx context.Context, req *dto.AdminCreateSegmentPriceFactorRequest) (*dto.AdminCreateSegmentPriceFactorResponse, error) {
	level3 := strings.TrimSpace(req.Level3)
	if level3 == "" {
		return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_LEVEL3_REQUIRED", "Level3 is required", ErrLevel3Required)
	}
	if req.PriceFactor <= 0 {
		return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_PRICE_INVALID", "Price factor must be greater than zero", ErrPriceFactorInvalid)
	}

	row := &models.SegmentPriceFactor{
		Level3:      level3,
		PriceFactor: req.PriceFactor,
		CreatedAt:   utils.UTCNow(),
		UpdatedAt:   utils.UTCNow(),
	}
	if err := f.segmentPriceFactorRepo.Save(ctx, row); err != nil {
		return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_SAVE_FAILED", "Failed to save segment price factor", err)
	}

	return &dto.AdminCreateSegmentPriceFactorResponse{
		Message: "Segment price factor saved successfully",
	}, nil
}

// AdminListSegmentPriceFactors returns the latest price factor per level3.
func (f *SegmentPriceFactorFlowImpl) AdminListSegmentPriceFactors(ctx context.Context) (*dto.AdminListSegmentPriceFactorsResponse, error) {
	rows, err := f.segmentPriceFactorRepo.ListLatestByLevel3(ctx)
	if err != nil {
		return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_LIST_FAILED", "Failed to list segment price factors", err)
	}

	items := make([]dto.AdminSegmentPriceFactorItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.AdminSegmentPriceFactorItem{
			Level3:      r.Level3,
			PriceFactor: r.PriceFactor,
			CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		})
	}

	// Sort by level3 for deterministic output
	sort.Slice(items, func(i, j int) bool {
		return strings.Compare(items[i].Level3, items[j].Level3) < 0
	})

	return &dto.AdminListSegmentPriceFactorsResponse{
		Message: "Segment price factors retrieved successfully",
		Items:   items,
	}, nil
}

// AdminListLevel3Options returns available level3 keys from the audience spec (filtered to entries with available audience > 0).
func (f *SegmentPriceFactorFlowImpl) AdminListLevel3Options(ctx context.Context) (*dto.AdminListLevel3OptionsResponse, error) {
	filePath := audienceSpecFilePath()
	spec, err := readAudienceSpecFileV2(filePath)
	if err != nil {
		return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_LEVEL3_LOAD_FAILED", "Failed to load audience spec", err)
	}

	set := make(map[string]struct{})
	for _, l2map := range spec {
		for _, node := range l2map {
			if node == nil || len(node.Items) == 0 {
				continue
			}
			for l3, leaf := range node.Items {
				if leaf.AvailableAudience > 0 {
					set[l3] = struct{}{}
				}
			}
		}
	}

	items := make([]string, 0, len(set))
	for k := range set {
		items = append(items, k)
	}
	sort.Strings(items)

	return &dto.AdminListLevel3OptionsResponse{
		Message: "Level3 options retrieved successfully",
		Items:   items,
	}, nil
}
