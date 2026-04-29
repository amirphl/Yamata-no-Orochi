package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// SentSplusMessageRepositoryImpl implements SentSplusMessageRepository.
type SentSplusMessageRepositoryImpl struct {
	*BaseRepository[models.SentSplusMessage, models.SentSplusMessageFilter]
}

func NewSentSplusMessageRepository(db *gorm.DB) SentSplusMessageRepository {
	return &SentSplusMessageRepositoryImpl{BaseRepository: NewBaseRepository[models.SentSplusMessage, models.SentSplusMessageFilter](db)}
}

func (r *SentSplusMessageRepositoryImpl) ByID(ctx context.Context, id uint) (*models.SentSplusMessage, error) {
	db := r.getDB(ctx)
	var row models.SentSplusMessage
	if err := db.Last(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *SentSplusMessageRepositoryImpl) ListByProcessedCampaign(ctx context.Context, processedCampaignID uint, limit, offset int) ([]*models.SentSplusMessage, error) {
	filter := models.SentSplusMessageFilter{ProcessedCampaignID: &processedCampaignID}
	return r.ByFilter(ctx, filter, "id ASC", limit, offset)
}

func (r *SentSplusMessageRepositoryImpl) applyFilter(db *gorm.DB, f models.SentSplusMessageFilter) *gorm.DB {
	if f.ID != nil {
		db = db.Where("id = ?", *f.ID)
	}
	if f.ProcessedCampaignID != nil {
		db = db.Where("processed_campaign_id = ?", *f.ProcessedCampaignID)
	}
	if f.PhoneNumber != nil {
		db = db.Where("phone_number = ?", *f.PhoneNumber)
	}
	if f.Status != nil {
		db = db.Where("status = ?", *f.Status)
	}
	if f.CreatedAfter != nil {
		db = db.Where("created_at >= ?", *f.CreatedAfter)
	}
	if f.CreatedBefore != nil {
		db = db.Where("created_at < ?", *f.CreatedBefore)
	}
	return db
}

func (r *SentSplusMessageRepositoryImpl) ByFilter(ctx context.Context, filter models.SentSplusMessageFilter, orderBy string, limit, offset int) ([]*models.SentSplusMessage, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.SentSplusMessage{}), filter)
	if orderBy != "" {
		query = query.Order(orderBy)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var rows []*models.SentSplusMessage
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SentSplusMessageRepositoryImpl) Count(ctx context.Context, filter models.SentSplusMessageFilter) (int64, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.SentSplusMessage{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *SentSplusMessageRepositoryImpl) Exists(ctx context.Context, filter models.SentSplusMessageFilter) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}

func (r *SentSplusMessageRepositoryImpl) UpdateSendResultByTrackingID(
	ctx context.Context,
	trackingID string,
	status models.SplusSendStatus,
	partsDelivered int,
	serverID, errorCode, description *string,
) error {
	db := r.getDB(ctx)
	updates := map[string]any{
		"status":          status,
		"parts_delivered": partsDelivered,
		"server_id":       serverID,
		"error_code":      errorCode,
		"description":     description,
	}
	return db.Model(&models.SentSplusMessage{}).Where("tracking_id = ?", trackingID).Updates(updates).Error
}
