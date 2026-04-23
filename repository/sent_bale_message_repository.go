package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// SentBaleMessageRepositoryImpl implements SentBaleMessageRepository.
type SentBaleMessageRepositoryImpl struct {
	*BaseRepository[models.SentBaleMessage, models.SentBaleMessageFilter]
}

func NewSentBaleMessageRepository(db *gorm.DB) SentBaleMessageRepository {
	return &SentBaleMessageRepositoryImpl{BaseRepository: NewBaseRepository[models.SentBaleMessage, models.SentBaleMessageFilter](db)}
}

func (r *SentBaleMessageRepositoryImpl) ByID(ctx context.Context, id uint) (*models.SentBaleMessage, error) {
	db := r.getDB(ctx)
	var row models.SentBaleMessage
	if err := db.Last(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *SentBaleMessageRepositoryImpl) ListByProcessedCampaign(ctx context.Context, processedCampaignID uint, limit, offset int) ([]*models.SentBaleMessage, error) {
	filter := models.SentBaleMessageFilter{ProcessedCampaignID: &processedCampaignID}
	return r.ByFilter(ctx, filter, "id ASC", limit, offset)
}

func (r *SentBaleMessageRepositoryImpl) applyFilter(db *gorm.DB, f models.SentBaleMessageFilter) *gorm.DB {
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

func (r *SentBaleMessageRepositoryImpl) ByFilter(ctx context.Context, filter models.SentBaleMessageFilter, orderBy string, limit, offset int) ([]*models.SentBaleMessage, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.SentBaleMessage{}), filter)
	if orderBy != "" {
		query = query.Order(orderBy)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var rows []*models.SentBaleMessage
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SentBaleMessageRepositoryImpl) Count(ctx context.Context, filter models.SentBaleMessageFilter) (int64, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.SentBaleMessage{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *SentBaleMessageRepositoryImpl) Exists(ctx context.Context, filter models.SentBaleMessageFilter) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}

func (r *SentBaleMessageRepositoryImpl) UpdateSendResultByTrackingID(
	ctx context.Context,
	trackingID string,
	status models.BaleSendStatus,
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
	return db.Model(&models.SentBaleMessage{}).Where("tracking_id = ?", trackingID).Updates(updates).Error
}
