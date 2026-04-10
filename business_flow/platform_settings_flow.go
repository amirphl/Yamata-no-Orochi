package businessflow

import (
	"context"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
)

// PlatformSettingsFlow defines operations for platform settings.
type PlatformSettingsFlow interface {
	CreatePlatformSettings(ctx context.Context, req *dto.CreatePlatformSettingsRequest, metadata *ClientMetadata) (*dto.CreatePlatformSettingsResponse, error)
	ListPlatformSettings(ctx context.Context, customerID uint) (*dto.ListPlatformSettingsResponse, error)
}

// PlatformSettingsFlowImpl implements PlatformSettingsFlow.
type PlatformSettingsFlowImpl struct {
	platformSettingsRepo repository.PlatformSettingsRepository
	multimediaRepo       repository.MultimediaAssetRepository
}

// NewPlatformSettingsFlow creates a new platform settings flow.
func NewPlatformSettingsFlow(
	platformSettingsRepo repository.PlatformSettingsRepository,
	multimediaRepo repository.MultimediaAssetRepository,
) PlatformSettingsFlow {
	return &PlatformSettingsFlowImpl{
		platformSettingsRepo: platformSettingsRepo,
		multimediaRepo:       multimediaRepo,
	}
}

func (f *PlatformSettingsFlowImpl) CreatePlatformSettings(ctx context.Context, req *dto.CreatePlatformSettingsRequest, metadata *ClientMetadata) (*dto.CreatePlatformSettingsResponse, error) {
	if req == nil {
		return nil, NewBusinessError("INVALID_REQUEST", "request is required", nil)
	}

	customerIDAny := ctx.Value(utils.CustomerIDKey)
	customerID, ok := customerIDAny.(uint)
	if !ok || customerID == 0 {
		return nil, NewBusinessError("MISSING_CUSTOMER_ID", "customer id is required", nil)
	}

	status := models.PlatformSettingsStatusInitialized

	var multimediaID *uint
	var multimediaUUID *string
	if req.MultimediaUUID != nil && *req.MultimediaUUID != "" {
		asset, err := f.multimediaRepo.ByUUID(ctx, *req.MultimediaUUID)
		if err != nil {
			return nil, err
		}
		if asset == nil {
			return nil, NewBusinessError("MULTIMEDIA_NOT_FOUND", "multimedia not found", nil)
		}
		multimediaID = &asset.ID
		assetUUID := asset.UUID.String()
		multimediaUUID = &assetUUID
	}

	row := models.PlatformSettings{
		UUID:         uuid.New(),
		CustomerID:   customerID,
		Platform:     req.Platform,
		Name:         req.Name,
		Description:  req.Description,
		MultimediaID: multimediaID,
		Status:       status,
	}

	if err := f.platformSettingsRepo.Save(ctx, &row); err != nil {
		return nil, err
	}

	return &dto.CreatePlatformSettingsResponse{
		Message:        "Platform settings created successfully",
		ID:             row.ID,
		Platform:       row.Platform,
		Name:           row.Name,
		Description:    row.Description,
		MultimediaUUID: multimediaUUID,
		Status:         string(row.Status),
		CreatedAt:      row.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (f *PlatformSettingsFlowImpl) ListPlatformSettings(ctx context.Context, customerID uint) (*dto.ListPlatformSettingsResponse, error) {
	if customerID == 0 {
		return nil, NewBusinessError("MISSING_CUSTOMER_ID", "customer id is required", nil)
	}
	rows, err := f.platformSettingsRepo.ByFilter(ctx, models.PlatformSettingsFilter{CustomerID: &customerID}, "id DESC", 0, 0)
	if err != nil {
		return nil, err
	}

	items := make([]dto.PlatformSettingsItem, 0, len(rows))
	for _, row := range rows {
		var multimediaUUID *string
		if row.MultimediaID != nil {
			asset, err := f.multimediaRepo.ByID(ctx, *row.MultimediaID)
			if err != nil {
				return nil, err
			}
			if asset != nil {
				u := asset.UUID.String()
				multimediaUUID = &u
			}
		}
		items = append(items, dto.PlatformSettingsItem{
			ID:             row.ID,
			Platform:       row.Platform,
			Name:           row.Name,
			Description:    row.Description,
			MultimediaUUID: multimediaUUID,
			Status:         string(row.Status),
			CreatedAt:      row.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      row.UpdatedAt.Format(time.RFC3339),
		})
	}

	return &dto.ListPlatformSettingsResponse{
		Message: "Platform settings retrieved",
		Items:   items,
	}, nil
}
