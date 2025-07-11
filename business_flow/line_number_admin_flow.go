// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"context"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdminLineNumberFlow handles admin operations on line numbers
type AdminLineNumberFlow interface {
	Create(ctx context.Context, req *dto.AdminCreateLineNumberRequest, metadata *ClientMetadata) (*dto.AdminLineNumberDTO, error)
	ListAll(ctx context.Context, metadata *ClientMetadata) ([]*dto.AdminLineNumberDTO, error)
	UpdateBatch(ctx context.Context, req *dto.AdminUpdateLineNumbersRequest, metadata *ClientMetadata) error
	GetReport(ctx context.Context, metadata *ClientMetadata) ([]*dto.AdminLineNumberReportItem, error)
}

type AdminLineNumberFlowImpl struct {
	lineRepo repository.LineNumberRepository
	db       *gorm.DB
}

func NewAdminLineNumberFlow(lineRepo repository.LineNumberRepository, db *gorm.DB) AdminLineNumberFlow {
	return &AdminLineNumberFlowImpl{
		lineRepo: lineRepo,
		db:       db,
	}
}

func (f *AdminLineNumberFlowImpl) Create(ctx context.Context, req *dto.AdminCreateLineNumberRequest, metadata *ClientMetadata) (*dto.AdminLineNumberDTO, error) {
	// Validate
	if req == nil {
		return nil, NewBusinessError("LINE_NUMBER_VALIDATION_FAILED", "Create line number validation failed", ErrLineNumberValueRequired)
	}
	value := strings.TrimSpace(req.LineNumber)
	if value == "" {
		return nil, NewBusinessError("LINE_NUMBER_REQUIRED", "Line number is required", ErrLineNumberValueRequired)
	}
	if len(value) > 50 {
		value = value[:50]
	}
	if req.PriceFactor <= 0 {
		return nil, NewBusinessError("PRICE_FACTOR_INVALID", "Price factor must be greater than zero", ErrPriceFactorInvalid)
	}

	// Uniqueness check
	existing, err := f.lineRepo.ByValue(ctx, value)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, NewBusinessError("LINE_NUMBER_EXISTS", "Line number already exists", ErrLineNumberAlreadyExists)
	}

	// Build entity
	ln := models.LineNumber{
		UUID:        uuid.New(),
		Name:        req.Name,
		LineNumber:  value,
		PriceFactor: req.PriceFactor,
		Priority:    req.Priority,
		IsActive:    req.IsActive,
		CreatedAt:   utils.UTCNow(),
		UpdatedAt:   utils.UTCNow(),
	}

	// Save
	if err := f.lineRepo.Save(ctx, &ln); err != nil {
		return nil, err
	}

	resp := ToLineNumberDTO(ln)
	return &resp, nil
}

func (f *AdminLineNumberFlowImpl) ListAll(ctx context.Context, metadata *ClientMetadata) ([]*dto.AdminLineNumberDTO, error) {
	lines, err := f.lineRepo.ByFilter(ctx, models.LineNumberFilter{}, "id DESC", 0, 0)
	if err != nil {
		return nil, NewBusinessError("LINE_NUMBER_LIST_FAILED", "Failed to list line numbers", err)
	}
	result := make([]*dto.AdminLineNumberDTO, 0, len(lines))
	for _, ln := range lines {
		dtoItem := ToLineNumberDTO(*ln)
		result = append(result, &dtoItem)
	}
	return result, nil
}

func (f *AdminLineNumberFlowImpl) UpdateBatch(ctx context.Context, req *dto.AdminUpdateLineNumbersRequest, metadata *ClientMetadata) error {
	if req == nil || len(req.Items) == 0 {
		return nil
	}
	// Validate and map
	updates := make([]*models.LineNumber, 0, len(req.Items))
	for _, item := range req.Items {
		if item.ID == 0 {
			return NewBusinessError("LINE_NUMBER_UPDATE_VALIDATION_FAILED", "Line number ID is required", ErrLineNumberValueRequired)
		}

		updates = append(updates, &models.LineNumber{
			ID:          item.ID,
			Priority:    item.Priority,
			IsActive:    item.IsActive,
			UpdatedAt:   utils.UTCNow(),
		})
	}
	// Persist
	if err := f.lineRepo.UpdateBatch(ctx, updates); err != nil {
		return NewBusinessError("LINE_NUMBER_BATCH_UPDATE_FAILED", "Failed to update line numbers", err)
	}
	return nil
}

func (f *AdminLineNumberFlowImpl) GetReport(ctx context.Context, metadata *ClientMetadata) ([]*dto.AdminLineNumberReportItem, error) {
	// TODO: implement aggregation logic across messages/campaigns/transactions
	return []*dto.AdminLineNumberReportItem{}, nil
}
