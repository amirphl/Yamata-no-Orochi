package repository

import (
	"context"
	"errors"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// SMSStatusJobRepositoryImpl implements SMSStatusJobRepository
type SMSStatusJobRepositoryImpl struct {
	*BaseRepository[models.SMSStatusJob, any]
}

func NewSMSStatusJobRepository(db *gorm.DB) SMSStatusJobRepository {
	return &SMSStatusJobRepositoryImpl{BaseRepository: NewBaseRepository[models.SMSStatusJob, any](db)}
}

func (r *SMSStatusJobRepositoryImpl) ByID(ctx context.Context, id uint) (*models.SMSStatusJob, error) {
	db := r.getDB(ctx)
	var row models.SMSStatusJob
	if err := db.Last(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ListDue returns jobs scheduled before or at 'now' with retry_count < 3
func (r *SMSStatusJobRepositoryImpl) ListDue(ctx context.Context, now time.Time, limit int) ([]*models.SMSStatusJob, error) {
	if limit <= 0 {
		limit = 100
	}
	db := r.getDB(ctx)
	var rows []*models.SMSStatusJob
	if err := db.Where("scheduled_at <= ? AND retry_count < ? AND executed_at == NULL", now, 3).
		Order("scheduled_at ASC, id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SMSStatusJobRepositoryImpl) SaveBatch(ctx context.Context, jobs []*models.SMSStatusJob) error {
	return r.BaseRepository.SaveBatch(ctx, jobs)
}

func (r *SMSStatusJobRepositoryImpl) Update(ctx context.Context, job *models.SMSStatusJob) error {
	db := r.getDB(ctx)
	return db.Save(job).Error
}

// ByFilter: since no filter is defined, apply order/limit/offset only
func (r *SMSStatusJobRepositoryImpl) ByFilter(ctx context.Context, _ any, orderBy string, limit, offset int) ([]*models.SMSStatusJob, error) {
	db := r.getDB(ctx)
	if orderBy != "" {
		db = db.Order(orderBy)
	}
	if limit > 0 {
		db = db.Limit(limit)
	}
	if offset > 0 {
		db = db.Offset(offset)
	}
	var rows []*models.SMSStatusJob
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SMSStatusJobRepositoryImpl) Count(ctx context.Context, _ any) (int64, error) {
	db := r.getDB(ctx)
	var count int64
	if err := db.Model(&models.SMSStatusJob{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *SMSStatusJobRepositoryImpl) Exists(ctx context.Context, filter any) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}
