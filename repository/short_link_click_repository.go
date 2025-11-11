package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// ShortLinkClickRepositoryImpl implements ShortLinkClickRepository
type ShortLinkClickRepositoryImpl struct {
	*BaseRepository[models.ShortLinkClick, any]
}

func NewShortLinkClickRepository(db *gorm.DB) ShortLinkClickRepository {
	return &ShortLinkClickRepositoryImpl{BaseRepository: NewBaseRepository[models.ShortLinkClick, any](db)}
}

func (r *ShortLinkClickRepositoryImpl) ByID(ctx context.Context, id uint) (*models.ShortLinkClick, error) {
	db := r.getDB(ctx)
	var row models.ShortLinkClick
	if err := db.Last(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ByFilter: since no filter is defined, return with order/limit/offset only
func (r *ShortLinkClickRepositoryImpl) ByFilter(ctx context.Context, _ any, orderBy string, limit, offset int) ([]*models.ShortLinkClick, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.ShortLinkClick{})
	if orderBy != "" {
		query = query.Order(orderBy)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var rows []*models.ShortLinkClick
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *ShortLinkClickRepositoryImpl) Count(ctx context.Context, _ any) (int64, error) {
	db := r.getDB(ctx)
	var count int64
	if err := db.Model(&models.ShortLinkClick{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *ShortLinkClickRepositoryImpl) Exists(ctx context.Context, filter any) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}
