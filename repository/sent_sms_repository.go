package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// SentSMSRepositoryImpl implements SentSMSRepository
type SentSMSRepositoryImpl struct {
	*BaseRepository[models.SentSMS, models.SentSMSFilter]
}

func NewSentSMSRepository(db *gorm.DB) SentSMSRepository {
	return &SentSMSRepositoryImpl{BaseRepository: NewBaseRepository[models.SentSMS, models.SentSMSFilter](db)}
}

func (r *SentSMSRepositoryImpl) ByID(ctx context.Context, id uint) (*models.SentSMS, error) {
	db := r.getDB(ctx)
	var row models.SentSMS
	if err := db.Last(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *SentSMSRepositoryImpl) ListByProcessedCampaign(ctx context.Context, processedCampaignID uint, limit, offset int) ([]*models.SentSMS, error) {
	filter := models.SentSMSFilter{ProcessedCampaignID: &processedCampaignID}
	return r.ByFilter(ctx, filter, "id ASC", limit, offset)
}

func (r *SentSMSRepositoryImpl) applyFilter(db *gorm.DB, f models.SentSMSFilter) *gorm.DB {
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

func (r *SentSMSRepositoryImpl) ByFilter(ctx context.Context, filter models.SentSMSFilter, orderBy string, limit, offset int) ([]*models.SentSMS, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.SentSMS{}), filter)
	if orderBy != "" {
		query = query.Order(orderBy)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var rows []*models.SentSMS
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SentSMSRepositoryImpl) Count(ctx context.Context, filter models.SentSMSFilter) (int64, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.SentSMS{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *SentSMSRepositoryImpl) Exists(ctx context.Context, filter models.SentSMSFilter) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}
