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
	ListLatestSegmentPriceFactors(ctx context.Context) (*dto.ListLatestSegmentPriceFactorsResponse, error)
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

	items := mapSegmentPriceFactorRows(rows)

	return &dto.AdminListSegmentPriceFactorsResponse{
		Message: "Segment price factors retrieved successfully",
		Items:   items.AdminItems,
	}, nil
}

// ListLatestSegmentPriceFactors returns the latest price factor per level3 for authenticated users.
func (f *SegmentPriceFactorFlowImpl) ListLatestSegmentPriceFactors(ctx context.Context) (*dto.ListLatestSegmentPriceFactorsResponse, error) {
	rows, err := f.segmentPriceFactorRepo.ListLatestByLevel3(ctx)
	if err != nil {
		return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_LIST_FAILED", "Failed to list segment price factors", err)
	}

	items := mapSegmentPriceFactorRows(rows)

	return &dto.ListLatestSegmentPriceFactorsResponse{
		Message: "Segment price factors retrieved successfully",
		Items:   items.PublicItems,
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

type segmentPriceFactorMappedItems struct {
	AdminItems  []dto.AdminSegmentPriceFactorItem
	PublicItems []dto.SegmentPriceFactorItem
}

func mapSegmentPriceFactorRows(rows []*models.SegmentPriceFactor) segmentPriceFactorMappedItems {
	adminItems := make([]dto.AdminSegmentPriceFactorItem, 0, len(rows))
	publicItems := make([]dto.SegmentPriceFactorItem, 0, len(rows))
	for _, r := range rows {
		createdAt := r.CreatedAt.Format(time.RFC3339)
		adminItems = append(adminItems, dto.AdminSegmentPriceFactorItem{
			Level3:      r.Level3,
			PriceFactor: r.PriceFactor,
			CreatedAt:   createdAt,
		})
		publicItems = append(publicItems, dto.SegmentPriceFactorItem{
			Level3:      r.Level3,
			PriceFactor: r.PriceFactor,
			CreatedAt:   createdAt,
		})
	}

	// Sort by level3 for deterministic output
	sort.Slice(adminItems, func(i, j int) bool {
		return strings.Compare(adminItems[i].Level3, adminItems[j].Level3) < 0
	})
	sort.Slice(publicItems, func(i, j int) bool {
		return strings.Compare(publicItems[i].Level3, publicItems[j].Level3) < 0
	})

	return segmentPriceFactorMappedItems{
		AdminItems:  adminItems,
		PublicItems: publicItems,
	}
}
